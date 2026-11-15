import { createHash, randomUUID } from 'node:crypto';
import { readdir, readFile, writeFile, mkdir, mkdtemp, rm, stat, realpath, rename, unlink } from 'node:fs/promises';
import { join, resolve, relative, extname, dirname, isAbsolute, sep } from 'node:path';
import { importRows, LocalCatalog } from './local.js';
import { canonical, prepareSnapshot, validateSnapshot } from './library.js';
import { importMetadata, MAX_IMPORT_IMAGE_BYTES, SHA256 } from './import-metadata.js';
import { staticImageInfo } from './image-info.js';
import { publishDirectory as publishAtomic } from './publish-directory.js';
import { configuredImportClient, connectImport, IMPORT_API } from './import-client.js';
import { readAccess } from './auth.js';
import { EmomoClient, DEFAULT_API_URL } from './client.js';
import { setTimeout as delay } from 'node:timers/promises';
import { EmomoError, publicError } from './error.js';

const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const digest = value => hash(canonical(value));
const fail = message => new EmomoError('INVALID_IMPORT_BATCH', message);
const readJSON = async path => {
  if ((await stat(path)).size > 32 * 1024 * 1024) throw fail('Batch metadata is too large.');
  return JSON.parse(await readFile(path, 'utf8'));
};
async function saveJSON(path, value) {
  const temporary = `${path}.${randomUUID()}`;
  try {
    await writeFile(temporary, JSON.stringify(value, null, 2) + '\n', { flag: 'wx', mode: 0o600 });
    await rename(temporary, path);
  } finally { await unlink(temporary).catch(() => {}); }
}
async function scan(root) {
  const images = [], rejected = [];
  async function walk(directory) {
    const entries = (await readdir(directory, { withFileTypes: true })).sort((a, b) => a.name.localeCompare(b.name));
    for (const entry of entries) {
      if (entry.name.startsWith('.')) continue;
      const path = join(directory, entry.name), name = relative(root, path);
      if (entry.isSymbolicLink()) { rejected.push({ path: name, reason: 'symbolic_link' }); continue; }
      if (entry.isDirectory()) { await walk(path); continue; }
      if (!entry.isFile() || !['.png', '.jpg', '.jpeg', '.webp', '.gif'].includes(extname(path).toLowerCase())) continue;
      try {
        if ((await stat(path)).size > MAX_IMPORT_IMAGE_BYTES) throw new Error();
        const bytes = await readFile(path), info = staticImageInfo(bytes);
        images.push({ path: name, sha256: hash(bytes), bytes: bytes.length, image_info: info });
      } catch { rejected.push({ path: name, reason: 'unsupported_animated_or_invalid_image' }); }
    }
  }
  await walk(root);
  return { images, rejected };
}
async function asset(batch, row) {
  if (!SHA256.test(row.sha256)) throw fail('Invalid image hash.');
  const path = join(await realpath(batch), 'assets', row.sha256);
  const actual = await realpath(path);
  if (actual !== path) throw fail('Batch images must be regular files inside the batch.');
  if ((await stat(path)).size > MAX_IMPORT_IMAGE_BYTES) throw fail('Batch image exceeds the size limit.');
  const bytes = await readFile(path);
  if (hash(bytes) !== row.sha256 || canonical(staticImageInfo(bytes)) !== canonical(row.image_info)) throw fail('A prepared image changed. Prepare a new batch.');
  return bytes;
}
function annotationTemplate(row) {
  return { sha256: row.sha256, decision: 'pending', description: '', ocr_text: '', text_presence: null,
    tags: [], search_aliases: [], category: 'usable', preview_only: false, queries: [], reason: '', duplicate_of: '' };
}

