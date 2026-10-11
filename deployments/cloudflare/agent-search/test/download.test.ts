import assert from 'node:assert/strict';
import { before, after, test } from 'node:test';
import { build } from 'esbuild';
import { createHash } from 'node:crypto';
import { Miniflare, Response as MFResponse } from 'miniflare';

const PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/l9sAAAAASUVORK5CYII=', 'base64');
const key = `collections/reviewed/${createHash('sha256').update(PNG).digest('hex')}.png`;
let mf: Miniflare;
let script: string;
let outbound = 0;
const options = () => ({
  modules: true, script, compatibilityDate: '2026-07-30',
  durableObjects: { DOWNLOAD_IP_LIMITER: { className: 'PerIpLimiter', useSQLite: true } },
  r2Buckets: { IMAGES: 'images' },
  ratelimits: { DOWNLOAD_RATE_LIMITER: { namespace_id: '1004', simple: { limit: 30, period: 60 as const } } },
  outboundService: () => { outbound++; return new MFResponse('Forbidden', { status: 502 }); },
});
before(async () => {
  const bundle = await build({ entryPoints: ['src/download.ts'], write: false, bundle: true, format: 'esm', platform: 'browser', external: ['cloudflare:workers'] });
  script = bundle.outputFiles[0].text;
  mf = new Miniflare(options());
  await (await mf.getR2Bucket('IMAGES') as unknown as R2Bucket).put(key, new Uint8Array(PNG));
});
after(async () => { await mf?.dispose(); });
const request = (ip: string, method = 'GET', headers: Record<string, string> = {}, path = key) => mf.dispatchFetch(`https://images.test/${path}`, { method, headers: { 'cf-connecting-ip': ip, ...headers } });

test('full original bytes stream from R2 and repeated concurrent downloads cannot bypass the interval', async () => {
  const responses = await Promise.all(Array.from({ length: 20 }, (_, i) => request('198.51.100.1', 'GET', { 'x-forwarded-for': `203.0.113.${i}`, 'if-none-match': '*' })));
  const ok = responses.filter(r => r.status === 200);
  assert.equal(ok.length, 1);
  assert.deepEqual(Buffer.from(await ok[0].arrayBuffer()), PNG);
  assert.equal(ok[0].headers.get('content-type'), 'image/png');
  for (const response of responses) {
    assert.equal(response.headers.get('cache-control'), 'no-store');
    assert.equal(response.headers.get('cdn-cache-control'), 'no-store');
    if (response !== ok[0]) { assert.equal(response.status, 429); assert.equal(response.headers.get('retry-after'), '1'); }
  }
  assert.equal((await request('198.51.100.2')).status, 200);
  await new Promise(resolve => setTimeout(resolve, 1050));
  assert.equal((await request('198.51.100.1')).status, 200);
  assert.equal(outbound, 0);
});

test('HEAD, ranges, missing objects, query strings and unsupported methods have no byte-serving bypass', async () => {
  const head = await request('198.51.100.3', 'HEAD');
  assert.equal(head.status, 200); assert.equal((await head.arrayBuffer()).byteLength, 0);
  assert.equal(head.headers.get('content-length'), String(PNG.length));
  assert.equal((await request('198.51.100.3')).status, 429);
  assert.equal((await request('198.51.100.4', 'GET', { range: 'bytes=0-1' })).status, 416);
  assert.equal((await request('198.51.100.4')).status, 429);
  assert.equal((await request('198.51.100.5', 'GET', {}, `collections/reviewed/${'0'.repeat(64)}.png`)).status, 404);
  assert.equal((await request('198.51.100.5')).status, 429);
  for (const path of ['', 'private/test.png', `${key}?x=1`, key.replace('collections', '%63ollections')]) assert.equal((await request('198.51.100.6', 'GET', {}, path)).status, 404);
  assert.equal((await request('198.51.100.6', 'POST')).status, 405);
});

test('missing identity, limiter and R2 bindings fail closed; edge bulk guard returns its longer retry interval', async () => {
  // Miniflare supplies loopback CF-Connecting-IP automatically; strip it inside a test-only entrypoint.
  const missingIdentityBundle = await build({ stdin: { contents: "import handler from './src/download.ts'; export { PerIpLimiter } from './src/rate-limit.ts'; export default { fetch(r,e,c) { r = new Request(r); r.headers.delete('cf-connecting-ip'); return handler.fetch(r,e,c); } };", resolveDir: process.cwd() }, write: false, bundle: true, format: 'esm', platform: 'browser', external: ['cloudflare:workers'] });
  const missingIdentity = new Miniflare({ ...options(), script: missingIdentityBundle.outputFiles[0].text });
  try { assert.equal((await missingIdentity.dispatchFetch(`https://images.test/${key}`)).status, 503); }
  finally { await missingIdentity.dispose(); }
  const broken = new Miniflare({ ...options(), durableObjects: {} });
  try {
    assert.equal((await broken.dispatchFetch(`https://images.test/${key}`, { headers: { 'cf-connecting-ip': '198.51.100.9' } })).status, 503);
    await broken.setOptions({ ...options(), r2Buckets: {} });
    assert.equal((await broken.dispatchFetch(`https://images.test/${key}`, { headers: { 'cf-connecting-ip': '198.51.100.10' } })).status, 503);
    await broken.setOptions({ ...options(), ratelimits: { DOWNLOAD_RATE_LIMITER: { namespace_id: '1005', simple: { limit: 1, period: 60 } } } });
    await broken.dispatchFetch(`https://images.test/${key}`, { headers: { 'cf-connecting-ip': '198.51.100.11' } });
    const denied = await broken.dispatchFetch(`https://images.test/${key}`, { headers: { 'cf-connecting-ip': '198.51.100.11' } });
    assert.equal(denied.status, 429); assert.equal(denied.headers.get('retry-after'), '60');
  } finally { await broken.dispose(); }
  assert.equal(outbound, 0);
});
