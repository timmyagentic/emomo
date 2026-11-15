import { lstat, chmod } from 'node:fs/promises';
import { join } from 'node:path';
import { EmomoError } from './error.js';

// SQLite's process lock is released even if the CLI is killed. The dedicated
// file contains no catalog data; it prevents concurrent commands from submitting
// the same paid Batch or overwriting one another's annotation checkpoints.
export async function withImportLock(directory, action) {
  const path = join(directory, 'import-lock.sqlite');
  try {
    const info = await lstat(path);
    if (!info.isFile() || info.isSymbolicLink()) throw new EmomoError('INVALID_IMPORT_LOCK', 'The import lock must be a regular file.');
  } catch (error) { if (error.code !== 'ENOENT') throw error; }
  const { DatabaseSync } = await import('node:sqlite');
  const database = new DatabaseSync(path);
  try {
    await chmod(path, 0o600);
    try { database.exec('PRAGMA busy_timeout=0; BEGIN EXCLUSIVE'); }
    catch (error) {
      if ([5, 6].includes(error.errcode)) throw new EmomoError('IMPORT_BUSY', 'Another command is processing this batch directory. Retry when it finishes.');
      throw error;
    }
    return await action();
  } finally { database.close(); }
}
