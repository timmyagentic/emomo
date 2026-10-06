import assert from 'node:assert/strict';
import { before, after, test } from 'node:test';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createServer } from 'node:http';
import { once } from 'node:events';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { build } from 'esbuild';
import { Miniflare, Response as MFResponse } from 'miniflare';
import type { JsonValue } from '@bufbuild/protobuf';
import { indexRow, importSQL } from '../src/metadata.js';

const execute = promisify(execFile);
const PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/l9sAAAAASUVORK5CYII=', 'base64');
const cli = fileURLToPath(new URL('../../../../cli/', import.meta.url).href);
let mf: Miniflare;
let temporary: string;
let script: string;
let outboundCalls = 0;
let bundleInputs: string[];
const options = (enabled: string, imageBase = 'https://r2.emomo.net') => ({
  modules: true,
  script,
  // Latest stable v4 local workerd date; deployment config keeps today's date.
  compatibilityDate: '2026-07-30',
  compatibilityFlags: ['nodejs_compat'],
  bindings: { AGENT_API_ENABLED: enabled, IMAGE_BASE_URL: imageBase },
  d1Databases: { DB: 'local-index' },
  ratelimits: { EMOMO_RATE_LIMITER: { namespace_id: '1002', simple: { limit: 30, period: 60 as const } } },
  // Block every external HTTP request and prove success without one.
  outboundService: () => { outboundCalls++; return new MFResponse('External HTTP forbidden in this test', { status: 502 }); },
});

before(async () => {
  temporary = await mkdtemp(join(tmpdir(), 'emomo-d1-'));
  const bundle = await build({ entryPoints: ['src/index.ts'], write: false, bundle: true, format: 'esm', platform: 'browser', target: 'es2022', metafile: true });
  script = bundle.outputFiles[0].text;
  bundleInputs = Object.keys(bundle.metafile!.inputs);
  mf = new Miniflare(options('true'));
  const db = await mf.getD1Database('DB');
  const schema = await readFile(fileURLToPath(new URL('../migrations/0001_text_index.sql', import.meta.url).href), 'utf8');
  await db.batch(schema.split('-- statement').map(sql => sql.trim()).filter(Boolean).map(sql => db.prepare(sql)));
  const samples: JsonValue[] = [
    { meme: { id: 'cat-1', storage_key: 'cat-1.png', image_info: { width: 1, height: 1, format: 2 }, category: 'reaction' }, annotation: { meme_id: 'cat-1', description: '一只猫咪露出无语嫌弃的表情', ocr_text: '' } },
    { meme: { id: 'boss-1', storage_key: 'boss-1.png', tags: ['boss'], category: 'work' }, annotation: { meme_id: 'boss-1', description: '狗头礼貌微笑', ocr_text: '谢谢老板' } },
    { meme: { id: 'meeting-1', storage_key: 'meeting-1.png', category: 'work' }, annotation: { meme_id: 'meeting-1', description: '一只狗头正在开会', ocr_text: '我想下班' } },
    { meme: { id: 'unknown-1', storage_key: 'unknown-1.png', tags: ['unknown'] } },
    { meme: { id: 'english-1', storage_key: 'english-1.png', category: 'reaction' }, annotation: { meme_id: 'english-1', description: 'dog rolls its eyes', ocr_text: '' } },
  ];
  await db.batch(samples.map(sample => db.prepare(importSQL(indexRow(sample)))));
});
after(async () => { await mf?.dispose(); await rm(temporary, { recursive: true, force: true }); });

let clientIP = 1;
const request = (path: string, body?: object) => mf.dispatchFetch(`https://emomo.test${path}`, {
  method: body ? 'POST' : 'GET',
  headers: { 'content-type': 'application/json', 'cf-connecting-ip': `192.0.2.${clientIP++}` },
  body: body ? JSON.stringify(body) : undefined,
});

test('real local D1 finds Chinese bigrams, OCR, single characters and English without external HTTP', async () => {
  for (const [query, id] of [['无语', 'cat-1'], ['猫', 'cat-1'], ['谢谢老板', 'boss-1'], ['下班', 'meeting-1'], ['ＢＯＳＳ', 'boss-1'], ['rolls eyes', 'english-1']]) {
    const response = await request('/agent/v1/search', { query });
    assert.equal(response.status, 200);
    const data = JSON.parse(await response.text());
    assert.equal(data.results[0].meme.id, id);
    assert.equal(data.profile, 'keyword');
    assert.equal(data.expanded_query, '');
    assert.ok(data.results[0].score >= 0 && data.results[0].score <= 1);
    assert.equal(data.results[0].meme.url, `https://r2.emomo.net/${id}.png`);
  }
  assert.equal(outboundCalls, 0);
});

