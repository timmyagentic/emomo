import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { test } from 'node:test';
import { staticImageInfo } from '../src/image-info.js';
import { PNG } from './helpers.js';
const fixture = name => readFileSync(new URL(`./fixtures/${name}`, import.meta.url));
const baseline = fixture('static-baseline.jpg'), progressive = fixture('static-progressive.jpg'), webp = fixture('static-lossless.webp');
const trailer = Buffer.from('legacy-source-metadata');

test('complete image streams accept source trailers without changing original bytes', () => {
  for (const [bytes, expected] of [[PNG, { width: 1, height: 1, format: 2 }], [baseline, { width: 16, height: 16, format: 1 }], [progressive, { width: 16, height: 16, format: 1 }], [webp, { width: 16, height: 16, format: 3 }]]) {
    const original = Buffer.concat([bytes, trailer]), before = createHash('sha256').update(original).digest('hex');
    assert.deepEqual(staticImageInfo(original), expected);
    assert.equal(createHash('sha256').update(original).digest('hex'), before);
  }
});

test('trailers do not hide missing image terminators or incomplete RIFF chunks', () => {
  for (const bytes of [baseline, progressive]) {
    assert.throws(() => staticImageInfo(Buffer.concat([bytes.subarray(0, -2), trailer])));
    // An EOI-like value in an APP segment is not the end of an entropy scan.
    const comment = Buffer.from([255, 0xe1, 0, 4, 255, 0xd9]);
    assert.throws(() => staticImageInfo(Buffer.concat([bytes.subarray(0, 2), comment, bytes.subarray(2, -2), trailer])));
  }
  assert.throws(() => staticImageInfo(Buffer.concat([baseline.subarray(0, -2), Buffer.from([255, 0, 0xd9])])));
  assert.throws(() => staticImageInfo(Buffer.concat([PNG.subarray(0, -12), trailer])));
  assert.throws(() => staticImageInfo(webp.subarray(0, -4)));
  const wrongLength = Buffer.concat([webp, Buffer.from([0, 0])]); wrongLength.writeUInt32LE(wrongLength.length - 8, 4);
  assert.throws(() => staticImageInfo(wrongLength));
});
