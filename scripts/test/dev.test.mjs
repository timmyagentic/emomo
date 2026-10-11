import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { plan, developmentEnv, assertCleanConfig } from '../dev.mjs';

test('invalid requests fail before executing commands', () => {
  assert.throws(() => plan('deploy', 'agent'), /Usage/);
  assert.throws(() => plan('check', 'production'), /Usage/);
});
test('default profile excludes legacy dependencies; full retains mobile generation checks', () => {
  assert.deepEqual(plan('bootstrap', 'agent').map(x => x.cwd), ['deployments/cloudflare/agent-search']);
  const full = plan('check', 'full');
  assert.ok(full.some(x => x.args.join(' ') === 'git diff --exit-code -- gen'));
  assert.ok(full.some(x => x.args.join(' ') === 'go test ./...'));
  for (const action of ['bootstrap', 'check']) for (const profile of ['agent', 'full']) {
    assert.ok(plan(action, profile).every(x => !x.args.includes('deploy') && !x.args.includes('--remote')));
  }
});
test('tool environment excludes credentials and caller config without changing HOME', () => {
  const env = developmentEnv('/checkout', { HOME: '/user', PATH: '/bin', HTTPS_PROXY: 'http://proxy.invalid:8080', NODE_EXTRA_CA_CERTS: '/session/ca.pem', CLOUDFLARE_API_TOKEN: 'fixture', NODE_OPTIONS: '--require=unexpected', npm_config_userconfig: '/user/private' });
  assert.equal(env.GOENV, 'off');
  assert.equal(env.HOME, '/user');
  assert.equal(env.HTTPS_PROXY, 'http://proxy.invalid:8080');
  assert.equal(env.NODE_EXTRA_CA_CERTS, '/session/ca.pem');
  assert.equal(env.CLOUDFLARE_API_TOKEN, undefined);
  assert.equal(env.NODE_OPTIONS, undefined);
  assert.equal(env.npm_config_userconfig, '/dev/null');
  assert.equal(env.npm_config_cache, '/checkout/.dev-cache/npm');
  assert.equal(env.WRANGLER_LOG_PATH, '/checkout/.dev-cache/logs');
});
test('reject environment files by name, including nested tool directory', t => {
  const root = mkdtempSync(join(tmpdir(), 'emomo-dev-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  mkdirSync(join(root, 'worker'));
  writeFileSync(join(root, '.env.example'), 'EXAMPLE=');
  assertCleanConfig(root, ['worker']);
  writeFileSync(join(root, 'worker', '.dev.vars.local'), 'fixture');
  assert.throws(() => assertCleanConfig(root, ['worker']), /Refusing local environment file/);
});
