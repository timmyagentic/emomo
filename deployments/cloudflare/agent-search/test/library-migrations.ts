import { readFile, readdir } from 'node:fs/promises';
import type { Miniflare } from 'miniflare';

export async function applyLibraryMigrations(db: Awaited<ReturnType<Miniflare['getD1Database']>>) {
  for (const file of (await readdir('migrations')).filter(name => name.endsWith('.sql')).sort()) {
    const contents = await readFile(`migrations/${file}`, 'utf8');
    const statements = contents.split('-- statement').map(sql => sql.trim()).filter(Boolean);
    await db.batch(statements.map(sql => db.prepare(sql)));
  }
}
