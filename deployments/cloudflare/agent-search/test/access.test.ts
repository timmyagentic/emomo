import assert from 'node:assert/strict';
import { test } from 'node:test';
import { build } from 'esbuild';
import { Miniflare } from 'miniflare';
import { admission, bindings, policies, registry, token } from './access-fixture.js';

// This RPC test entrypoint is never built or exported by either production Worker.
const bundle = await build({ stdin: { contents: `
export { Admission } from './src/access.ts';
export default { async fetch(r,e) {
 const { name, method, args } = await r.json();
 return Response.json((await e.ADMISSION.getByName(name)[method](...args)) ?? null);
} };`, resolveDir: process.cwd() }, write: false, bundle: true, format: 'esm', platform: 'browser', external: ['cloudflare:workers'] });
const limits = { perSecond: 100, perDay: 100, inFlight: 2, failures: 2, cooldownSeconds: 1 };
test('durable quotas serialize concurrent requests and keep operation counters isolated', async () => {
 const mf = new Miniflare({ modules: true, script: bundle.outputFiles[0].text, compatibilityDate: '2026-07-30', durableObjects: admission });
 try {
  const rpc = async (name: string, method: string, ...args: unknown[]) => (await mf.dispatchFetch('https://test/rpc', { method: 'POST', body: JSON.stringify({ name, method, args }) })).json() as Promise<any>;
  const results = await Promise.all(Array.from({ length: 20 }, () => rpc('key/a/read', 'quota', { hour: 3, day: 5 }, 0)));
  assert.equal(results.filter(r => !r.retry).length, 3);
  assert.equal(results.filter(r => r.reason === 'hour').length, 17);
  assert.equal((await rpc('key/a/read', 'quota', { hour: 10, day: 3 }, 0)).reason, 'day');
  assert.equal((await rpc('key/a/search', 'quota', { hour: 3, day: 5 }, 1000)).retry, 0);
  assert.equal((await rpc('key/a/search', 'quota', { hour: 3, day: 5 }, 1000)).reason, 'rate');
  assert.equal((await rpc('key/b/read', 'quota', { hour: 3, day: 5 }, 0)).retry, 0);
 } finally { await mf.dispose(); }
});
test('global capacity, budget and circuit admit one recovery probe', async () => {
 const mf = new Miniflare({ modules: true, script: bundle.outputFiles[0].text, compatibilityDate: '2026-07-30', durableObjects: admission });
 try {
  const rpc = async (name: string, method: string, ...args: unknown[]) => (await mf.dispatchFetch('https://test/rpc', { method: 'POST', body: JSON.stringify({ name, method, args }) })).json() as Promise<any>;
  const a = await rpc('global/read','enter',limits), b = await rpc('global/read','enter',limits);
  assert.equal((await rpc('global/read','enter',limits)).reason,'capacity');
  await rpc('global/read','finish',a.lease,true,limits); await rpc('global/read','finish',b.lease,true,limits);
  assert.equal((await rpc('global/read','enter',limits)).reason,'circuit');
  await new Promise(r => setTimeout(r,1050));
  const probe = await rpc('global/read','enter',limits); assert.ok(probe.lease);
  assert.equal((await rpc('global/read','enter',limits)).reason,'circuit');
  await rpc('global/read','finish',probe.lease,false,limits);
  assert.ok((await rpc('global/read','enter',limits)).lease);
  const budget = { ...limits, perDay: 1 };
  assert.ok((await rpc('global/download','enter',budget)).lease);
  assert.equal((await rpc('global/download','enter',budget)).reason,'global-day');
  const rate = { ...limits, perSecond: 1 };
  assert.ok((await rpc('global/search','enter',rate)).lease);
  assert.equal((await rpc('global/search','enter',rate)).reason,'global-rate');
 } finally { await mf.dispose(); }
});
test('real API rejects missing, wrong, expired and revoked keys before missing backend; quota follows a key across IPs', async () => {
 const worker = await build({ entryPoints: ['src/index.ts'], write: false, bundle: true, format: 'esm', platform: 'browser', external: ['cloudflare:workers'] });
 const keys = { ...registry, 'test-0': { ...registry['test-0'], quotas: { read: { hour: 2, day: 2 } } }, 'test-1': { ...registry['test-1'], disabled: true }, 'test-2': { ...registry['test-2'], expiresAt: 1 } };
 const mf = new Miniflare({ modules: true, script: worker.outputFiles[0].text, compatibilityDate: '2026-07-30', bindings: { ...bindings, ACCESS_KEYS_JSON: JSON.stringify(keys), AGENT_API_ENABLED: 'true', IMAGE_BASE_URL: 'https://images.test' }, durableObjects: { ...admission, PER_IP_LIMITER: { className: 'PerIpLimiter', useSQLite: true } }, ratelimits: { EMOMO_RATE_LIMITER: { namespace_id: '1900', simple: { limit: 1000, period: 60 } } } });
 try {
  const call = (key: string, ip = '192.0.2.1') => mf.dispatchFetch('https://api.test/agent/v1/stats', { headers: { authorization: key, 'cf-connecting-ip': ip } });
  for (const key of ['', `Bearer ${token(1)}`, `Bearer ${token(2)}`, `Bearer ${token(0).slice(0,-1)}x`]) assert.equal((await call(key)).status,401);
  // No DB binding: admitted requests fail as 503, demonstrating admission is before DB access.
  assert.equal((await call(`Bearer ${token(0)}`,'192.0.2.2')).status,503);
  assert.equal((await call(`Bearer ${token(0)}`,'192.0.2.3')).status,503);
  const denied = await call(`Bearer ${token(0)}`,'192.0.2.4');
  assert.equal(denied.status,429); assert.equal(denied.headers.get('x-emomo-limit'),'day'); assert.ok(Number(denied.headers.get('retry-after')) > 0);
 } finally { await mf.dispose(); }
});

