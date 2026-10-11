import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, writeFile, stat, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createHash } from 'node:crypto';
import { importRows, LocalCatalog } from '../src/local.js';
import { prepareSnapshot, validateSnapshot } from '../src/library.js';
import { installSkill } from '../src/skill.js';
import { PNG } from './helpers.js';

test('catalog, snapshot and skill publication preserves private Unix roots and works on Windows', async t => {
  const root = await mkdtemp(join(tmpdir(), 'emomo-private-publish-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  // Reproduce on permissive umask; the caller must not need umask 077 to
  // preserve the privacy previously provided by the mkdtemp staging root.
  const previous = process.umask(0o022);
  t.after(() => process.umask(previous));
  const source = join(root, 'source.png');
  await writeFile(source, PNG);
  const catalogPath = join(root, 'catalog');
  await importRows([{ id: 1, description: 'fixture', category: 'usable',
    width: 1, height: 1, frames: 1, media_kind: 'image', file_path: source,
    sha256: createHash('sha256').update(PNG).digest('hex') }], catalogPath);
  const snapshot = await prepareSnapshot(catalogPath, join(root, 'snapshot'), { collection: 'fixture', revision: 'one' });
  const skill = await installSkill({ directory: join(root, 'skills') });
  for (const directory of [catalogPath, snapshot.path, skill.path]) {
    const info = await stat(directory);
    assert.equal(info.isDirectory(), true);
    // Windows uses inherited ACLs, not Unix mode bits; do not falsely assert
    // ACL privacy from stat.mode. All publication/readback still runs there.
    if (process.platform !== 'win32') assert.equal(info.mode & 0o777, 0o700, directory);
  }
  const catalog = await LocalCatalog.open(catalogPath);
  try { assert.deepEqual(await readFile((await catalog.get('local-1')).meme.localPath), PNG); }
  finally { catalog.close(); }
  assert.equal((await validateSnapshot(snapshot.path)).report.hashesVerified, true);
  assert.match(await readFile(join(skill.path, 'SKILL.md'), 'utf8'), /name: emomo/);
});
