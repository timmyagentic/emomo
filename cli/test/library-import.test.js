import assert from 'node:assert/strict';
import { test } from 'node:test';
import { execFileSync } from 'node:child_process';
import { mkdtemp, writeFile, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { run } from '../src/cli.js';
import { PNG } from './helpers.js';

test('one maintenance import accepts both reviewed formats and preserves original bytes and aliases', async t => {
  const root = await mkdtemp(join(tmpdir(), 'emomo-maintenance-import-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const image = join(root, 'original.png'); await writeFile(image, PNG);
  const sha256 = createHash('sha256').update(PNG).digest('hex');
  const standard = join(root, 'standard.json');
  await writeFile(standard, JSON.stringify([{ id: 'cat', category: 'usable', description: '猫生气', intent_tags: ['猫', '生气'], file_path: image, sha256, width: 1, height: 1, frames: 1, media_kind: 'image' }]));
  const reviewed = join(root, 'reviewed.jsonl');
  const row = { id: 'cat', canonical_id: 'cat', disposition: 'canonical', decision: 'keep', searchable: true, ocr_review: 'verified_visible_text', image_text: '', search_text: '', tags: ['猫', '生气'], path: image, sha256, width: 1, height: 1, frames: 1 };
  await writeFile(reviewed, [row, { ...row, id: 'alias', disposition: 'duplicate_alias', searchable: false }].map(JSON.stringify).join('\n'));
  const binary = fileURLToPath(new URL('../bin/emomo-library.js', import.meta.url));
  for (const [name, source] of [['ordinary', standard], ['reviewed', reviewed]]) {
    const target = join(root, name);
    const result = JSON.parse(execFileSync(process.execPath, [binary, 'import', source, '--dir', target], { encoding: 'utf8' }));
    assert.equal(result.ok, true);
    const detail = await run(['get', 'local-cat', '--catalog', target]);
    assert.equal(detail.data.meme.sha256, sha256);
    assert.deepEqual(await readFile(detail.data.meme.localPath), PNG);
    if (name === 'reviewed') {
      const alias = (await run(['get', 'local-alias', '--catalog', target])).data.meme;
      assert.equal(alias.canonicalId, 'local-cat');
      assert.equal(alias.sha256, sha256);
    }
    assert.throws(() => execFileSync(process.execPath, [binary, 'import', source, '--dir', target], { stdio: 'pipe' }));
  }
  assert.deepEqual(await readFile(image), PNG);
});
