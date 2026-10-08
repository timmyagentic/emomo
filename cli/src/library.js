import { createHash } from 'node:crypto';
import { readFile, writeFile, mkdir, mkdtemp, rename, rm, realpath, stat } from 'node:fs/promises';
import { resolve, join, dirname, relative, isAbsolute } from 'node:path';
import { LocalCatalog } from './local.js';
import { identifyImage } from './client.js';
import { EmomoError, publicError } from './error.js';

const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const key = value => typeof value === 'string' && /^[a-zA-Z0-9_-]{1,100}$/.test(value);
const digest = value => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
const fail = () => new EmomoError('INVALID_SNAPSHOT', 'The private library snapshot is invalid or incomplete.');
export function canonical(value) {
  if (Array.isArray(value)) return `[${value.map(canonical).join(',')}]`;
  if (value && typeof value === 'object') return `{${Object.keys(value).sort().map(k => `${JSON.stringify(k)}:${canonical(value[k])}`).join(',')}}`;
  return JSON.stringify(value);
}
async function bounded(path, maximum = 32 * 1024 * 1024) {
  if ((await stat(path)).size > maximum) throw fail();
  const bytes = await readFile(path);
  if (bytes.length > maximum) throw fail();
  return bytes;
}
async function contained(root, asset) {
  if (typeof asset !== 'string' || isAbsolute(asset) || asset.includes('\\') || asset.split('/').some(p => !p || p === '.' || p === '..')) throw fail();
  const path = await realpath(join(root, asset));
  const rel = relative(root, path);
  if (!rel || rel === '..' || rel.startsWith('../') || isAbsolute(rel)) throw fail();
  return path;
}
function validateRows(rows) {
  const allowed = new Set(['id','sourceId','description','imageText','category','tags','subjects','scenarios','aliases','contentFlags','quality','mediaKind','previewOnly','image','sha256','publicReleaseClearance','asset']);
  if (!Array.isArray(rows) || !rows.length || rows.length > 100000) throw fail();
  const ids = new Set();
  for (const r of rows) {
    if (!r || Object.keys(r).some(k => !allowed.has(k)) || !key(r.id) || ids.has(r.id) || !digest(r.sha256) || !['usable','object_sticker'].includes(r.category)) throw fail();
    ids.add(r.id);
    for (const f of ['sourceId','description','imageText','quality','publicReleaseClearance']) if (typeof r[f] !== 'string' || r[f].length > 16384) throw fail();
    for (const f of ['tags','subjects','scenarios','aliases','contentFlags']) if (!Array.isArray(r[f]) || r[f].length > 256 || r[f].some(v => typeof v !== 'string' || v.length > 2048)) throw fail();
    if (!r.image || Object.keys(r.image).some(k => !['width','height','frames','format'].includes(k)) || !['png','jpeg','webp','gif'].includes(r.image.format)) throw fail();
    for (const f of ['width','height','frames']) if (!Number.isSafeInteger(r.image[f]) || r.image[f] < 1) throw fail();
    if (!['image','preview','animation'].includes(r.mediaKind) || r.previewOnly !== (r.mediaKind === 'preview') || (r.image.frames > 1) !== (r.mediaKind === 'animation')) throw fail();
    const extension = r.image.format === 'jpeg' ? 'jpg' : r.image.format;
    if (r.asset !== `assets/${r.sha256}.${extension}`) throw fail();
  }
}
function summary(rows) {
  const assets = new Set(rows.map(r => r.asset));
  return { records: rows.length, usable: rows.filter(r => r.category === 'usable').length, objects: rows.filter(r => r.category === 'object_sticker').length,
    uniqueAssets: assets.size, animations: rows.filter(r => r.image.frames > 1).length,
    publicReleaseUnverified: rows.filter(r => r.publicReleaseClearance !== 'CLEARED').map(r => r.id),
    remoteCompatibilityBlocked: rows.filter(r => r.image.format === 'gif' || r.image.frames > 1).map(r => r.id),
    publicReleaseReady: false, modelsUsed: [], networkCalls: 0 };
}
function revisionDigest(manifest) {
  return hash(canonical({ collection: manifest.collection, recordsSha256: manifest.recordsSha256, vocabularySha256: manifest.vocabularySha256 }));
}
export async function validateSnapshot(directory) {
  const root = await realpath(resolve(directory));
  const manifest = JSON.parse((await bounded(await contained(root, 'snapshot.json'))).toString());
  if (manifest.schemaVersion !== 1 || manifest.format !== 'emomo-private-library' || !key(manifest.collection) || !key(manifest.revision) || !digest(manifest.recordsSha256) || !digest(manifest.vocabularySha256) || !digest(manifest.contentRevision)) throw fail();
  const bytes = await bounded(await contained(root, 'records.json'));
  const vocab = await bounded(await contained(root, 'vocabulary.json'));
  if (hash(bytes) !== manifest.recordsSha256 || hash(vocab) !== manifest.vocabularySha256 || revisionDigest(manifest) !== manifest.contentRevision) throw fail();
  const rows = JSON.parse(bytes.toString()); validateRows(rows);
  const vocabulary = JSON.parse(vocab.toString());
  if (!vocabulary || !['intents','subjects','gaps'].every(f => vocabulary[f] && typeof vocabulary[f] === 'object' && !Array.isArray(vocabulary[f]))) throw fail();
  if (manifest.records !== rows.length) throw fail();
  let totalBytes = 0; const seen = new Set();
  for (const r of rows) {
    if (seen.has(r.asset)) continue;
    const image = await bounded(await contained(root, r.asset), 25 * 1024 * 1024);
    if (hash(image) !== r.sha256) throw fail();
    const format = identifyImage(image, { allowGif: true });
    if (format.extension !== (r.image.format === 'jpeg' ? 'jpg' : r.image.format)) throw fail();
    seen.add(r.asset); totalBytes += image.length;
  }
  return { root, manifest, rows, vocabulary, report: { ...summary(rows), totalBytes, contentRevision: manifest.contentRevision, collection: manifest.collection, revision: manifest.revision, hashesVerified: true } };
}
export async function prepareSnapshot(catalogPath, directory, { collection, revision } = {}) {
  if (!key(collection) || !key(revision)) throw new EmomoError('INVALID_ARGUMENT', 'Specify a collection and revision using letters, digits, hyphens or underscores.');
  const catalog = await LocalCatalog.open(catalogPath);
  let stage, reserved = false;
  const target = resolve(directory);
  try {
    const rows = catalog.db.prepare('SELECT data FROM items ORDER BY id').all().map(r => JSON.parse(r.data)).sort((a,b) => a.id.localeCompare(b.id));
    validateRows(rows);
    await mkdir(dirname(target), { recursive: true });
    stage = await mkdtemp(join(dirname(target), '.emomo-snapshot-'));
    await mkdir(join(stage, 'assets'));
    const seen = new Set();
    for (const row of rows) {
      // Read and hash the exact bytes we copy; a source changing after validation cannot slip through.
      const bytes = await bounded(await catalog.contained(row.asset), 25 * 1024 * 1024);
      if (hash(bytes) !== row.sha256) throw fail();
      if (!seen.has(row.asset)) { await writeFile(join(stage, row.asset), bytes, { mode: 0o600, flag: 'wx' }); seen.add(row.asset); }
    }
    const records = canonical(rows) + '\n', vocabulary = canonical(catalog.lexicon) + '\n';
    const manifest = { schemaVersion: 1, format: 'emomo-private-library', collection, revision, createdAt: new Date().toISOString(), records: rows.length, recordsSha256: hash(records), vocabularySha256: hash(vocabulary) };
    manifest.contentRevision = revisionDigest(manifest);
    await writeFile(join(stage, 'records.json'), records, { mode: 0o600, flag: 'wx' });
    await writeFile(join(stage, 'vocabulary.json'), vocabulary, { mode: 0o600, flag: 'wx' });
    await writeFile(join(stage, 'snapshot.json'), JSON.stringify(manifest,null,2)+'\n', { mode: 0o600, flag: 'wx' });
    const validated = await validateSnapshot(stage);
    await writeFile(join(stage, 'review.json'), JSON.stringify(validated.report,null,2)+'\n', { mode: 0o600, flag: 'wx' });
    try { await mkdir(target); reserved = true; }
    catch (e) { if (e.code === 'EEXIST') throw new EmomoError('SNAPSHOT_EXISTS', 'The snapshot destination already exists; use a new revision directory.'); throw e; }
    await rename(stage, target); reserved = false;
    return { path: target, ...validated.report };
  } finally {
    catalog.close();
    if (reserved) await rm(target, { recursive: true, force: true });
    if (stage) await rm(stage, { recursive: true, force: true });
  }
}
export async function diffSnapshots(basePath, targetPath) {
  const base = await validateSnapshot(basePath), target = await validateSnapshot(targetPath);
  if (base.manifest.collection !== target.manifest.collection) throw new EmomoError('COLLECTION_MISMATCH', 'Compare revisions of the same collection; local image IDs are scoped to a collection.');
  const old = new Map(base.rows.map(r => [r.id,r])), current = new Map(target.rows.map(r => [r.id,r]));
  const oldAssets = new Set(base.rows.map(r => r.asset));
  const added = [], changed = [], removed = [];
  for (const [id,r] of current) {
    const previous = old.get(id);
    if (!previous) added.push({ id, assetReused: oldAssets.has(r.asset) });
    else {
      const fields = Object.keys(r).filter(f => canonical(r[f]) !== canonical(previous[f]));
      if (fields.length) changed.push({ id, fields, imageChanged: r.sha256 !== previous.sha256 });
    }
  }
  for (const id of old.keys()) if (!current.has(id)) removed.push(id);
  const lexiconChanged = canonical(base.vocabulary) !== canonical(target.vocabulary);
  const newAssets = [...new Set(target.rows.map(r => r.asset))].filter(asset => !oldAssets.has(asset));
  let newAssetBytes = 0;
  for (const asset of newAssets) newAssetBytes += (await stat(await contained(target.root, asset))).size;
  return { collection: base.manifest.collection, baseRevision: base.manifest.contentRevision, targetRevision: target.manifest.contentRevision,
    added, changed, removed, lexiconChanged, newAssets, newAssetBytes, unchanged: !added.length && !changed.length && !removed.length && !lexiconChanged,
    removalsArePlanOnly: true, publicReleaseReady: false, modelsUsed: [], networkCalls: 0 };
}
export async function libraryMain(args) {
  const command = args[0] ?? 'help';
  try {
    let data;
    if (command === 'help' && args.length <= 1) data = { commands: ['prepare --catalog PATH --dir NEW_PATH --collection NAME --revision NAME','validate PATH','diff BASE_PATH TARGET_PATH'], localOnly: true, publishes: false };
    else if (command === 'prepare') {
      const opts = {};
      for (let i=1;i<args.length;i+=2) {
        if (!['--catalog','--dir','--collection','--revision'].includes(args[i]) || !args[i+1] || args[i+1].startsWith('--') || opts[args[i]]) throw new EmomoError('INVALID_ARGUMENT','Use all four prepare options exactly once.');
        opts[args[i]] = args[i+1];
      }
      if (Object.keys(opts).length !== 4) throw new EmomoError('INVALID_ARGUMENT','Use all four prepare options exactly once.');
      data = await prepareSnapshot(opts['--catalog'], opts['--dir'], { collection: opts['--collection'], revision: opts['--revision'] });
    } else if (command === 'validate' && args.length === 2) data = (await validateSnapshot(args[1])).report;
    else if (command === 'diff' && args.length === 3) data = await diffSnapshots(args[1], args[2]);
    else throw new EmomoError('INVALID_ARGUMENT','Run emomo-library help for the local maintenance commands.');
    process.stdout.write(JSON.stringify({schemaVersion:1,ok:true,command,data})+'\n');return 0;
  } catch (error) { process.stdout.write(JSON.stringify({schemaVersion:1,ok:false,command,error:publicError(error)})+'\n');return 1; }
}
