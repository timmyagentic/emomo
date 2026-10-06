import assert from 'node:assert/strict';
import { test } from 'node:test';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { PNG, serve, json, wireMeme } from './helpers.js';

const exec = promisify(execFile);
const root = fileURLToPath(new URL('../', import.meta.url));

test('packed npm install includes the CLI and skill, and performs search -> detail -> image download', { timeout: 30000 }, async t => {
  const temporary = await mkdtemp(join(tmpdir(), 'emomo-install-e2e-'));
  t.after(() => rm(temporary, { recursive: true, force: true }));
  const packed = JSON.parse((await exec('npm', ['pack', '--pack-destination', temporary, '--json'], { cwd: root })).stdout)[0];
  const paths = packed.files.map(file => file.path);
  assert.ok(paths.includes('bin/emomo.js'));
  assert.ok(paths.includes('skills/emomo/SKILL.md'));
  assert.ok(paths.includes('LICENSE'));
  assert.ok(paths.every(path => !path.includes('.env') && !path.startsWith('test/')));
  const prefix = join(temporary, 'installed');
  await exec('npm', ['install', '--global', '--prefix', prefix, '--ignore-scripts', '--no-audit', '--no-fund', join(temporary, packed.filename)]);
  const binary = process.platform === 'win32' ? join(prefix, 'emomo.cmd') : join(prefix, 'bin', 'emomo');
  let origin;
  const api = await serve((request, response, body) => {
    if (request.url === '/agent/v1/search') {
      assert.equal(JSON.parse(body).query, '开会想下班');
      json(response, { query: '开会想下班', results: [{ meme: wireMeme(origin), description: '本地协议测试样本', score: 0.9 }], total: 1 });
    } else if (request.url === '/agent/v1/memes/meme-1') json(response, { meme: wireMeme(origin) });
    else if (request.url === '/image.png') { response.writeHead(200, { 'Content-Type': 'image/png' }); response.end(PNG); }
    else json(response, { error: 'not found' }, 404);
  }, t);
  origin = api.origin;
  const env = { ...process.env, EMOMO_API_URL: `${origin}/agent/v1`, EMOMO_API_TOKEN: '', CODEX_HOME: join(temporary, 'codex') };
  const invoke = async args => JSON.parse((await exec(binary, args, { env })).stdout);
  const installed = await invoke(['skill', 'install']);
  assert.match(await readFile(join(installed.data.path, 'SKILL.md'), 'utf8'), /emomo download/);
  const search = await invoke(['search', '开会想下班', '--limit', '3']);
  assert.equal(search.ok, true);
  const detail = await invoke(['get', search.data.results[0].id]);
  assert.equal(detail.data.meme.id, 'meme-1');
  const download = await invoke(['download', detail.data.meme.id, '--dir', join(temporary, 'images')]);
  assert.deepEqual(await readFile(download.data.path), PNG);
  assert.equal(download.data.mimeType, 'image/png');
  assert.equal((await invoke(['capabilities'])).data.version, '0.1.1');
});
