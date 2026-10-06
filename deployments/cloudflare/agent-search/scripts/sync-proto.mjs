import { readFile, writeFile, mkdir } from 'node:fs/promises';

// These files are generated from backend/proto/emomo/v1, never hand-edited here.
for (const name of ['api', 'meme', 'types']) {
  const source = new URL(`../../../../frontend/gen/emomo/v1/${name}_pb.ts`, import.meta.url);
  const target = new URL(`../gen/emomo/v1/${name}_pb.ts`, import.meta.url);
  const content = await readFile(source);
  if (process.argv.includes('--check')) {
    if (!(await readFile(target)).equals(content)) throw new Error('Canonical protobuf output is out of sync; regenerate and run gen:sync.');
  } else {
    await mkdir(new URL('../gen/emomo/v1/', import.meta.url), { recursive: true });
    await writeFile(target, content);
  }
}
