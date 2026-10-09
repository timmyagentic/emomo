import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtemp, readFile, writeFile, mkdir, symlink } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { run } from '../src/cli.js';
import { installSkill } from '../src/skill.js';

test('skill installation follows CODEX_HOME and is idempotent', async () => {
  const home = await mkdtemp(join(tmpdir(), 'emomo-skill-codex-'));
  const result = await run(['skill', 'install'], { env: { CODEX_HOME: home } });
  assert.equal(result.data.path, join(home, 'skills', 'emomo'));
  const text = await readFile(join(result.data.path, 'SKILL.md'), 'utf8');
  assert.match(text, /name: emomo/);
  assert.match(text, /emomo search/);
  assert.equal((await installSkill({ env: { CODEX_HOME: home } })).alreadyInstalled, true);
});

test('Claude and universal skill installs support an explicit skills directory', async () => {
  for (const agent of ['claude', 'agents']) {
    const directory = await mkdtemp(join(tmpdir(), `emomo-skill-${agent}-`));
    const result = await run(['skill', 'install', '--agent', agent, '--dir', directory], { env: {} });
    assert.equal(result.data.path, join(directory, 'emomo'));
    assert.equal(result.data.agent, agent);
  }
});

test('existing user skills, incomplete directories, and symlinks are never replaced', async () => {
  for (const kind of ['custom', 'empty', 'symlink']) {
    const directory = await mkdtemp(join(tmpdir(), 'emomo-skill-preserve-'));
    const path = join(directory, 'emomo');
    if (kind === 'symlink') await symlink(directory, path, 'dir');
    else {
      await mkdir(path);
      if (kind === 'custom') await writeFile(join(path, 'SKILL.md'), 'user-authored instructions');
    }
    await assert.rejects(installSkill({ directory }), error => error.code === 'SKILL_EXISTS');
    if (kind === 'custom') assert.equal(await readFile(join(path, 'SKILL.md'), 'utf8'), 'user-authored instructions');
  }
});

test('concurrent installation reserves a single skill destination', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'emomo-skill-concurrent-'));
  const results = await Promise.allSettled([installSkill({ directory }), installSkill({ directory })]);
  assert.ok(results.some(result => result.status === 'fulfilled'));
  assert.match(await readFile(join(directory, 'emomo', 'SKILL.md'), 'utf8'), /name: emomo/);
});
