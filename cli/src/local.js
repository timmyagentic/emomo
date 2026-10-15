import { publishDirectory } from './publish-directory.js';
import { createHash } from 'node:crypto';
import { readFile, writeFile, mkdir, mkdtemp, rm, realpath } from 'node:fs/promises';
import { join, resolve, relative, isAbsolute, dirname } from 'node:path';
import { pathToFileURL, fileURLToPath } from 'node:url';
import { EmomoError } from './error.js';
import { identifyImage, saveImage } from './client.js';
import { normalize, tokens, indexTerms, keywordPlan, subjectFilter } from './keyword-search.js';
export { normalize, tokens } from './keyword-search.js';

const MAX_IMAGE = 25 * 1024 * 1024;
const MAX_METADATA = 32 * 1024 * 1024;
const TYPES = new Set(['usable', 'object_sticker']);
const array = value => Array.isArray(value) && value.every(x => typeof x === 'string');
const plain = value => value && typeof value === 'object' && !Array.isArray(value);
const bad = () => new EmomoError('INVALID_CATALOG', 'The local catalog is invalid. Rebuild it from reviewed metadata.');
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
async function sqlite() {
  try { return (await import('node:sqlite')).DatabaseSync; }
  catch { throw new EmomoError('LOCAL_RUNTIME_UNSUPPORTED', 'Local catalogs require Node.js 22.13 or newer with SQLite FTS5.'); }
}
async function jsonFile(path) {
  const bytes = await readFile(path);
  if (bytes.length > MAX_METADATA) throw bad();
  try { return JSON.parse(bytes.toString('utf8')); } catch { throw bad(); }
}
function vocabulary(value = {}) {
  const intents = value.intents ?? {}, subjects = value.subjects ?? {}, gaps = value.gaps ?? {};
  if (![intents, subjects, gaps].every(plain) || !Object.values(intents).every(array) || !Object.values(subjects).every(array) || !Object.values(gaps).every(x => typeof x === 'string')) throw bad();
  if (value.searchMode !== undefined && value.searchMode !== 'facets') throw bad();
  return { intents, subjects, gaps, ...(value.searchMode ? { searchMode: value.searchMode } : {}) };
}
function record(row) {
  if (!plain(row) || !['string', 'number'].includes(typeof row.id) || !/^[\w-]{1,150}$/.test(String(row.id)) || !TYPES.has(row.category) || typeof row.file_path !== 'string' || !isAbsolute(row.file_path) || !/^[a-f0-9]{64}$/.test(row.sha256 ?? '') || typeof row.description !== 'string') throw bad();
  for (const name of ['subjects', 'intent_tags', 'scenarios', 'query_aliases', 'content_flags']) if (row[name] !== undefined && !array(row[name])) throw bad();
  for (const n of ['width', 'height', 'frames']) if (!Number.isSafeInteger(row[n]) || row[n] < 1) throw bad();
  if (!['image', 'preview', 'animation'].includes(row.media_kind) || (row.frames > 1) !== (row.media_kind === 'animation')) throw bad();
  if (row.media_kind === 'animation' && row.preview_only) throw bad();
  if (row.image_text !== undefined && typeof row.image_text !== 'string') throw bad();
  if (row.canonical_id !== undefined && (!/^[\w-]{1,150}$/.test(row.canonical_id) || typeof row.searchable !== 'boolean' || !['verified_visible_text','normalized_repetition','partially_illegible'].includes(row.ocr_review))) throw bad();
  if (row.ocr_review === 'partially_illegible' && row.image_text) throw bad();
  return { ...(row.canonical_id !== undefined ? { canonicalId: `local-${row.canonical_id}`, searchable: row.searchable, ocrReview: row.ocr_review } : {}), id: `local-${row.id}`, sourceId: String(row.id), description: row.description, imageText: row.image_text ?? '', category: row.category,
    tags: row.intent_tags ?? [], subjects: row.subjects ?? [], scenarios: row.scenarios ?? [], aliases: row.query_aliases ?? [], contentFlags: row.content_flags ?? [],
    quality: row.quality ?? '', mediaKind: row.media_kind, previewOnly: row.media_kind === 'preview',
    image: { width: row.width, height: row.height, frames: row.frames }, sha256: row.sha256, publicReleaseClearance: row.public_release_clearance ?? 'UNVERIFIED', asset: '' };
}

export async function importCatalog(source, directory, vocabularyPath) {
  const rows = await jsonFile(resolve(source));
  return importRows(rows, directory, vocabularyPath ? await jsonFile(resolve(vocabularyPath)) : {});
}