test('image Worker uses the API-owned admission namespace and enforces download quota across IPs', async () => {
 const api = await build({ entryPoints: ['src/index.ts'], write: false, bundle: true, format: 'esm', platform: 'browser', external: ['cloudflare:workers'] });
 const images = await build({ entryPoints: ['src/download.ts'], write: false, bundle: true, format: 'esm', platform: 'browser', external: ['cloudflare:workers'] });
 const keys = { 'test-0': { ...registry['test-0'], quotas: { download: { hour: 1, day: 1 } } } };
 const mf = new Miniflare({ workers: [
  { name: 'api', modules: true, script: api.outputFiles[0].text, compatibilityDate: '2026-07-30', durableObjects: admission },
  { name: 'images', modules: true, script: images.outputFiles[0].text, compatibilityDate: '2026-07-30', bindings: { ...bindings, ACCESS_KEYS_JSON: JSON.stringify(keys) }, durableObjects: { ADMISSION: { className: 'Admission', scriptName: 'api', useSQLite: true }, DOWNLOAD_IP_LIMITER: { className: 'PerIpLimiter', useSQLite: true } }, ratelimits: { DOWNLOAD_RATE_LIMITER: { namespace_id: '1901', simple: { limit: 1000, period: 60 } } } },
 ] });
 try {
  const worker = await mf.getWorker('images');
  const url = `https://images.test/collections/reviewed/${'a'.repeat(64)}.png`;
  const call = (ip: string, authorization = `Bearer ${token(0)}`) => worker.fetch(url,{headers:{authorization,'cf-connecting-ip':ip}});
  assert.equal((await call('192.0.2.1','')).status,401);
  // Missing R2 proves the admitted first request reaches storage; the next must not.
  assert.equal((await call('192.0.2.2')).status,503);
  const denied = await call('192.0.2.3');
  assert.equal(denied.status,429); assert.equal(denied.headers.get('x-emomo-limit'),'day');
 } finally { await mf.dispose(); }
});
