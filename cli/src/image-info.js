import { Buffer } from 'node:buffer';
import { MAX_IMPORT_IMAGE_BYTES } from './import-metadata.js';

// Validate static containers in Node and Workers. Visual review still belongs
// to the Agent; parsing headers does not establish decoded image quality.
export function staticImageInfo(input) {
  const bytes = Buffer.from(input.buffer, input.byteOffset, input.byteLength);
  const invalid = () => new Error('Use a complete static PNG, JPEG or WebP image, at most 25 MiB.');
  if (!bytes.length || bytes.length > MAX_IMPORT_IMAGE_BYTES) throw invalid();
  let width = 0, height = 0, format = 0;
  if (bytes.subarray(0, 8).equals(Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]))) {
    let end = false, data = false;
    for (let offset = 8; offset + 12 <= bytes.length;) {
      const size = bytes.readUInt32BE(offset), type = bytes.toString('ascii', offset + 4, offset + 8);
      if (size > bytes.length - offset - 12 || type === 'acTL') throw invalid();
      if (offset === 8) {
        if (type !== 'IHDR' || size !== 13) throw invalid();
        width = bytes.readUInt32BE(offset + 8); height = bytes.readUInt32BE(offset + 12);
      }
      if (type === 'IDAT') data = true;
      offset += size + 12;
      if (type === 'IEND') { end = size === 0 && offset === bytes.length; break; }
    }
    if (!end || !data) throw invalid();
    format = 2;
  } else if (bytes[0] === 255 && bytes[1] === 216) {
    if (bytes[bytes.length - 2] !== 255 || bytes[bytes.length - 1] !== 217) throw invalid();
    let offset = 2, scan = false;
    while (offset < bytes.length - 2) {
      if (bytes[offset++] !== 255) throw invalid();
      while (bytes[offset] === 255) offset++;
      const marker = bytes[offset++];
      if (marker === 0xda) { scan = true; break; }
      if (marker === 0x01 || marker >= 0xd0 && marker <= 0xd7) continue;
      if (offset + 2 > bytes.length) throw invalid();
      const size = bytes.readUInt16BE(offset);
      if (size < 2 || offset + size > bytes.length) throw invalid();
      if ([0xc0, 0xc1, 0xc2].includes(marker)) {
        if (size < 8) throw invalid();
        height = bytes.readUInt16BE(offset + 3); width = bytes.readUInt16BE(offset + 5);
      }
      offset += size;
    }
    if (!scan) throw invalid();
    format = 1;
  } else if (bytes.toString('ascii', 0, 4) === 'RIFF' && bytes.toString('ascii', 8, 12) === 'WEBP') {
    if (bytes.readUInt32LE(4) + 8 !== bytes.length) throw invalid();
    let data = false;
    for (let offset = 12; offset + 8 <= bytes.length;) {
      const type = bytes.toString('ascii', offset, offset + 4), size = bytes.readUInt32LE(offset + 4), start = offset + 8;
      if (size > bytes.length - start || ['ANIM', 'ANMF'].includes(type)) throw invalid();
      if (type === 'VP8X') {
        if (size < 10 || bytes[start] & 2) throw invalid();
        width = bytes.readUIntLE(start + 4, 3) + 1; height = bytes.readUIntLE(start + 7, 3) + 1;
      } else if (type === 'VP8 ' && size >= 10) {
        if (!bytes.subarray(start + 3, start + 6).equals(Buffer.from([0x9d, 1, 0x2a]))) throw invalid();
        width = bytes.readUInt16LE(start + 6) & 0x3fff; height = bytes.readUInt16LE(start + 8) & 0x3fff; data = true;
      } else if (type === 'VP8L' && size >= 5 && bytes[start] === 0x2f) {
        const bits = bytes.readUInt32LE(start + 1);
        width = (bits & 0x3fff) + 1; height = ((bits >>> 14) & 0x3fff) + 1; data = true;
      }
      offset = start + size + (size % 2);
      if (offset > bytes.length) throw invalid();
    }
    if (!data) throw invalid();
    format = 3;
  }
  if (!format || !width || !height || width > 100000 || height > 100000) throw invalid();
  return { width, height, format };
}