export async function importRows(rows, directory, lexiconInput = {}) {
  if (!Array.isArray(rows) || !rows.length || rows.length > 100000) throw bad();
  const lexicon = vocabulary(lexiconInput);
  const eligible = rows.filter(row => TYPES.has(row?.category));
  if (!eligible.length) throw bad();
  // Preflight all rows before creating a staging directory. Input is private data, never bundled with the npm package.
  const items = eligible.map(record);
  if (new Set(items.map(r => r.id)).size !== items.length) throw bad();
  const byId = new Map(items.map(r => [r.id, r]));
  for (const r of items) if (r.canonicalId !== undefined) {
    const primary = byId.get(r.canonicalId);
    if (!primary || primary.canonicalId !== primary.id || !primary.searchable || r.searchable !== (r.id === r.canonicalId)) throw bad();
  }
  const target = resolve(directory), parent = dirname(target);
  await mkdir(parent, { recursive: true });
  const stage = await mkdtemp(join(parent, '.emomo-catalog-'));
  let db;
  try {
    await mkdir(join(stage, 'assets'));
    for (let i = 0; i < items.length; i++) {
      const bytes = await readFile(eligible[i].file_path);
      if (bytes.length > MAX_IMAGE || hash(bytes) !== items[i].sha256) throw new EmomoError('IMAGE_INTEGRITY_ERROR', 'A source image is too large or no longer matches its reviewed hash.');
      const format = identifyImage(bytes, { allowGif: true });
      if (format.extension === 'gif' && items[i].image.frames < 1) throw bad();
      items[i].image.format = format.extension === 'jpg' ? 'jpeg' : format.extension;
      items[i].asset = `assets/${items[i].sha256}.${format.extension}`;
      await writeFile(join(stage, items[i].asset), bytes, { mode: 0o600, flag: 'wx' }).catch(error => { if (error.code !== 'EEXIST') throw error; });
    }
    const Database = await sqlite();
    db = new Database(join(stage, 'search.sqlite3'));
    db.exec('CREATE TABLE items(id INTEGER PRIMARY KEY, category TEXT NOT NULL, media TEXT NOT NULL, text_present INTEGER NOT NULL, data TEXT NOT NULL); CREATE VIRTUAL TABLE search USING fts5(caption,description,subjects,intents,aliases); BEGIN;');
    const insert = db.prepare('INSERT INTO items VALUES(?,?,?,?,?)');
    const index = db.prepare('INSERT INTO search(rowid,caption,description,subjects,intents,aliases) VALUES(?,?,?,?,?,?)');
    items.forEach((r, i) => {
      insert.run(i + 1, r.category, r.mediaKind, r.imageText ? 1 : 0, JSON.stringify(r));
      if (r.searchable !== false) index.run(i + 1, ...[r.imageText, r.description, r.subjects.join(' '), r.tags.join(' '), r.aliases.join(' ')].map(v => tokens(v).join(' ')));
    });
    db.exec('COMMIT'); db.close(); db = undefined;
    await writeFile(join(stage, 'manifest.json'), JSON.stringify({ schemaVersion: 1, format: 'emomo-local-catalog', items: items.length, name: 'Reviewed local gallery', lexicon, indexSha256: hash(await readFile(join(stage, 'search.sqlite3'))) }) + '\n', { mode: 0o600 });
    // Exclusive reservation protects existing catalogs, including empty user directories.
    try { await publishDirectory(stage, target, 'manifest.json'); } catch (e) { if (e.code === 'EEXIST') throw new EmomoError('CATALOG_EXISTS', 'The destination catalog already exists. Choose a new directory.'); throw e; }
    return { path: target, totalMemes: String(items.length), usable: items.filter(r => r.category === 'usable').length, objects: items.filter(r => r.category === 'object_sticker').length, portable: true, modelsUsed: [] };
  } finally {
    db?.close();
    await rm(stage, { recursive: true, force: true });
  }
}

