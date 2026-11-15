import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtemp, mkdir, writeFile, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createHash } from 'node:crypto';
import { ingestDirectory, publishDirectory } from '../src/directory-import.js';
import { staticImageInfo } from '../src/image-info.js';
import { importMetadata } from '../src/import-metadata.js';
import { ImportClient } from '../src/import-client.js';
import { PNG } from './helpers.js';
const sha256 = createHash('sha256').update(PNG).digest('hex');
const metadata = { sha256, image_info: { width: 1, height: 1, format: 2 }, category: 'usable', description: '猫咪开心', ocr_text: '', text_presence: 3, tags: ['猫', '开心'], search_aliases: [] };

test('directory to reviewed catalog to confirmed import preserves bytes and exact duplicates', async t => {
  const root = await mkdtemp(join(tmpdir(), 'emomo-directory-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const source = join(root, 'source'), batch = join(root, 'batch');
  await mkdir(source); await writeFile(join(source, '猫.png'), PNG); await writeFile(join(source, 'copy.png'), PNG);
  await writeFile(join(source, 'bad.jpg'), 'invalid');
  const pending = await ingestDirectory(source, batch);
  assert.equal(pending.status, 'needs_annotation'); assert.equal(pending.exactDuplicates, 1); assert.equal(pending.pending.length, 1); assert.equal(pending.rejected.length, 1);
  let annotations = JSON.parse(await readFile(pending.annotations, 'utf8'));
  annotations[0] = { ...annotations[0], ...metadata, decision: 'keep', queries: ['猫 开心'] };
  await writeFile(pending.annotations, JSON.stringify(annotations));
  const ready = await ingestDirectory(source, batch);
  assert.equal(ready.status, 'ready'); assert.equal(ready.newImages, 1); assert.ok(ready.checks.every(row => row.passed));
  assert.equal((await ingestDirectory(source, batch)).confirmation, ready.confirmation);
  const writes = [], stored = new Map();
  const client = { apiUrl: 'https://imports.timmyagentic.si/admin/v1', lookup: async () => stored, upload: async (value, bytes) => {
    assert.deepEqual(bytes, PNG); writes.push(value);
    const meme = { id: 'public-cat', content_hash: sha256 }; const duplicate = stored.has(sha256); stored.set(sha256, meme);
    return { meme, duplicate };
  } };
  await assert.rejects(publishDirectory(batch, 'wrong-confirmation', client)); assert.equal(writes.length, 0);
  const result = await publishDirectory(batch, ready.confirmation, client);
  assert.equal(result.status, 'published'); assert.equal(result.indexReadbackVerified, true); assert.equal(result.completed, 1);
  assert.equal((await publishDirectory(batch, ready.confirmation, client)).results[0].duplicate, true);
  await writeFile(join(batch, 'assets', sha256), 'tampered');
  await assert.rejects(publishDirectory(batch, ready.confirmation, client)); assert.equal(writes.length, 1);
  assert.deepEqual(await readFile(join(source, '猫.png')), PNG);
});

test('invalid or pending annotations cannot publish, remote exact duplicates avoid annotation', async t => {
  const root = await mkdtemp(join(tmpdir(), 'emomo-existing-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const source = join(root, 'source'), batch = join(root, 'batch');
  await mkdir(source); await writeFile(join(source, 'cat.png'), PNG);
  const existing = new Map([[sha256, { id: 'historical-id', content_hash: sha256 }]]);
  const ready = await ingestDirectory(source, batch, { apiUrl: 'https://imports.example/admin/v1', lookup: async () => existing });
  assert.equal(ready.status, 'ready'); assert.equal(ready.newImages, 0); assert.equal(ready.alreadyOnline, 1); assert.equal(ready.onlineSimilarCandidatesChecked, false);
  const pending = await ingestDirectory(source, join(root, 'pending'));
  await assert.rejects(publishDirectory(pending.batch, 'anything', {}));
  const annotations = JSON.parse(await readFile(pending.annotations, 'utf8'));
  annotations[0] = { ...annotations[0], decision: 'keep' };
  await writeFile(pending.annotations, JSON.stringify(annotations));
  await assert.rejects(ingestDirectory(source, pending.batch));
});

test('container checks reject truncation, GIF and APNG; explicit text status is required', () => {
  assert.deepEqual(staticImageInfo(PNG), { width: 1, height: 1, format: 2 });
  assert.throws(() => staticImageInfo(PNG.subarray(0, 33)));
  assert.throws(() => staticImageInfo(Buffer.from('GIF89a')));
  const animated = Buffer.from(PNG); animated.write('acTL', 37, 'ascii'); assert.throws(() => staticImageInfo(animated));
  assert.throws(() => importMetadata({ ...metadata, text_presence: 0 }));
  assert.throws(() => importMetadata({ ...metadata, ocr_text: 'visible text' }));
  assert.throws(() => importMetadata({ ...metadata, tags: [] }));
});

test('maintenance credentials never follow redirects and upload receipts must match', async () => {
  const token = `emomo_import_${'a'.repeat(64)}`;
  let calls = 0;
  const client = new ImportClient('https://imports.example/admin/v1', token, async (url, options) => {
    calls++; assert.equal(options.redirect, 'manual');
    return new Response('', { status: 302, headers: { location: 'https://elsewhere.example' } });
  });
  await assert.rejects(client.upload(metadata, PNG), { code: 'IMPORT_REDIRECT' }); assert.equal(calls, 1);
  assert.throws(() => new ImportClient('http://example.com', token));
  const wrong = new ImportClient('https://imports.example/admin/v1', token, async () => Response.json({ meme: { content_hash: 'wrong' }, duplicate: false }));
  await assert.rejects(wrong.upload(metadata, PNG), { code: 'IMPORT_RESPONSE_INVALID' });
});

test('lookup failure leaves no half-created batch; failed publication is resumable and edits invalidate approval', async t => {
  const root = await mkdtemp(join(tmpdir(), 'emomo-retry-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const source = join(root, 'source'), batch = join(root, 'batch');
  await mkdir(source); await writeFile(join(source, 'cat.png'), PNG);
  const client = { apiUrl: 'https://imports.timmyagentic.si/admin/v1', lookup: async () => { throw new Error('offline'); } };
  await assert.rejects(ingestDirectory(source, batch, client));
  client.lookup = async () => new Map();
  const pending = await ingestDirectory(source, batch, client);
  const annotations = JSON.parse(await readFile(pending.annotations, 'utf8'));
  annotations[0] = { ...annotations[0], ...metadata, decision: 'keep', queries: ['猫 开心'] };
  await writeFile(pending.annotations, JSON.stringify(annotations));
  const ready = await ingestDirectory(source, batch, client);
  client.upload = async () => { throw new Error('connection lost'); };
  const failed = await publishDirectory(batch, ready.confirmation, client);
  assert.equal(failed.status, 'partial'); assert.equal(failed.remaining, 1);
  const meme = { id: 'resumed', content_hash: sha256 };
  client.upload = async () => ({ meme, duplicate: true });
  client.lookup = async () => new Map([[sha256, meme]]);
  assert.equal((await publishDirectory(batch, ready.confirmation, client)).status, 'published');
  annotations[0].description = 'changed';
  await writeFile(pending.annotations, JSON.stringify(annotations));
  await assert.rejects(publishDirectory(batch, ready.confirmation, client));
});