export async function ingestDirectory(source, output, client = null) {
  const root = await realpath(resolve(source)), batch = resolve(output ?? `${root}-emomo-import`);
  const outputRelative = relative(root, batch);
  if (!outputRelative || (outputRelative !== '..' && !outputRelative.startsWith(`..${sep}`) && !isAbsolute(outputRelative))) throw fail('Place the batch directory outside the source directory.');
  const current = await scan(root);
  if (!current.images.length) throw fail('No supported static images found.');
  let manifest;
  try { manifest = await readJSON(join(batch, 'scan.json')); }
  catch (error) { if (error.code !== 'ENOENT') throw error; }
  if (!manifest) {
    const unique = [...new Map(current.images.map(row => [row.sha256, row])).values()];
    const existing = client ? await client.lookup(unique.map(row => row.sha256)) : new Map();
    await mkdir(dirname(batch), { recursive: true });
    const stage = await mkdtemp(join(dirname(batch), '.emomo-import-'));
    try {
      await mkdir(join(stage, 'assets'), { mode: 0o700 });
      for (const row of unique) {
        const bytes = await readFile(join(root, row.path));
        if (hash(bytes) !== row.sha256) throw fail('A source image changed during scanning. Start a new batch.');
        await writeFile(join(stage, 'assets', row.sha256), bytes, { flag: 'wx', mode: 0o600 });
      }
      manifest = { schemaVersion: 1, source: root, ...current, unique, existing: [...existing.values()], importApi: client?.apiUrl ?? IMPORT_API, remoteDuplicatesChecked: !!client };
      await saveJSON(join(stage, 'annotations.json'), unique.filter(row => !existing.has(row.sha256)).map(annotationTemplate));
      await saveJSON(join(stage, 'scan.json'), manifest);
      await publishAtomic(stage, batch, 'scan.json');
    } finally { await rm(stage, { recursive: true, force: true }); }
  }
  if (manifest.source !== root || digest(current) !== digest({ images: manifest.images, rejected: manifest.rejected })) throw fail('Source files changed since scanning. Prepare a new batch directory.');
  const expected = manifest.unique.filter(row => !manifest.existing.some(item => item.content_hash === row.sha256));
  const annotations = await readJSON(join(batch, 'annotations.json'));
  if (!Array.isArray(annotations) || annotations.length !== expected.length || new Set(annotations.map(row => row.sha256)).size !== expected.length || annotations.some(row => !expected.some(image => image.sha256 === row.sha256))) throw fail('Annotate every unique new image exactly once.');
  const pending = annotations.filter(row => row.decision === 'pending');
  if (pending.length) return { status: 'needs_annotation', batch, totalFiles: current.images.length, uniqueImages: manifest.unique.length,
    exactDuplicates: current.images.length - manifest.unique.length, alreadyOnline: manifest.existing.length, rejected: current.rejected,
    annotations: join(batch, 'annotations.json'), pending: pending.map(row => ({ sha256: row.sha256, image: join(batch, 'assets', row.sha256) })),
    instruction: 'View every pending image. Fill description, exact visible OCR, text_presence (1 unknown, 2 with text, 3 without), factual tags and 1–3 search queries. Decide keep/exclude/duplicate; record a reason and duplicate_of for non-kept images. Re-run the same ingest command to validate.' };
  const imports = [];
  const remoteDuplicateTargets = new Map();
  for (const row of annotations) {
    const image = expected.find(item => item.sha256 === row.sha256);
    await asset(batch, image);
    if (row.decision === 'keep') {
      let metadata;
      try { metadata = importMetadata({ ...row, image_info: image.image_info }); }
      catch { throw fail(`Complete the annotation for ${row.sha256}.`); }
      if (!Array.isArray(row.queries) || !row.queries.length || row.queries.length > 3 || row.queries.some(query => typeof query !== 'string' || !query.trim() || query.length > 160)) throw fail('Provide 1–3 realistic keyword queries for every kept image.');
      imports.push({ metadata, queries: row.queries });
    } else {
      if (!['exclude', 'duplicate'].includes(row.decision) || typeof row.reason !== 'string' || !row.reason.trim()) throw fail('Every excluded or duplicate image needs an explicit reason.');
      if (row.decision === 'duplicate' && !annotations.some(target => target.sha256 === row.duplicate_of && target.decision === 'keep') && !manifest.existing.some(target => target.content_hash === row.duplicate_of)) {
        if (!SHA256.test(row.duplicate_of) || !client) throw fail('A duplicate must point to a kept image or a verified online SHA-256.');
        const target = (await client.lookup([row.duplicate_of])).get(row.duplicate_of);
        if (!target) throw fail('The selected duplicate target does not exist online.');
        remoteDuplicateTargets.set(row.duplicate_of, target);
      }
    }
  }
  const plan = { schemaVersion: 1, importApi: manifest.importApi, imports, coverage: { images: manifest.images, rejected: manifest.rejected, existing: manifest.existing, remoteDuplicateTargets: [...remoteDuplicateTargets.values()], annotations } };
  const confirmation = digest(plan), catalogPath = join(batch, `catalog-${confirmation.slice(0, 16)}`), snapshotPath = join(batch, `snapshot-${confirmation.slice(0, 16)}`);
  const checks = [];
  if (imports.length) {
    const rows = imports.map(({ metadata: m }) => ({ id: m.sha256, category: m.category, description: m.preview_only ? `[静态首帧预览] ${m.description}` : m.description,
      image_text: m.ocr_text, intent_tags: m.tags, query_aliases: m.search_aliases, file_path: join(batch, 'assets', m.sha256), sha256: m.sha256,
      width: m.image_info.width, height: m.image_info.height, frames: 1, media_kind: m.preview_only ? 'preview' : 'image' }));
    try { await stat(catalogPath); } catch (error) {
      if (error.code !== 'ENOENT') throw error;
      await importRows(rows, catalogPath);
    }
    const catalog = await LocalCatalog.open(catalogPath);
    try {
      await catalog.verify();
      for (const { metadata: m, queries } of imports) {
        for (const query of queries) {
          const found = await catalog.search(query, { limit: 100, category: m.category });
          checks.push({ sha256: m.sha256, query, passed: found.results.some(result => result.id === `local-${m.sha256}`) });
        }
      }
    } finally { catalog.close(); }
    if (checks.some(check => !check.passed)) return { status: 'validation_failed', batch, checks, instruction: 'Review the missing terms or inappropriate queries, then re-run ingest.' };
    try { await stat(snapshotPath); } catch (error) {
      if (error.code !== 'ENOENT') throw error;
      await prepareSnapshot(catalogPath, snapshotPath, { collection: 'reviewed', revision: confirmation.slice(0, 16) });
    }
    await validateSnapshot(snapshotPath);
  }
  await saveJSON(join(batch, 'prepared.json'), { ...plan, confirmation });
  const report = { status: 'ready', batch, confirmation, totalFiles: manifest.images.length, newImages: imports.length,
    exactDuplicates: manifest.images.length - manifest.unique.length, alreadyOnline: manifest.existing.length,
    excluded: annotations.filter(row => row.decision === 'exclude').length, similarDuplicates: annotations.filter(row => row.decision === 'duplicate').length,
    rejected: manifest.rejected, checks, catalogPath: imports.length ? catalogPath : null, snapshotPath: imports.length ? snapshotPath : null,
    remoteDuplicatesChecked: manifest.remoteDuplicatesChecked, publishCommand: `emomo-library publish "${batch}" --confirm ${confirmation}` };
  await saveJSON(join(batch, 'review.json'), report);
  return report;
}

