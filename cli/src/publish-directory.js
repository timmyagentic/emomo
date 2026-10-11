import { mkdir, readdir, rename, rm } from 'node:fs/promises';
import { join } from 'node:path';

// Reserve exclusively, then populate that directory: Windows cannot rename a
// directory over even an empty directory. Publish the reader's manifest last,
// so interrupted installs fail closed instead of exposing incomplete content.
export async function publishDirectory(stage, target, manifest) {
  await mkdir(target);
  try {
    const names = await readdir(stage);
    names.sort((a, b) => Number(a === manifest) - Number(b === manifest));
    for (const name of names) await rename(join(stage, name), join(target, name));
  } catch (error) {
    await rm(target, { recursive: true, force: true });
    throw error;
  }
}
