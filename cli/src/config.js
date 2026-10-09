import { homedir } from 'node:os';
import { join, resolve } from 'node:path';
import { readFile, mkdir, writeFile, rename, unlink } from 'node:fs/promises';
import { randomUUID } from 'node:crypto';
import { EmomoError } from './error.js';
import { LocalCatalog } from './local.js';

function configDirectory(env) { return resolve(env.EMOMO_CONFIG_DIR || join(env.XDG_CONFIG_HOME || join(homedir(), '.config'), 'emomo')); }
export async function readConfig(env) {
  try {
    const bytes = await readFile(join(configDirectory(env), 'config.json'));
    if (bytes.length > 65536) throw new Error('size');
    const data = JSON.parse(bytes.toString('utf8'));
    if (!data || typeof data !== 'object' || Array.isArray(data) || data.catalog !== undefined && typeof data.catalog !== 'string') throw new Error('shape');
    return data;
  } catch (error) {
    if (error.code === 'ENOENT') return {};
    throw new EmomoError('INVALID_CONFIG', 'The local CLI configuration is invalid. Review config.json.');
  }
}
export async function selectCatalog(flags, env) {
  if (flags.catalog && flags['api-url']) throw new EmomoError('INVALID_ARGUMENT', '--catalog and --api-url cannot be combined.');
  if (flags.catalog) return resolve(flags.catalog);
  if (flags['api-url']) return undefined;
  if (env.EMOMO_CATALOG) return resolve(env.EMOMO_CATALOG);
  if (env.EMOMO_API_URL) return undefined;
  return (await readConfig(env)).catalog;
}
export async function useCatalog(path, env) {
  const catalog = await LocalCatalog.open(path);
  try { await catalog.verify(); } finally { catalog.close(); }
  const directory = configDirectory(env), config = await readConfig(env), temporary = join(directory, `.config-${randomUUID()}.tmp`);
  await mkdir(directory, { recursive: true });
  try {
    await writeFile(temporary, JSON.stringify({ ...config, catalog: resolve(path) }) + '\n', { mode: 0o600, flag: 'wx' });
    await rename(temporary, join(directory, 'config.json'));
  } finally { await unlink(temporary).catch(() => {}); }
  return { path: resolve(path), configPath: join(directory, 'config.json'), source: 'local' };
}