export async function publishDirectory(batchPath, confirmation, client) {
  const batch = await realpath(resolve(batchPath)), prepared = await readJSON(join(batch, 'prepared.json'));
  const { confirmation: expected, ...plan } = prepared;
  if (!confirmation || confirmation !== expected || digest(plan) !== expected) throw fail('Confirm the exact prepared revision shown by ingest.');
  if (!client) throw new EmomoError('IMPORT_ACCESS_REQUIRED', 'Configure maintenance access with emomo-library connect --key-file PRIVATE_FILE.');
  if (plan.importApi && plan.importApi !== client.apiUrl) throw fail('Maintenance endpoint changed. Prepare and confirm a batch for the new endpoint.');
  if (canonical(await readJSON(join(batch, 'annotations.json'))) !== canonical(plan.coverage.annotations)) throw fail('Annotations changed. Re-run ingest and confirm its new revision.');
  // Validate the entire batch before making the first write request.
  for (const row of plan.imports) {
    importMetadata(row.metadata);
    await asset(batch, row.metadata);
  }
  const alreadyOnline = await client.lookup(plan.imports.map(row => row.metadata.sha256));
  const results = plan.imports.filter(row => alreadyOnline.has(row.metadata.sha256)).map(row => ({ sha256: row.metadata.sha256, meme: alreadyOnline.get(row.metadata.sha256), duplicate: true }));
  const pending = plan.imports.filter(row => !alreadyOnline.has(row.metadata.sha256));
  const failed = [];
  const receiptPath = join(batch, 'publication.json');
  for (const row of pending) {
    try {
      const response = await client.upload(row.metadata, await asset(batch, row.metadata));
      results.push({ sha256: row.metadata.sha256, ...response });
    } catch (error) { failed.push({ sha256: row.metadata.sha256, error: publicError(error) }); }
    await saveJSON(receiptPath, { confirmation, apiUrl: client.apiUrl, results, failed, status: 'publishing' });
    if (failed.length) break;
  }
  let verified = false;
  let verificationError;
  try {
    const published = await client.lookup(results.map(row => row.sha256));
    verified = results.every(row => published.get(row.sha256)?.id === row.meme.id);
  } catch (error) { verificationError = publicError(error); }
  const report = { status: failed.length ? 'partial' : verified ? 'published' : 'verification_failed', confirmation, apiUrl: client.apiUrl,
    total: plan.imports.length, completed: results.length, remaining: plan.imports.length - results.length, results, failed,
    indexReadbackVerified: verified, verificationError, publicSearchVerified: false, receipt: receiptPath };
  await saveJSON(receiptPath, report);
  return report;
}

