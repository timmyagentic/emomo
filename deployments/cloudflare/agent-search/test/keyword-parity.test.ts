import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtemp, readFile, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { build } from 'esbuild';
import { Miniflare, Response } from 'miniflare';
import { authorizedFetch, bindings, admission } from './access-fixture.js';
import { indexRow, importSQL } from '../src/metadata.js';

const execute = promisify(execFile);
const cli = fileURLToPath(new URL('../../../../cli/bin/emomo.js', import.meta.url));
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/l9sAAAAASUVORK5CYII=', 'base64');

test('local and hosted search agree on every keyword, aliases, subjects, filters and negative queries', async t => {
  const root = await mkdtemp(join(tmpdir(), 'emomo-keyword-parity-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const image = join(root, 'source.png'); await writeFile(image, png);
  const samples = [
    { id: 'cat', tags: ['猫', '生气'], text: '火大' },
    { id: 'panda', tags: ['熊猫', '生气'], text: '别惹我' },
    { id: 'happy', tags: ['猫', '开心'], text: '开心' },
    { id: 'birthday', tags: ['狗', '生日快乐'], text: '生日快乐' },
    { id: 'refusal', tags: ['礼貌', '拒绝'], text: '谢谢，不用了' },
    { id: 'dumpling', tags: ['饺子'], text: '', category: 'object_sticker' },
  ];
  const metadata = samples.map(s => ({ id: s.id, category: s.category || 'usable', description: s.tags.join('；'), image_text: s.text, intent_tags: s.tags, subjects: [], scenarios: [], query_aliases: [], file_path: image, sha256: createHash('sha256').update(png).digest('hex'), width: 1, height: 1, frames: 1, media_kind: 'image' }));
  const input = join(root, 'metadata.json'); await writeFile(input, JSON.stringify(metadata));
  const catalog = join(root, 'catalog');
  const env = { ...process.env, EMOMO_CONFIG_DIR: join(root, 'config'), EMOMO_API_TOKEN: '', EMOMO_API_URL: '', EMOMO_CATALOG: '' };
  const invoke = async (args: string[]) => JSON.parse((await execute(process.execPath, [cli, ...args], { env })).stdout).data;
  await invoke(['catalog', 'import', input, '--dir', catalog]);
  const before = await readFile(join(catalog, 'search.sqlite3'));
  const bundle = await build({ entryPoints: ['src/index.ts'], write: false, bundle: true, format: 'esm', platform: 'browser', target: 'es2022', external: ['cloudflare:workers'] });
  let outbound = 0;
  const mf = new Miniflare({ modules: true, script: bundle.outputFiles[0].text, compatibilityDate: '2026-07-30', compatibilityFlags: ['nodejs_compat'],
    bindings: { ...bindings, AGENT_API_ENABLED: 'true', IMAGE_BASE_URL: 'https://images.example' },
    d1Databases: { DB: 'parity' }, durableObjects: { ...admission, PER_IP_LIMITER: { className: 'PerIpLimiter', useSQLite: true } },
    ratelimits: { EMOMO_RATE_LIMITER: { namespace_id: '1010', simple: { limit: 100, period: 60 } } },
    outboundService: () => { outbound++; return new Response('forbidden', { status: 502 }); },
  });
  t.after(() => mf.dispose());
  const db = await mf.getD1Database('DB');
  const schema = await readFile(new URL('../migrations/0001_text_index.sql', import.meta.url), 'utf8');
  await db.batch(schema.split('-- statement').map(s => s.trim()).filter(Boolean).map(s => db.prepare(s)));
  await db.batch(samples.map(s => db.prepare(importSQL(indexRow({ meme: { id: 'local-' + s.id, storage_key: s.id + '.png', tags: s.tags, category: s.category || 'usable' }, annotation: { meme_id: 'local-' + s.id, description: s.tags.join('；'), ocr_text: s.text }, text_presence: s.text ? 2 : 1 })))));
  const cases: [string, string?, string[]?][] = [
    ['猫 生气', undefined, ['local-cat']], ['猫愤怒', undefined, ['local-cat']],
    ['熊猫 生气', undefined, ['local-panda']], ['猫 生日快乐', undefined, []],
    ['礼貌拒绝', undefined, ['local-refusal']], ['饺子', undefined, []],
    ['饺子', 'object_sticker', ['local-dumpling']], ['火星文未知', undefined, []],
  ];
  let ip = 0;
  for (const [query, category, expected] of cases) {
    const local = await invoke(['search', query, '--catalog', catalog, ...(category ? ['--category', category] : [])]);
    const response = await authorizedFetch(mf, 'https://emomo.test/agent/v1/search', { method: 'POST', headers: { 'content-type': 'application/json', 'cf-connecting-ip': `192.0.2.${++ip}` }, body: JSON.stringify({ query, ...(category ? { category } : {}) }) });
    assert.equal(response.status, 200, query);
    const remote = await response.json() as { results: { meme: { id: string } }[] };
    const localIds = local.results.map((r: { id: string }) => r.id);
    assert.deepEqual(localIds, expected, query);
    assert.deepEqual(remote.results.map(r => r.meme.id), localIds, query);
  }
  assert.deepEqual(await readFile(join(catalog, 'search.sqlite3')), before);
  assert.deepEqual(await readFile(image), png);
  assert.equal(outbound, 0);
});