test('filters and empty results never fall back to semantic search; protobuf aliases are accepted', async () => {
  const matched = await request('/agent/v1/search', { query: '猫', topK: 1, textPresence: 3, category: 'reaction', profile: 'keyword' });
  assert.equal((JSON.parse(await matched.text())).total, 1);
  for (const input of [{ query: '猫', category: 'work' }, { query: '猫', text_presence: 2 }, { query: '扶额' }]) {
    const response = await request('/agent/v1/search', input);
    assert.equal(response.status, 200);
    assert.equal((JSON.parse(await response.text())).total, 0);
  }
  const unknown = await request('/agent/v1/search', { query: 'unknown', text_presence: 1 });
  assert.equal((JSON.parse(await unknown.text())).total, 1);
  assert.equal(outboundCalls, 0);
});

test('detail/categories/stats use canonical metadata only, and legacy/bulk/browser routes remain closed', async () => {
  const detail = await request('/agent/v1/memes/cat-1');
  assert.equal(detail.status, 200);
  assert.equal((JSON.parse(await detail.text())).meme.image_info.format, 2);
  const stats = await request('/agent/v1/stats');
  assert.equal((JSON.parse(await stats.text())).total_memes, '5');
  const categories = await request('/agent/v1/categories');
  assert.deepEqual((JSON.parse(await categories.text())).categories, ['reaction', 'work']);
  for (const path of ['/api/v1/search', '/api/v1/memes']) assert.equal((await request(path)).status, 410);
  for (const path of ['/agent/v1/memes', '/agent/v1/search/stream', '/health', '/agent/v1/memes/cat%2D1', '/agent/v1/stats?admin=1']) assert.equal((await request(path)).status, 404);
  assert.equal((await request('/agent/v1/search')).status, 405);
  assert.equal((await request('/agent/v1/memes/missing')).status, 404);
  assert.equal((await mf.dispatchFetch('https://emomo.test/agent/v1/stats', { headers: { origin: 'https://emomo.net' } })).status, 403);
  assert.equal(outboundCalls, 0);
});

test('bounds, unknown profiles, malformed bodies and query operators cannot bypass the text-only service', async () => {
  for (const input of [{ query: '' }, { query: '猫', top_k: 0 }, { query: '猫', top_k: 101 }, { query: '猫', top_k: 1.5 }, { query: '猫', text_presence: 9 }, { query: '猫', profile: 'qwen3vl' }, { query: '猫', collection: 'image_vectors' }, { query: '* NEAR()' }, { query: '猫'.repeat(161) }, { query: '猫', unknown: 'value' }]) {
    // NEAR is tokenized as a literal word, so this input is valid and empty.
    const response = await request('/agent/v1/search', input);
    assert.equal(response.status, input.query === '* NEAR()' ? 200 : 400);
  }
  for (const [body, type, status] of [['{', 'application/json', 400], ['x', 'text/plain', 415], ['x'.repeat(8193), 'application/json', 413]] as const) {
    assert.equal((await mf.dispatchFetch('https://emomo.test/agent/v1/search', { method: 'POST', headers: { 'content-type': type, 'cf-connecting-ip': '192.0.2.2' }, body })).status, status);
  }
  assert.equal(outboundCalls, 0);
});

test('offline upserts update the FTS index and quote apostrophes as data', async () => {
  const db = await mf.getD1Database('DB');
  const row = indexRow({ meme: { id: 'unknown-1', storage_key: 'unknown-1.png' }, annotation: { meme_id: 'unknown-1', description: "a cat's surprise" } });
  await db.prepare(importSQL(row)).run();
  const newHit = await db.prepare('SELECT COUNT(*) AS hits FROM meme_fts WHERE meme_fts MATCH ?').bind('"surprise"').first('hits');
  const oldHit = await db.prepare('SELECT COUNT(*) AS hits FROM meme_fts WHERE meme_fts MATCH ?').bind('"unknown"').first('hits');
  assert.equal(newHit, 1);
  assert.equal(oldHit, 0);
});

