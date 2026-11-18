import assert from 'node:assert/strict';
import { test } from 'node:test';
import { build } from 'esbuild';
import { createHash } from 'node:crypto';
import { readFile, writeFile, mkdtemp, rm } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { Miniflare, Response as MFResponse } from 'miniflare';
import { token, bindings, admission } from './access-fixture.js';
import { applyLibraryMigrations } from './library-migrations.js';
import { indexRow, importSQL, UPSERT, rowValues } from '../src/metadata.js';
// @ts-expect-error Exercise the dependency-free CLI against real local Workers.
import { takedownCommand } from '../../../../cli/src/library-takedown.js';
// @ts-expect-error Exercise the dependency-free import client against real local Workers.
import { ImportClient } from '../../../../cli/src/import-client.js';

const hash = (value: Buffer | string) => createHash('sha256').update(value).digest('hex');
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/l9sAAAAASUVORK5CYII=', 'base64');
const sha256 = hash(png);
const key = `collections/reviewed/${sha256}.png`;
const importKey = `emomo_import_${hash('import fixture')}`;
const takedownKey = `emomo_takedown_${hash('takedown fixture')}`;
const maintenanceUrl = 'https://imports.test/admin/v1';

test('selected originals leave search, details and downloads, and a confirmed retry preserves the D1 receipt', async t => {
  const [maintenance, images, api] = await Promise.all(['import', 'download', 'index'].map(name =>
    build({ entryPoints: [`src/${name}.ts`], write: false, bundle: true, format: 'esm', platform: 'browser', external: ['cloudflare:workers', 'node:buffer'] })));
  let outbound = 0;
  const shared = { modules: true, compatibilityDate: '2026-07-30', compatibilityFlags: ['nodejs_compat'], d1Databases: { DB: 'library' },
    outboundService: () => { outbound++; return new MFResponse('Forbidden', { status: 502 }); } };
  const bucket = { r2Buckets: { IMAGES: 'images' } };
  const mf = new Miniflare({ workers: [
    { ...shared, ...bucket, name: 'maintenance', script: maintenance.outputFiles[0].text,
      bindings: { IMPORT_KEY_SHA256: hash(importKey), TAKEDOWN_KEY_SHA256: hash(takedownKey), IMAGE_BASE_URL: 'https://images.test' } },
    { ...shared, ...bucket, name: 'images', script: images.outputFiles[0].text, bindings,
      durableObjects: { ADMISSION: { className: 'Admission', scriptName: 'api', useSQLite: true }, DOWNLOAD_IP_LIMITER: { className: 'PerIpLimiter', useSQLite: true } },
      ratelimits: { DOWNLOAD_RATE_LIMITER: { namespace_id: '2001', simple: { limit: 1000, period: 60 } } } },
    { ...shared, name: 'api', script: api.outputFiles[0].text,
      bindings: { ...bindings, AGENT_API_ENABLED: 'true', IMAGE_BASE_URL: 'https://images.test' },
      durableObjects: { ...admission, PER_IP_LIMITER: { className: 'PerIpLimiter', useSQLite: true } },
      ratelimits: { EMOMO_RATE_LIMITER: { namespace_id: '2002', simple: { limit: 1000, period: 60 } } } },
  ] });
  t.after(() => mf.dispose());
  const db = await mf.getD1Database('DB', 'maintenance');
  await applyLibraryMigrations(db);
  const r2 = (await mf.getBindings<{ IMAGES: R2Bucket }>('maintenance')).IMAGES;
  await r2.put(key, new Uint8Array(png));

  const makeRow = (id: string, hashValue = sha256, storageKey = key) => indexRow({ meme: {
    id, content_hash: hashValue, storage_key: storageKey, category: 'usable',
  }, annotation: { meme_id: id, description: '猫咪',
  } });
  await db.batch([
    db.prepare(importSQL(makeRow('archive-id'))),
    db.prepare(importSQL(makeRow('other-id', 'b'.repeat(64), `collections/reviewed/${'b'.repeat(64)}.png`))),
    db.prepare(importSQL(makeRow('unknown-old-id', '', 'old.png'))),
  ]);

  const worker = async (name: string) => await mf.getWorker(name) as unknown as { fetch: Miniflare['dispatchFetch'] };
  const management = await worker('maintenance');
  const download = await worker('images');
  const search = await worker('api');
  const post = (path: string, body: unknown, key = takedownKey, extra = {}) => management.fetch(`${maintenanceUrl}/${path}`, {
    method: 'POST', headers: { authorization: `Bearer ${key}`, 'content-type': 'application/json', ...extra }, body: JSON.stringify(body),
  });
  let ip = 1;
  const caller = () => ({ authorization: `Bearer ${token(ip)}`, 'cf-connecting-ip': `198.51.100.${ip++}` });
  const getImage = (method = 'GET') => download.fetch(`https://images.test/${key}`, { method, headers: caller() });
  const lookup = async () => (await post('lookup', { sha256: [sha256] }, importKey)).json() as Promise<{ existing: {id:string}[] }>;
  const searchForCat = async (host: string) => (await search.fetch(`https://${host}/agent/v1/search`, { method: 'POST',
    headers: { ...caller(), 'content-type': 'application/json' }, body: JSON.stringify({ query: '猫咪' }),
  })).json() as Promise<{ results: {meme:{id:string}}[] }>;

  assert.equal((await getImage()).status, 200);
  assert.ok((await searchForCat('before.test')).results.some(row => row.meme.id === 'archive-id'));
  assert.deepEqual((await lookup()).existing.map(row => row.id), ['archive-id']);
  assert.equal((await post('takedowns/plan', { sha256 }, importKey)).status, 401);
  assert.equal((await post('lookup', { sha256: [sha256] })).status, 401);
  assert.equal((await post('takedowns/plan', { sha256 }, takedownKey, { origin: 'https://browser.test' })).status, 403);
  assert.equal((await post('takedowns/plan', { id: 'missing' })).status, 404);
  assert.equal((await post('takedowns/plan', { id: 'unknown-old-id' })).status, 409);
  assert.equal((await post('takedowns/plan', { sha256, id: 'other-id' })).status, 400);
  assert.equal((await post('takedowns', { sha256, confirmation: '0'.repeat(64) }, importKey)).status, 401);

  const area = await mkdtemp(join(tmpdir(), 'emomo-takedown-'));
  t.after(() => rm(area, { recursive: true, force: true }));
  const selection = join(area, 'selection.json'), plan = join(area, 'plan.json'), original = join(area, 'original.png');
  const keyFile = join(area, 'private'), env = { EMOMO_CONFIG_DIR: join(area, 'config') };
  await writeFile(keyFile, takedownKey, { mode: 0o600 });
  await writeFile(original, png);
  await writeFile(selection, JSON.stringify([{ file: original }, { id: 'archive-id' }, { sha256 }]));
  await takedownCommand(['connect-takedown', '--key-file', keyFile, '--api-url', maintenanceUrl], env);

  // Bridge the configured CLI client to workerd without forwarding credentials to the network.
  const bridge = async (url: string, options: RequestInit) => {
    assert.ok(url.startsWith(`${maintenanceUrl}/`));
    const request = new Request(url, options);
    const response = await management.fetch(url, { method: request.method, headers: Object.fromEntries(request.headers),
      body: request.method === 'POST' ? new Uint8Array(await request.arrayBuffer()) : undefined });
    return new Response(response.body as ReadableStream<Uint8Array> | null, { status: response.status, headers: Object.fromEntries(response.headers) });
  };
  const originalFetch = globalThis.fetch;
  globalThis.fetch = bridge as typeof fetch;
  t.after(() => { globalThis.fetch = originalFetch; });

  const first = await takedownCommand(['takedown-plan', selection, '--out', plan], env);
  assert.equal(first.items.length, 1);
  assert.deepEqual(first.items[0].targets, [{ id: 'archive-id', storage_key: key }]);
  assert.equal(first.items[0].taken_down_at, '');
  assert.deepEqual((await lookup()).existing.map(row => row.id), ['archive-id']);
  await assert.rejects(takedownCommand(['takedown', plan, '--confirm', '0'.repeat(64)], env), { code: 'INVALID_TAKEDOWN' });
  await db.prepare(importSQL(makeRow('another-id'))).run();
  const stale = await takedownCommand(['takedown', plan, '--confirm', first.confirmation], env);
  assert.equal(stale.status, 'partial');
  assert.equal(stale.failed.error.httpStatus, 409);
  assert.equal((await lookup()).existing.length, 2);

  const secondPlan = join(area, 'second-plan.json');
  const second = await takedownCommand(['takedown-plan', selection, '--out', secondPlan], env);
  assert.deepEqual(second.items[0].targets.map((row: {id:string}) => row.id), ['another-id', 'archive-id']);

  // A delete failure cannot commit a block record or remove only some of the aliases.
  await db.prepare("CREATE TRIGGER fail_takedown BEFORE DELETE ON memes WHEN old.id = 'archive-id' BEGIN SELECT RAISE(ABORT, 'injected failure'); END;").run();
  const blocked = await post('takedowns', { sha256, confirmation: second.items[0].confirmation });
  assert.equal(blocked.status, 503);
  assert.equal((await lookup()).existing.length, 2);
  assert.equal(await db.prepare('SELECT 1 FROM library_takedowns WHERE sha256=?').bind(sha256).first(), null);
  await db.prepare('DROP TRIGGER fail_takedown').run();

  const results = await Promise.all([
    takedownCommand(['takedown', secondPlan, '--confirm', second.confirmation], env),
    post('takedowns', { sha256, confirmation: second.items[0].confirmation }),
  ]);
  const completed = results[0];
  assert.equal(completed.status, 'taken_down');
  assert.equal(results[1].status, 200);
  assert.deepEqual(await results[1].json(), completed.results[0]);
  assert.equal((await lookup()).existing.length, 0);
  assert.equal((await getImage()).status, 404);
  assert.equal((await getImage('HEAD')).status, 404);
  assert.ok(await r2.head(key));
  assert.equal((await management.fetch(`${maintenanceUrl}/images/${sha256}`, { headers: { authorization: `Bearer ${importKey}` } })).status, 404);
  for (const id of ['archive-id', 'another-id']) {
    assert.equal((await search.fetch(`https://after.test/agent/v1/memes/${id}`, { headers: caller() })).status, 404);
  }
  assert.deepEqual((await searchForCat('after.test')).results.map(row => row.meme.id).sort(), ['other-id', 'unknown-old-id']);
  const stats = await search.fetch('https://after.test/agent/v1/stats', { headers: caller() });
  assert.equal((await stats.json() as {total_memes:string}).total_memes, '2');
  const archived = await post('takedowns/plan', { id: 'archive-id' });
  assert.deepEqual(await archived.json(), completed.results[0]);

  const fresh = new ImportClient(maintenanceUrl, importKey, bridge);
  const metadata = { sha256, image_info: { width: 1, height: 1, format: 2 }, category: 'usable',
    description: '猫咪开心', ocr_text: '', text_presence: 3, tags: ['猫'], search_aliases: [] };
  await assert.rejects(fresh.upload(metadata, png), { code: 'IMAGE_TAKEN_DOWN', httpStatus: 409 });
  await db.prepare(importSQL(makeRow('restored-id'))).run();
  await db.prepare(UPSERT).bind(...rowValues(makeRow('other-id'))).run();
  assert.equal((await lookup()).existing.length, 0);
  assert.equal(await db.prepare("SELECT id FROM memes WHERE id='restored-id'").first(), null);
  assert.ok(await db.prepare("SELECT id FROM memes WHERE id='other-id'").first());

  const retry = await takedownCommand(['takedown', secondPlan, '--confirm', second.confirmation], env);
  assert.deepEqual(retry.results, completed.results);
  assert.equal(JSON.parse(await readFile(retry.receipt, 'utf8')).status, 'taken_down');
  assert.deepEqual(await readFile(original), png);

  // Persist the first receipt and recover the second after its response is lost.
  const more = ['c'.repeat(64), 'd'.repeat(64)];
  for (const [index, digest] of more.entries()) {
    await db.prepare(importSQL(makeRow(`next-${index}`, digest, `collections/reviewed/${digest}.png`))).run();
  }
  await writeFile(selection, JSON.stringify(more.map(sha256 => ({ sha256 }))));
  const retryPlan = join(area, 'retry-plan.json');
  const pending = await takedownCommand(['takedown-plan', selection, '--out', retryPlan], env);
  const attempts: string[] = [];
  globalThis.fetch = (async (url: string, options: RequestInit) => {
    const { sha256 } = JSON.parse(await new Request(url, options).text()) as { sha256: string };
    assert.ok(url.endsWith('/takedowns'));
    attempts.push(sha256);
    const response = await bridge(url, options);
    if (attempts.length === 2) {
      assert.equal(response.status, 200);
      await response.body?.cancel();
      throw new Error('Simulate lost response');
    }
    return response;
  }) as typeof fetch;
  const interrupted = await takedownCommand(['takedown', retryPlan, '--confirm', pending.confirmation], env);
  assert.equal(interrupted.status, 'partial');
  assert.equal(interrupted.results.length, 1);
  assert.equal(interrupted.failed.error.retryable, true);
  const resumed = await takedownCommand(['takedown', retryPlan, '--confirm', pending.confirmation], env);
  assert.equal(resumed.status, 'taken_down');
  assert.deepEqual(resumed.results.map((row: {sha256:string}) => row.sha256), more);
  assert.deepEqual(attempts, [...more, more[1]]);
  assert.equal(outbound, 0);
});