async function verifyPublicSamples(batch, report, env) {
  if (report.status !== 'published' || report.apiUrl !== IMPORT_API || !report.results.length) return report;
  const samples = [];
  try {
    const access = await readAccess(env);
    if (!access || access.apiUrl !== DEFAULT_API_URL) {
      report.publicVerification = 'Configure ordinary Emomo read access to verify public search and download.';
      return report;
    }
    const reader = new EmomoClient({ baseUrl: DEFAULT_API_URL, token: access.token });
    const prepared = await readJSON(join(batch, 'prepared.json'));
    const directory = join(batch, `online-check-${randomUUID()}`);
    for (const result of report.results.slice(0, 3)) {
      const input = prepared.imports.find(row => row.metadata.sha256 === result.sha256);
      // Respect the independent public search and download admission intervals.
      await delay(1100);
      const search = await reader.search(input.queries[0], { limit: 100, category: input.metadata.category });
      const matched = search.results.some(row => row.id === result.meme.id);
      await delay(1100);
      const download = await reader.download(result.meme.id, directory);
      samples.push({ id: result.meme.id, query: input.queries[0], matched, sha256Verified: download.sha256 === result.sha256, path: download.path });
    }
    report.publicSearchVerified = samples.every(row => row.matched && row.sha256Verified);
  } catch (error) { report.publicVerificationError = publicError(error); }
  finally {
    report.publicSamples = samples;
    await saveJSON(report.receipt, report);
  }
  return report;
}

export async function directoryCommand(args, env = process.env) {
  const command = args[0], options = {};
  const offset = command === 'connect' ? 1 : 2;
  for (let i = offset; i < args.length; i += 2) {
    if (!['--dir', '--confirm', '--key-file', '--api-url'].includes(args[i]) || !args[i + 1] || options[args[i]]) throw fail('Invalid maintenance command options.');
    options[args[i]] = args[i + 1];
  }
  if (command === 'connect') {
    if (!options['--key-file'] || options['--dir'] || options['--confirm']) throw fail('Use connect --key-file PRIVATE_FILE [--api-url URL].');
    return connectImport(options['--key-file'], options['--api-url'], env);
  }
  if (!args[1] || args[1].startsWith('--') || options['--key-file'] || options['--api-url']) throw fail('Specify a directory. Configure maintenance access separately.');
  const client = await configuredImportClient(env);
  if (command === 'ingest') {
    if (options['--confirm']) throw fail('Use publish after reviewing the prepared batch.');
    return ingestDirectory(args[1], options['--dir'], client);
  }
  if (options['--dir']) throw fail('Use publish BATCH --confirm REVISION.');
  const report = await publishDirectory(args[1], options['--confirm'], client);
  return verifyPublicSamples(resolve(args[1]), report, env);
}
