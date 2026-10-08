import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtemp, readFile, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { indexRow, importSQL } from '../src/metadata.js';
import { indexTerms, matchExpression } from '../src/terms.js';

const execute = promisify(execFile);
const input = { meme: { id: 'cat-1', storage_key: 'cat-1.png', image_info: { width: 1, height: 1, format: 2 } }, annotation: { meme_id: 'cat-1', description: '猫咪无语', ocr_text: '谢谢老板' } };

test('Chinese two-character terms and single-character subjects are deterministic; FTS syntax is literal', () => {
  assert.ok(indexTerms('猫咪露出无语的表情').split(' ').includes('无语'));
  assert.ok(indexTerms('猫咪露出无语的表情').split(' ').includes('猫'));
  assert.equal(matchExpression('无语 猫'), '"无语" OR "猫"');
  assert.equal(matchExpression('ＨＥＬＬＯ'), '"hello"');
  assert.equal(matchExpression('猫" OR *'), '"猫" OR "or"');
  assert.throws(() => matchExpression('***'));
  assert.throws(() => matchExpression(Array.from({ length: 65 }, (_, i) => `word${i}`).join(' ')));
});

test('canonical metadata is reused, unknown OCR stays unknown, no signed URLs or bad IDs imported', () => {
  const row = indexRow(input);
  assert.equal(row.text_presence, 2);
  assert.equal(row.description, '猫咪无语');
  assert.equal(indexRow({ meme: input.meme }).text_presence, 1);
  assert.equal(indexRow({ ...input, annotation: { meme_id: 'cat-1', ocr_text: ' !!! 🐈 ' } }).text_presence, 3);
  for (const ocr_text of ['无文字', 'None', '"no text"']) {
    const empty = indexRow({ ...input, annotation: { meme_id: 'cat-1', ocr_text } });
    assert.equal(empty.text_presence, 3);
    assert.equal(empty.ocr_terms, '');
  }
  assert.throws(() => indexRow({ meme: { ...input.meme, url: 'https://example.test/signed' } }));
  assert.throws(() => indexRow({ meme: { ...input.meme, id: '../cat' } }));
  assert.throws(() => indexRow({ meme: { ...input.meme, storage_key: '../cat.png' } }));
  assert.throws(() => indexRow({ ...input, annotation: { meme_id: 'another' } }));
  assert.throws(() => indexRow({ ...input, model_key: 'unsupported' }));
  assert.ok(importSQL(indexRow({ ...input, annotation: { meme_id: 'cat-1', description: "it's a cat" } })).includes("it''s a cat"));
});

test('stored text-presence labels survive import even when OCR is missing or disagrees', () => {
  // The actual retained library contains label-only annotations, including
  // has_text=true with no OCR. Deriving the filter from OCR loses that fact.
  assert.equal(indexRow({ ...input, annotation: { meme_id: 'cat-1', labels: { has_text: true } } }).text_presence, 2);
  const withoutText = indexRow({ ...input, annotation: { meme_id: 'cat-1', labels: { has_text: false }, ocr_text: 'word' } });
  assert.equal(withoutText.text_presence, 3);
  assert.equal(withoutText.ocr_text, 'word');
  assert.equal(indexRow({ ...input, annotation: { meme_id: 'cat-1', ocr_text: 'word' } }).text_presence, 2);
});

test('offline importer builds private SQL, refuses overwrite and removes failed partial output', async t => {
  const directory = await mkdtemp(join(tmpdir(), 'emomo-index-'));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const source = join(directory, 'metadata.jsonl');
  const output = join(directory, 'index.sql');
  await writeFile(source, JSON.stringify(input));
  const run = (target: string) => execute(process.execPath, ['--import', 'tsx', 'scripts/build-index.ts', source, target]);
  assert.equal(JSON.parse((await run(output)).stdout).modelCalls, 0);
  assert.match(await readFile(output, 'utf8'), /INSERT INTO memes/);
  await assert.rejects(run(output));
  await writeFile(source, `${JSON.stringify(input)}\n{"invalid":true}`);
  const failed = join(directory, 'failed.sql');
  await assert.rejects(run(failed));
  await assert.rejects(readFile(failed), { code: 'ENOENT' });
});

test('private reviewed text states and aliases survive without turning aliases into public facts', () => {
  const row = indexRow({ ...input, text_presence: 1, search_aliases: ['阴阳怪气'], annotation: { meme_id: 'cat-1', description: '完整描述', ocr_text: '' } });
  assert.equal(row.text_presence, 1); assert.match(row.tag_terms, /阴阳/);
  assert.deepEqual(JSON.parse(row.meme_json).tags ?? [], []);
  for (const text_presence of [0, 4, '1']) assert.throws(() => indexRow({ ...input, text_presence }));
  assert.throws(() => indexRow({ ...input, text_presence: 1, annotation: { meme_id:'cat-1', labels: { has_text:true } } }));
  assert.throws(() => indexRow({ ...input, search_aliases: ['x'.repeat(129)] }));
  assert.throws(() => indexRow({ ...input, search_aliases: [17] }));
});
