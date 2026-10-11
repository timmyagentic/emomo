import { readFile, writeFile } from 'node:fs/promises';
const source = new URL('../../frontend/gen/emomo/v1/account_pb.ts', import.meta.url);
const target = new URL('../gen/emomo/v1/account_pb.ts', import.meta.url);
const data = await readFile(source,'utf8');
if (process.argv.includes('--check')) { if (data !== await readFile(target,'utf8')) throw Error('Regenerate canonical account protobuf DTOs.'); }
else await writeFile(target,data);
