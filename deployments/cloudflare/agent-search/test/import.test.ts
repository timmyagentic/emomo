import assert from 'node:assert/strict';
import { test } from 'node:test';
import { readFile, mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createHash } from 'node:crypto';
import { build } from 'esbuild';
import { Miniflare, FormData, Response as MFResponse } from 'miniflare';
// @ts-expect-error Dependency-free Node maintenance client is exercised through its real HTTP encoding.
import { ImportClient } from '../../../../cli/src/import-client.js';
import { indexRow, importSQL } from '../src/metadata.js';

const PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/l9sAAAAASUVORK5CYII=', 'base64');
const hash = (bytes: Buffer | string) => createHash('sha256').update(bytes).digest('hex');
const token = `emomo_import_${'a'.repeat(64)}`;
const metadata = { sha256: hash(PNG), image_info: { width: 1, height: 1, format: 2 }, category: 'usable', description: '猫咪开心', ocr_text: '', text_presence: 3, tags: ['猫', '开心'], search_aliases: ['高兴'] };

test('maintenance API requires full metadata and isolated credentials, persists R2 then searchable D1 and is retry-safe', async t => {
  const bundle = await build({ entryPoints: ['src/import.ts'], write: false, bundle: true, format: 'esm', platform: 'browser', target: 'es2022', external: ['node:buffer', 'cloudflare:workers'] });
  let outbound = 0;
  const mf = new Miniflare({ modules: true, script: bundle.outputFiles[0].text, compatibilityDate: '2026-07-30', compatibilityFlags: ['nodejs_compat'],
    bindings: { IMPORT_KEY_SHA256: hash(token), IMAGE_BASE_URL: 'https://images.example' }, d1Databases: { DB: 'imports' }, r2Buckets: { IMAGES: 'images' },
    outboundService: () => { outbound++; return new MFResponse('', { status: 502 }); } });
  t.after(() => mf.dispose());
  const db = await mf.getD1Database('DB');
  const bucket = (await mf.getBindings<{ IMAGES: R2Bucket }>()).IMAGES;
  const schema = await readFile('migrations/0001_text_index.sql', 'utf8');
  await db.batch(schema.split('-- statement').map(s => s.trim()).filter(Boolean).map(s => db.prepare(s)));
  await db.prepare(await readFile('migrations/0002_import_hash.sql', 'utf8')).run();
  await db.prepare(importSQL(indexRow({ meme: { id: 'old', storage_key: 'old.png' }, annotation: { meme_id: 'old', description: '已有表情' } }))).run();
  const old = await db.prepare("SELECT * FROM memes WHERE id='old'").first();
  const upload = (meta: object, auth = token, data = PNG, origin = false) => {
    const form = new FormData(); form.set('metadata', JSON.stringify(meta)); form.set('image', new Blob([data]), 'image.png');
    return mf.dispatchFetch('https://imports.test/admin/v1/imports', { method: 'POST', headers: { authorization: `Bearer ${auth}`, ...(origin ? { origin: 'https://browser.example' } : {}) }, body: form });
  };
  assert.equal((await upload(metadata, `emomo_reader_${'a'.repeat(64)}`)).status, 401);
  assert.equal((await upload(metadata, token, PNG, true)).status, 403);
  assert.equal((await upload({ ...metadata, tags: [] })).status, 400);
  assert.equal((await upload({ ...metadata, image_info: { width: 99, height: 1, format: 2 } })).status, 400);
  assert.equal((await upload({ ...metadata, sha256: 'b'.repeat(64) })).status, 400);
  assert.equal((await upload(metadata, token, PNG.subarray(0, 33))).status, 400);
  assert.equal((await bucket.list()).objects.length, 0);
  const responses = await Promise.all([upload(metadata), upload(metadata), upload(metadata)]);
  for (const response of responses) assert.equal(response.status, 200, await response.clone().text());
  const receipts = await Promise.all(responses.map(response => response.json() as Promise<{meme:{id:string}; duplicate:boolean}>));
  assert.equal(receipts.filter(row => !row.duplicate).length, 1);
  const id = receipts[0].meme.id;
  assert.equal((await db.prepare('SELECT count(*) AS n FROM memes').first<{n:number}>())!.n, 2);
  assert.deepEqual(await db.prepare("SELECT * FROM memes WHERE id='old'").first(), old);
  assert.equal((await db.prepare("SELECT m.id FROM meme_fts JOIN memes m ON m.rowid=meme_fts.rowid WHERE meme_fts MATCH '猫'").first<{id:string}>())!.id, id);
  const object = await bucket.get(`collections/reviewed/${metadata.sha256}.png`);
  assert.deepEqual(Buffer.from(await object!.arrayBuffer()), PNG);
  const retry = await upload({ ...metadata, description: 'Do not overwrite' });
  assert.equal((await retry.json() as {description:string}).description, metadata.description);
  const lookup = await mf.dispatchFetch('https://imports.test/admin/v1/lookup', { method: 'POST', headers: { authorization: `Bearer ${token}` }, body: JSON.stringify({ sha256: [metadata.sha256] }) });
  assert.equal(lookup.status, 200);
  assert.equal((await lookup.json() as {existing:{id:string}[]}).existing[0].id, id);
  // Inject a D1 failure after the object has landed, then retry the same image.
  await db.prepare('DELETE FROM memes WHERE id = ?').bind(id).run();
  await db.prepare("CREATE TRIGGER fail_import BEFORE INSERT ON memes BEGIN SELECT RAISE(ABORT, 'injected failure'); END").run();
  assert.equal((await upload(metadata)).status, 503);
  assert.equal((await db.prepare('SELECT count(*) AS n FROM memes').first<{n:number}>())!.n, 1);
  assert.ok(await bucket.head(`collections/reviewed/${metadata.sha256}.png`));
  await db.prepare('DROP TRIGGER fail_import').run();
  assert.equal((await upload(metadata)).status, 200);
  assert.deepEqual(await db.prepare("SELECT * FROM memes WHERE id='old'").first(), old);
  const client = new ImportClient('https://imports.test/admin/v1', token, async (url: string, options: RequestInit) => {
    const request = new Request(url, options);
    const response = await mf.dispatchFetch(url, { method: request.method, headers: Object.fromEntries(request.headers), body: request.method === 'POST' ? new Uint8Array(await request.arrayBuffer()) : undefined });
    return new Response(await response.arrayBuffer(), { status: response.status, headers: Object.fromEntries(response.headers) });
  });
  assert.equal((await client.lookup([metadata.sha256])).get(metadata.sha256).id, id);
  assert.equal((await client.upload(metadata, PNG)).duplicate, true);
  const candidates = await client.search('猫 开心', { limit: 3 });
  assert.equal(candidates.results[0].id, id);
  const directory = await mkdtemp(join(tmpdir(), 'emomo-maintenance-image-'));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const downloaded = await client.download(id, directory);
  assert.deepEqual(await readFile(downloaded.path), PNG);
  const search = await mf.dispatchFetch('https://imports.test/admin/v1/search', { method: 'POST', headers: { authorization: `Bearer ${token}`, 'content-type': 'application/json' }, body: JSON.stringify({ query: '猫 开心', top_k: 3 }) });
  assert.equal(search.status, 200); assert.equal(search.headers.get('cache-control'), 'no-store');
  assert.equal((await search.json() as {results:{meme:{id:string}}[]}).results[0].meme.id, id);
  const original = await mf.dispatchFetch(`https://imports.test/admin/v1/images/${metadata.sha256}`, { headers: { authorization: `Bearer ${token}` } });
  assert.equal(original.status, 200); assert.deepEqual(Buffer.from(await original.arrayBuffer()), PNG);
  const denied = await mf.dispatchFetch(`https://imports.test/admin/v1/images/${metadata.sha256}`, { headers: { authorization: `Bearer emomo_reader_${'b'.repeat(64)}` } });
  assert.equal(denied.status, 401);
  const invalidSearch = await mf.dispatchFetch('https://imports.test/admin/v1/search', { method: 'POST', headers: { authorization: `Bearer ${token}`, 'content-type': 'application/json' }, body: JSON.stringify({ query: '猫', top_k: 1000 }) });
  assert.equal(invalidSearch.status, 400);
  assert.equal(outbound, 0);
});
