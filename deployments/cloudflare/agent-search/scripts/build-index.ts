import { createReadStream } from 'node:fs';
import { open, unlink } from 'node:fs/promises';
import { createInterface } from 'node:readline';
import { indexRow, importSQL } from '../src/metadata.js';

const [input, output, ...extra] = process.argv.slice(2);
if (!input || !output || extra.length) throw new Error('Usage: npm run index:build -- <metadata.jsonl> <new-output.sql>');
const file = await open(output, 'wx', 0o600);
let count = 0;
try {
  const lines = createInterface({ input: createReadStream(input), crlfDelay: Infinity });
  try {
    for await (const line of lines) {
      if (!line.trim()) continue;
      if (Buffer.byteLength(line) > 128 * 1024) throw new Error('Metadata line exceeds 128 KiB.');
      const sql = importSQL(indexRow(JSON.parse(line)));
      // Escaped text can contain newlines; stay below D1's per-statement limit.
      if (Buffer.byteLength(sql) > 96 * 1024) throw new Error('Import statement exceeds 96 KiB.');
      await file.write(`${sql}\n`);
      count++;
    }
  } finally { lines.close(); }
  if (!count) throw new Error('No metadata records.');
  await file.close();
  console.log(JSON.stringify({ records: count, output, modelCalls: 0 }));
} catch {
  await file.close();
  await unlink(output);
  throw new Error('Index build failed; validate metadata. Partial output removed.');
}
