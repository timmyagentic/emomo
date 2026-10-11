import { test } from 'node:test';
import assert from 'node:assert/strict';
import { setImmediate } from 'node:timers/promises';
import { mkdtemp, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createHash } from 'node:crypto';
import { importRows, LocalCatalog } from '../src/local.js';
import { PNG } from './helpers.js';

for (const mode of ['ordinary', 'refined']) {
  test(`${mode} search checks all 100 returned candidates with one active image validation`, async t => {
    const root = await mkdtemp(join(tmpdir(), 'emomo-search-memory-'));
    let catalog;
    t.after(async () => { catalog?.close(); await rm(root, { recursive: true, force: true }); });
    const source = join(root, 'tiny.png');
    await writeFile(source, PNG);
    const rows = Array.from({ length: 100 }, (_, id) => ({ id, category: 'usable',
      description: 'fixture', intent_tags: ['fixture'], media_kind: 'image',
      frames: 1, width: 1, height: 1, file_path: source,
      sha256: createHash('sha256').update(PNG).digest('hex') }));
    const path = join(root, 'catalog');
    await importRows(rows, path, mode === 'refined' ? { searchMode: 'facets' } : {});
    catalog = await LocalCatalog.open(path);
    const original = catalog.checkedImage.bind(catalog);
    let active = 0, peak = 0;
    const checked = [];
    catalog.checkedImage = async item => {
      active++;
      peak = Math.max(peak, active);
      try {
        // Suspend without allocating large buffers, exposing overlapping reads
        // while still exercising the real containment/hash validation below.
        await setImmediate();
        const result = await original(item);
        checked.push(item.id);
        return result;
      } finally { active--; }
    };
    const result = await catalog.search('fixture', { limit: 100 });
    assert.equal(result.results.length, 100);
    assert.deepEqual(checked.slice().sort(), result.results.map(item => item.id).sort());
    assert.equal(active, 0);
    assert.equal(peak, 1);
    // Bounded concurrency must not turn integrity failure into a partial success.
    await writeFile(join(path, 'assets', `${rows[0].sha256}.png`), 'changed');
    await assert.rejects(catalog.search('fixture', { limit: 100 }), { code: 'IMAGE_INTEGRITY_ERROR' });
  });
}