export class LocalCatalog {
  static async open(path) {
    const client = new LocalCatalog();
    try {
      client.root = await realpath(resolve(path));
      const manifest = await jsonFile(join(client.root, 'manifest.json'));
      if (manifest?.schemaVersion !== 1 || manifest.format !== 'emomo-local-catalog' || !Number.isSafeInteger(manifest.items) || !/^[a-f0-9]{64}$/.test(manifest.indexSha256 ?? '')) throw bad();
      client.lexicon = vocabulary(manifest.lexicon);
      const dbPath = await client.contained('search.sqlite3');
      if (hash(await readFile(dbPath)) !== manifest.indexSha256) throw bad();
      const Database = await sqlite();
      client.db = new Database(dbPath, { readOnly: true, enableLoadExtension: false });
      if (client.db.prepare('SELECT count(*) AS n FROM items').get().n !== manifest.items) throw bad();
      return client;
    } catch (error) {
      client.close();
      if (error instanceof EmomoError) throw error;
      throw new EmomoError('LOCAL_CATALOG_UNAVAILABLE', 'The configured local catalog cannot be opened. Mount its disk or select a valid catalog; no remote fallback was used.');
    }
  }
  close() { this.searchDb?.close(); this.searchDb = undefined; this.db?.close(); this.db = undefined; }
  async contained(asset) {
    if (typeof asset !== 'string' || isAbsolute(asset) || asset.split(/[\\/]/).includes('..')) throw bad();
    const path = await realpath(join(this.root, asset));
    const rel = relative(this.root, path);
    if (!rel || rel === '..' || rel.startsWith(`..${process.platform === 'win32' ? '\\' : '/'}`) || isAbsolute(rel)) throw bad();
    return path;
  }
  async candidate(r, score = 0) {
    if (!plain(r) || typeof r.id !== 'string' || !/^[\w-]+$/.test(r.id) || typeof r.asset !== 'string' || isAbsolute(r.asset) || r.asset.split(/[\\/]/).includes('..')) throw bad();
    const localPath = await this.checkedImage(r);
    return { ...(r.canonicalId ? { canonicalId: r.canonicalId, searchable: r.searchable, ocrReview: r.ocrReview } : {}), id: r.id, url: pathToFileURL(localPath).href, description: r.description, category: r.category, tags: r.tags, score,
      textPresence: r.imageText ? 'with_text' : 'unknown', image: r.image, imageText: r.imageText, subjects: r.subjects, scenarios: r.scenarios,
      mediaKind: r.mediaKind, previewOnly: r.previewOnly, quality: r.quality, contentFlags: r.contentFlags, publicReleaseClearance: r.publicReleaseClearance };
  }
  async search(query, { limit = 8, category, textPresence, profile, collection, media, subject, intent, includeObjects = false } = {}) {
    if (profile && profile !== 'keyword' || collection && collection !== 'local') throw new EmomoError('INVALID_ARGUMENT', 'Local catalogs support the keyword profile and local collection only.');
    const empty = reason => ({ query, expandedQuery: '', total: 0, results: [], reason, source: 'local' });
    if (textPresence === 3) return empty('text_absence_unverified');
    const gap = Object.entries(this.lexicon.gaps).find(([term]) => normalize(query).includes(normalize(term)));
    if (gap) return { ...empty('gallery_gap'), detail: gap[1] };
    let plan;
    try { plan = keywordPlan(query, this.lexicon); }
    catch { return empty('no_match'); }
    if (!this.searchDb) {
      // Existing catalog files stay read-only. One ephemeral index normalizes old
      // and reviewed catalogs to the hosted index without migrating user data.
      const Database = await sqlite();
      this.searchDb = new Database(':memory:');
      this.searchDb.exec('CREATE TABLE records(id INTEGER PRIMARY KEY, tags TEXT, category TEXT, media TEXT, text_present INTEGER, data TEXT); CREATE VIRTUAL TABLE keywords USING fts5(caption,description,tags); BEGIN;');
      const record = this.searchDb.prepare('INSERT INTO records VALUES(?,?,?,?,?,?)');
      const index = this.searchDb.prepare('INSERT INTO keywords(rowid,caption,description,tags) VALUES(?,?,?,?)');
      for (const item of this.db.prepare('SELECT id,data FROM items').all()) {
        const r = JSON.parse(item.data);
        if (r.searchable === false) continue;
        const tags = [...new Set([...r.tags, ...r.subjects])];
        record.run(item.id, JSON.stringify(tags), r.category, r.mediaKind, r.imageText ? 1 : 0, item.data);
        index.run(item.id, indexTerms(r.imageText), indexTerms(r.description), indexTerms([...tags, ...r.aliases, ...r.scenarios].join(' ')));
      }
      this.searchDb.exec('COMMIT');
    }
    const filters = subjectFilter(plan, 'r.tags');
    const clauses = ['keywords MATCH ?'], values = [plan.expression];
    if (category) { clauses.push('r.category = ?'); values.push(category); }
    else if (!includeObjects) clauses.push("r.category <> 'object_sticker'");
    if (media) { clauses.push('r.media = ?'); values.push(media); }
    if (textPresence === 2 || textPresence === 1) { clauses.push('r.text_present = ?'); values.push(textPresence === 2 ? 1 : 0); }
    for (const [field, value] of [['subjects', subject], ['tags', intent]]) if (value) {
      clauses.push(`EXISTS (SELECT 1 FROM json_each(r.data, '$.${field}') WHERE value = ?)`); values.push(value);
    }
    const statement = this.searchDb.prepare(`SELECT r.data, bm25(keywords,4,1,2) AS rank FROM keywords JOIN records r ON r.id=keywords.rowid WHERE ${clauses.join(' AND ')}${filters.sql} ORDER BY rank ASC,json_extract(r.data,'$.id') ASC`);
    const lookup = expression => statement.all(expression, ...values.slice(1), ...filters.values);
    let rows = lookup(plan.literalExpression);
    if (!rows.length && plan.expression !== plan.literalExpression) rows = lookup(plan.expression);
    const results = [];
    for (const row of rows.slice(0, limit)) {
      const score = Math.max(0, -row.rank) / (1 + Math.max(0, -row.rank));
      const keys = new Set(plan.groups.map(group => group.key));
      results.push({ ...await this.candidate(JSON.parse(row.data), score), match: {
        terms: plan.groups.map(group => group.key),
        intents: Object.keys(this.lexicon.intents).filter(key => keys.has(key)),
        subjects: plan.groups.filter(group => group.subject).map(group => group.key),
      } });
    }
    return { query, expandedQuery: '', total: rows.length, results, reason: rows.length ? 'matched' : 'no_match', source: 'local' };
  }
  item(id) {
    const row = this.db.prepare('SELECT data FROM items WHERE json_extract(data,\'$.id\') = ?').get(id);
    if (!row) throw new EmomoError('NOT_FOUND', 'This image ID is not in the selected local catalog.');
    return JSON.parse(row.data);
  }
  async checkedImage(item) {
    const localPath = await this.contained(item.asset).catch(error => { if (error instanceof EmomoError) throw error; throw new EmomoError('LOCAL_IMAGE_UNAVAILABLE', 'The original image is missing from the local catalog.'); });
    const bytes = await readFile(localPath);
    if (bytes.length > MAX_IMAGE || hash(bytes) !== item.sha256) throw new EmomoError('IMAGE_INTEGRITY_ERROR', 'The local image no longer matches its catalog hash.');
    identifyImage(bytes, { allowGif: true });
    return localPath;
  }
  async get(id) {
    const item = this.item(id);
    const versions = item.canonicalId ? this.db.prepare("SELECT data FROM items WHERE json_extract(data,'$.canonicalId') = ? ORDER BY id").all(item.canonicalId).map(row => { const r = JSON.parse(row.data); return { id: r.id, canonicalId: r.canonicalId, sha256: r.sha256, image: r.image }; }) : undefined;
    const meme = await this.candidate(item);
    return { meme: { ...meme, localPath: fileURLToPath(meme.url), sha256: item.sha256, ...(versions ? { versions } : {}) } };
  }
  async download(id, directory) {
    const item = this.item(id), { meme } = await this.get(id);
    const bytes = await readFile(meme.localPath);
    if (bytes.length > MAX_IMAGE || hash(bytes) !== item.sha256) throw new EmomoError('IMAGE_INTEGRITY_ERROR', 'The local image no longer matches its catalog hash.');
    const format = identifyImage(bytes, { allowGif: true });
    return saveImage(bytes, format, id, directory, meme.url);
  }
  async categories() { const rows = this.db.prepare('SELECT DISTINCT category FROM items ORDER BY category').all(); return { categories: rows.map(r => r.category), total: rows.length }; }
  async stats() {
    const counts = this.db.prepare('SELECT category,count(*) AS n FROM items GROUP BY category').all();
    return { totalMemes: String(counts.reduce((n, r) => n + r.n, 0)), defaultSearchMemes: String(this.db.prepare("SELECT count(*) AS n FROM items WHERE category='usable' AND coalesce(json_extract(data,'$.searchable'),1) = 1").get().n), totalCategories: counts.length, availableProfiles: ['keyword'], availableCollections: ['local'], source: 'local', modelsUsed: [] };
  }
  async verify() {
    const items = this.db.prepare('SELECT data FROM items').all();
    for (const row of items) {
      const r = JSON.parse(row.data), path = await this.contained(r.asset).catch(error => { if (error instanceof EmomoError) throw error; throw new EmomoError('LOCAL_IMAGE_UNAVAILABLE', 'A local image is missing. Restore the selected catalog.'); });
      const bytes = await readFile(path);
      if (hash(bytes) !== r.sha256) throw new EmomoError('IMAGE_INTEGRITY_ERROR', 'A local image failed the integrity check.');
      identifyImage(bytes, { allowGif: true });
    }
    return { source: 'local', catalogPath: this.root, reachable: true, checkedImages: items.length, stats: await this.stats() };
  }
}
