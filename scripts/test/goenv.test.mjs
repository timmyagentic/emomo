import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { developmentEnv } from '../dev.mjs';

test('real Go ignores persistent caller configuration with an isolated HOME', t => {
  const root = mkdtempSync(join(tmpdir(), 'emomo-goenv-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const fixture = join(root, 'go-env');
  const flags = '-tags=emomo_private_fixture';
  const proxy = 'https://proxy.invalid/emomo-fixture';
  writeFileSync(fixture, `GOFLAGS=${flags}\nGOPROXY=${proxy}\n`);
  const isolated = developmentEnv(root, { PATH: process.env.PATH, HOME: join(root, 'fixture-home'), GOENV: fixture });
  mkdirSync(isolated.HOME, { recursive: true });
  const readConfig = env => {
    // Use the installed Go runtime; this test never downloads a toolchain/module.
    const result = spawnSync('go', ['env', '-json', 'GOENV', 'GOFLAGS', 'GOPROXY', 'GOAUTH'], {
      cwd: root, env: { ...env, GOTOOLCHAIN: 'local' }, encoding: 'utf8',
    });
    assert.ifError(result.error);
    assert.equal(result.status, 0, result.stderr);
    return JSON.parse(result.stdout);
  };
  // Explicit fixture path exercises Go's real persistent config reader, avoiding
  // all reads/writes of the user's default configuration using an isolated fixture HOME.
  const baseline = readConfig({ ...isolated, GOENV: fixture });
  assert.equal(baseline.GOFLAGS, flags);
  assert.equal(baseline.GOPROXY, proxy);
  // Fail BEFORE invoking Go if protection is missing: never read the real user file.
  assert.equal(isolated.GOENV, 'off');
  assert.equal(isolated.HOME, join(root, '.dev-cache', 'home'));
  assert.equal(isolated.GOAUTH, 'off');
  const actual = readConfig(isolated);
  assert.equal(actual.GOENV, ''); // Go reports no configuration file when disabled.
  assert.equal(actual.GOFLAGS, '');
  assert.equal(actual.GOAUTH, 'off');
  assert.notEqual(actual.GOPROXY, proxy);
});
