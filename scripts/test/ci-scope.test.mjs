import assert from 'node:assert/strict';
import { test } from 'node:test';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

test('actual commit diffs scope retained product checks; schema changes and unknown history run all gates', t => {
  const root = mkdtempSync(join(tmpdir(), 'emomo-ci-scope-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const git = (...args) => execFileSync('git', args, { cwd: root, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  git('init'); git('config', 'user.name', 'Fixture'); git('config', 'user.email', 'fixture@example.invalid');
  const commit = path => { mkdirSync(dirname(join(root, path)), { recursive: true }); writeFileSync(join(root, path), 'fixture'); git('add', '.'); git('-c', 'commit.gpgSign=false', 'commit', '-m', 'fixture'); return git('rev-parse', 'HEAD'); };
  const first = commit('README.md'), cli = commit('cli/src/fixture.js');
  const scope = (base, head, event = 'pull_request') => Object.fromEntries(execFileSync(process.execPath, [fileURLToPath(new URL('../ci-scope.mjs', import.meta.url))], { cwd: root, env: { ...process.env, BASE_SHA: base, HEAD_SHA: head, EVENT_NAME: event, GITHUB_OUTPUT: '' }, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim().split('\n').map(line => line.split('=')));
  assert.deepEqual(scope(first, cli), { backend: 'false', frontend: 'false', gateway: 'false' });
  const backend = commit('backend/internal/fixture.go');
  assert.deepEqual(scope(cli, backend), { backend: 'true', frontend: 'false', gateway: 'false' });
  const schema = commit('backend/proto/emomo/v1/fixture.proto');
  const all = { backend: 'true', frontend: 'true', gateway: 'true' };
  assert.deepEqual(scope(backend, schema), all);
  assert.deepEqual(scope(first, cli, 'workflow_dispatch'), all);
  assert.deepEqual(scope('0'.repeat(40), cli), all);
  assert.deepEqual(scope('f'.repeat(40), cli), all);
});
