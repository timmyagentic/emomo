import { createHash } from 'node:crypto';
import { readFile, writeFile, mkdir, mkdtemp, rename, rm, realpath } from 'node:fs/promises';
import { join, resolve, relative, isAbsolute, dirname } from 'node:path';
import { pathToFileURL } from 'node:url';
import { EmomoError } from './error.js';
import { identifyImage, saveImage } from './client.js';
import { refinedSearch } from './refined-search.js';

const MAX_IMAGE = 25 * 1024 * 1024;
const MAX_METADATA = 32 * 1024 * 1024;
const TYPES = new Set(['usable', 'object_sticker']);
const array = value => Array.isArray(value) && value.every(x => typeof x === 'string');
const plain = value => value && typeof value === 'object' && !Array.isArray(value);
const bad = () => new EmomoError('INVALID_CATALOG', 'The local catalog is invalid. Rebuild it from reviewed metadata.');
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
export const normalize = text => text.normalize('NFKC').toLowerCase();
export function tokens(text) {
  const value = normalize(text);
  const out = new Set(value.match(/[a-z0-9]+/g) ?? []);
  for (const part of value.match(/[\u3400-\u9fff]+/g) ?? []) {
    if (part.length === 1) out.add(part);
    else for (let i = 0; i < part.length - 1; i++) out.add(part.slice(i, i + 2));
  }
  return [...out].sort();
}
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
  let reserved = false, db;
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
    try { await mkdir(target); reserved = true; } catch (e) { if (e.code === 'EEXIST') throw new EmomoError('CATALOG_EXISTS', 'The destination catalog already exists. Choose a new directory.'); throw e; }
    await rename(stage, target); reserved = false;
    return { path: target, totalMemes: String(items.length), usable: items.filter(r => r.category === 'usable').length, objects: items.filter(r => r.category === 'object_sticker').length, portable: true, modelsUsed: [] };
  } finally {
    db?.close();
    if (reserved) await rm(target, { recursive: true, force: true });
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
  close() { this.db?.close(); this.db = undefined; }
  async contained(asset) {
    if (typeof asset !== 'string' || isAbsolute(asset) || asset.split(/[\\/]/).includes('..')) throw bad();
    const path = await realpath(join(this.root, asset));
    const rel = relative(this.root, path);
    if (!rel || rel === '..' || rel.startsWith(`..${process.platform === 'win32' ? '\\' : '/'}`) || isAbsolute(rel)) throw bad();
    return path;
  }
  candidate(r, score = 0) {
    if (!plain(r) || typeof r.id !== 'string' || !/^[\w-]+$/.test(r.id) || typeof r.asset !== 'string' || isAbsolute(r.asset) || r.asset.split(/[\\/]/).includes('..')) throw bad();
    return { ...(r.canonicalId ? { canonicalId: r.canonicalId, searchable: r.searchable, ocrReview: r.ocrReview } : {}), id: r.id, url: pathToFileURL(join(this.root, r.asset)).href, description: r.description, category: r.category, tags: r.tags, score,
      textPresence: r.imageText ? 'with_text' : 'unknown', image: r.image, imageText: r.imageText, subjects: r.subjects, scenarios: r.scenarios,
      mediaKind: r.mediaKind, previewOnly: r.previewOnly, quality: r.quality, contentFlags: r.contentFlags, publicReleaseClearance: r.publicReleaseClearance };
  }
  subjectTerms(query) {
    let text = normalize(query), found = [];
    for (const name of ['熊猫', '哆啦A梦', '天线宝宝', '负鼠', '仓鼠']) {
      const terms = this.lexicon.subjects[name] ?? [];
      if (terms.some(t => text.includes(normalize(t)))) {
        found.push(name);
        for (const t of terms) text = text.replaceAll(normalize(t), ' ');
      }
    }
    for (const [name, terms] of Object.entries(this.lexicon.subjects)) if (!found.includes(name) && terms.some(t => text.includes(normalize(t)))) found.push(name);
    return found;
  }
  async search(query, { limit = 8, category, textPresence, profile, collection, media, subject, intent, includeObjects = false } = {}) {
    if (profile && profile !== 'keyword' || collection && collection !== 'local') throw new EmomoError('INVALID_ARGUMENT', 'Local catalogs support the keyword profile and local collection only.');
    if (this.lexicon.searchMode === 'facets') return refinedSearch(this, query, { limit, category, textPresence, media, subject, intent, includeObjects });
    const q = normalize(query.trim());
    const gap = Object.entries(this.lexicon.gaps).find(([term]) => q.includes(normalize(term)));
    if (gap) return { query, expandedQuery: '', total: 0, results: [], reason: 'gallery_gap', detail: gap[1], source: 'local' };
    const intents = Object.entries(this.lexicon.intents).filter(([, aliases]) => aliases.some(a => q.includes(normalize(a)))).map(([name]) => name);
    if (intent && !intents.includes(intent)) intents.push(intent);
    const subjects = this.subjectTerms(q);
    if (subject && !subjects.includes(subject)) subjects.push(subject);
    const ts = tokens([query, ...intents, ...subjects].join(' '));
    if (!ts.length) return { query, expandedQuery: '', total: 0, results: [], reason: 'no_match', source: 'local' };
    const clauses = ['search MATCH ?'], params = [ts.map(t => `"${t}"`).join(' OR ')];
    if (category) { clauses.push('items.category = ?'); params.push(category); }
    else if (!includeObjects) clauses.push("items.category = 'usable'");
    if (media) { clauses.push('items.media = ?'); params.push(media); }
    if (textPresence === 2) clauses.push('items.text_present = 1');
    if (textPresence === 1) clauses.push('items.text_present = 0');
    // No OCR text is treated as unknown, not reliable evidence of text absence.
    if (textPresence === 3) return { query, expandedQuery: '', total: 0, results: [], reason: 'text_absence_unverified', source: 'local' };
    const candidates = this.db.prepare(`SELECT items.data FROM search JOIN items ON items.id=search.rowid WHERE ${clauses.join(' AND ')} ORDER BY bm25(search,7,3,2,5,1) LIMIT 200`).all(...params);
    const raw = new Set(tokens(query)), ranked = [];
    for (const row of candidates) {
      const r = JSON.parse(row.data);
      const matchedIntents = intents.filter(t => r.tags.includes(t)), matchedSubjects = subjects.filter(t => r.subjects.includes(t));
      if (subject && !r.subjects.includes(subject) || intent && !r.tags.includes(intent)) continue;
      const literal = ['imageText', 'description'].filter(field => q.length >= 2 && normalize(r[field]).includes(q));
      const overlap = tokens(r.imageText + ' ' + r.description).filter(t => raw.has(t));
      if (!matchedIntents.length && !matchedSubjects.length && !literal.length && !overlap.length) continue;
      if (!matchedIntents.length && !matchedSubjects.length && !literal.length && q.length >= 4 && overlap.length < 2) continue;
      if (intents.length && !matchedIntents.length && !literal.length || subjects.length && !matchedSubjects.length) continue;
      if (intents.includes('讽刺同意') && !r.tags.includes('讽刺同意') || intents.includes('老板奖励') && !r.tags.includes('老板奖励')) continue;
      const food = (this.lexicon.intents['饭食'] ?? []).filter(a => !['吃饭','饿了'].includes(a) && q.includes(normalize(a)));
      if (food.some(a => !normalize(r.description + ' ' + r.imageText).includes(normalize(a)))) continue;
      let score = matchedIntents.length * 30 + matchedSubjects.length * 12 + literal.length * 35 + Math.min(overlap.length, 8) * 4;
      if (intents.length && matchedIntents.length === intents.length) score += 25;
      score += Math.min(Math.log2(Math.max(r.image.width, r.image.height)), 10) * 0.1;
      ranked.push({ ...this.candidate(r, Math.round(score * 100) / 100), match: { intents: matchedIntents, subjects: matchedSubjects, literalFields: literal, tokens: overlap } });
    }
    ranked.sort((a, b) => b.score - a.score || Number(a.id.slice(6)) - Number(b.id.slice(6)) || a.id.localeCompare(b.id));
    return { query, expandedQuery: [...intents, ...subjects].join(' '), total: ranked.length, results: ranked.slice(0, limit), reason: ranked.length ? 'matched' : 'no_match', source: 'local' };
  }
  item(id) {
    const row = this.db.prepare('SELECT data FROM items WHERE json_extract(data,\'$.id\') = ?').get(id);
    if (!row) throw new EmomoError('NOT_FOUND', 'This image ID is not in the selected local catalog.');
    return JSON.parse(row.data);
  }
  async get(id) {
    const item = this.item(id);
    const localPath = await this.contained(item.asset).catch(error => { if (error instanceof EmomoError) throw error; throw new EmomoError('LOCAL_IMAGE_UNAVAILABLE', 'The original image is missing from the local catalog.'); });
    const bytes = await readFile(localPath);
    if (bytes.length > MAX_IMAGE || hash(bytes) !== item.sha256) throw new EmomoError('IMAGE_INTEGRITY_ERROR', 'The local image no longer matches its catalog hash.');
    identifyImage(bytes, { allowGif: true });
    const versions = item.canonicalId ? this.db.prepare("SELECT data FROM items WHERE json_extract(data,'$.canonicalId') = ? ORDER BY id").all(item.canonicalId).map(row => { const r = JSON.parse(row.data); return { id: r.id, canonicalId: r.canonicalId, sha256: r.sha256, image: r.image }; }) : undefined;
    return { meme: { ...this.candidate(item), localPath, sha256: item.sha256, ...(versions ? { versions } : {}) } };
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