test('disabled and unavailable D1 states do not call any fallback; rate limiter bounds requests', async () => {
  const guarded = new Miniflare({ ...options('false'), ratelimits: { EMOMO_RATE_LIMITER: { namespace_id: '1003', simple: { limit: 2, period: 60 } } } });
  try {
    assert.equal((await guarded.dispatchFetch('https://emomo.test/agent/v1/stats')).status, 410);
    await guarded.setOptions({ ...options('true'), ratelimits: { EMOMO_RATE_LIMITER: { namespace_id: '1003', simple: { limit: 2, period: 60 } } } });
    assert.equal((await guarded.dispatchFetch('https://emomo.test/agent/v1/stats')).status, 503);
    assert.equal((await guarded.dispatchFetch('https://emomo.test/agent/v1/stats')).status, 503);
    const throttled = await guarded.dispatchFetch('https://emomo.test/agent/v1/stats');
    assert.equal(throttled.status, 429);
    assert.equal(throttled.headers.get('retry-after'), '60');
  } finally { await guarded.dispose(); }
  assert.equal(outboundCalls, 0);
});

test('actual installed CLI searches local D1, gets real metadata and downloads an image with no Worker outbound HTTP', { timeout: 30000 }, async () => {
  let local: Miniflare;
  const server = createServer(async (incoming, outgoing) => {
    try {
      if (incoming.url === '/cat-1.png') { outgoing.writeHead(200, { 'content-type': 'image/png' }); outgoing.end(PNG); return; }
      const chunks: Buffer[] = [];
      for await (const chunk of incoming) chunks.push(Buffer.from(chunk));
      const response = await local.dispatchFetch(`${origin}${incoming.url}`, {
        method: incoming.method,
        headers: { 'content-type': incoming.headers['content-type'] || 'application/json' },
        body: incoming.method === 'POST' ? Buffer.concat(chunks) : undefined,
      });
      outgoing.writeHead(response.status, Object.fromEntries(response.headers));
      outgoing.end(Buffer.from(await response.arrayBuffer()));
    } catch { outgoing.writeHead(500); outgoing.end('test bridge failed'); }
  });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  const address = server.address();
  assert.ok(address && typeof address !== 'string');
  const origin = `http://127.0.0.1:${address.port}`;
  local = new Miniflare(options('true', origin));
  try {
    const db = await local.getD1Database('DB');
    const schema = await readFile(fileURLToPath(new URL('../migrations/0001_text_index.sql', import.meta.url).href), 'utf8');
    await db.batch(schema.split('-- statement').map(sql => sql.trim()).filter(Boolean).map(sql => db.prepare(sql)));
    await db.prepare(importSQL(indexRow({ meme: { id: 'cat-1', storage_key: 'cat-1.png', image_info: { width: 1, height: 1, format: 2 } }, annotation: { meme_id: 'cat-1', description: '一只猫咪露出无语的表情' } }))).run();
    const packed = JSON.parse((await execute('npm', ['pack', '--pack-destination', temporary, '--json'], { cwd: cli })).stdout)[0];
    const prefix = join(temporary, 'installed');
    await execute('npm', ['install', '--global', '--prefix', prefix, '--ignore-scripts', '--no-audit', '--no-fund', join(temporary, packed.filename)]);
    const binary = join(prefix, 'bin', 'emomo');
    const env = { ...process.env, EMOMO_API_URL: `${origin}/agent/v1`, EMOMO_API_TOKEN: '', CODEX_HOME: join(temporary, 'codex') };
    const invoke = async (args: string[]) => JSON.parse((await execute(binary, args, { env })).stdout);
    const installed = await invoke(['skill', 'install']);
    assert.match(await readFile(join(installed.data.path, 'SKILL.md'), 'utf8'), /keyword|关键词/i);
    const result = await invoke(['search', '无语 猫', '--limit', '3']);
    assert.equal(result.data.results[0].id, 'cat-1');
    assert.equal((await invoke(['get', 'cat-1'])).data.meme.id, 'cat-1');
    const image = await invoke(['download', 'cat-1', '--dir', join(temporary, 'images')]);
    assert.deepEqual(await readFile(image.data.path), PNG);
    assert.equal(outboundCalls, 0);
  } finally { server.closeAllConnections(); server.close(); await local.dispose(); }
});

test('the bundled dependency graph contains only canonical protobuf and deterministic source modules', () => {
  assert.ok(bundleInputs.every(path => path.startsWith('src/') || path.startsWith('gen/') || path.startsWith('node_modules/@bufbuild/protobuf/')));
  assert.equal(outboundCalls, 0);
});
