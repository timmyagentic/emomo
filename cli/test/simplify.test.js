import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { run } from '../src/cli.js';
import { json, serve } from './helpers.js';

test('default help presents the daily workflow; advanced and old discovery expose compatible commands', async () => {
  const help = (await run(['--help'], { env: {} })).data;
  assert.deepEqual(help.commands.map(c => c.command), ['search', 'get', 'download', 'auth configure', 'doctor']);
  assert.ok(!help.commands[0].flags.includes('profile'));
  const full = (await run(['--help', '--all'], { env: {} })).data;
  for (const name of ['catalog use', 'auth clear', 'skill install', 'stats']) assert.ok(full.commands.some(c => c.command === name));
  assert.deepEqual((await run(['capabilities'], { env: {} })).data.commands, full.commands);
});

test('an explicitly saved remote library is used, shown in results, and does not inherit a local catalog', async t => {
  const root = await mkdtemp(join(tmpdir(), 'emomo-selection-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const env = { EMOMO_CONFIG_DIR: root };
  await writeFile(join(root, 'config.json'), JSON.stringify({ catalog: '/missing-user-disk' }));
  // Saving a source is explicit and must not query the network.
  const source = 'https://custom.example/agent/v1';
  await run(['catalog', 'use', source], { env });
  const saved = JSON.parse(await readFile(join(root, 'config.json')));
  assert.equal(saved.apiUrl, source); assert.equal(saved.catalog, undefined);
  const previous = globalThis.fetch;
  t.after(() => { globalThis.fetch = previous; });
  globalThis.fetch = async url => {
    assert.equal(String(url), source + '/search');
    return Response.json({ query: '猫', results: [] });
  };
  const result = await run(['search', '猫'], { env });
  assert.deepEqual(result.data.library, { type: 'remote', apiUrl: source, selectedBy: 'config' });
});

test('doctor consolidates remote source and auth configuration without returning secrets or extra requests', async t => {
  const root = await mkdtemp(join(tmpdir(), 'emomo-doctor-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const api = await serve((_req, res) => json(res, { total_memes: '7833', total_categories: 2 }), t);
  const env = { EMOMO_CONFIG_DIR: root };
  const result = await run(['doctor', '--api-url', api.origin + '/agent/v1'], { env });
  assert.equal(result.data.stats.totalMemes, '7833');
  assert.deepEqual(result.data.library, { type: 'remote', apiUrl: api.origin + '/agent/v1', selectedBy: '--api-url' });
  assert.equal(result.data.auth.configured, false);
  assert.equal(result.data.auth.used, false);
  assert.equal(api.requests.length, 1);
});

test('legacy saved local selection remains local and never silently switches to the hosted library', async t => {
  const root = await mkdtemp(join(tmpdir(), 'emomo-selection-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const env = { EMOMO_CONFIG_DIR: root };
  await mkdir(join(root, 'config'));
  const saved = JSON.stringify({ catalog: join(root, 'offline-disk') });
  await writeFile(join(root, 'config.json'), saved);
  await assert.rejects(run(['search', '猫'], { env }), { code: 'LOCAL_CATALOG_UNAVAILABLE' });
  assert.equal(await readFile(join(root, 'config.json'), 'utf8'), saved);
});
