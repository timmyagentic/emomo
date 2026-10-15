import { homedir } from 'node:os';
import { join, resolve } from 'node:path';
import { readFile, mkdir, writeFile, rename, unlink } from 'node:fs/promises';
import { randomUUID } from 'node:crypto';
import { EmomoError } from './error.js';
import { LocalCatalog } from './local.js';
import { apiUrl, DEFAULT_API_URL } from './client.js';

function configDirectory(env) { return resolve(env.EMOMO_CONFIG_DIR || join(env.XDG_CONFIG_HOME || join(homedir(), '.config'), 'emomo')); }
export async function readConfig(env) {
  try {
    const bytes = await readFile(join(configDirectory(env), 'config.json'));
    if (bytes.length > 65536) throw new Error('size');
    const data = JSON.parse(bytes.toString('utf8'));
    if (!data || typeof data !== 'object' || Array.isArray(data) || data.catalog !== undefined && typeof data.catalog !== 'string' || data.apiUrl !== undefined && typeof data.apiUrl !== 'string' || data.catalog && data.apiUrl) throw new Error('shape');
    return data;
  } catch (error) {
    if (error.code === 'ENOENT') return {};
    throw new EmomoError('INVALID_CONFIG', 'The local CLI configuration is invalid. Review config.json.');
  }
}
export async function selectLibrary(flags, env) {
  if (flags.catalog && flags['api-url']) throw new EmomoError('INVALID_ARGUMENT', '--catalog and --api-url cannot be combined.');
  const local = (path, selectedBy) => ({ type: 'local', path: resolve(path), selectedBy });
  const remote = (url, selectedBy) => ({ type: 'remote', apiUrl: apiUrl(url).href.replace(/\/+$/, ''), selectedBy });
  if (flags.catalog) return local(flags.catalog, '--catalog');
  if (flags['api-url']) return remote(flags['api-url'], '--api-url');
  if (env.EMOMO_CATALOG) return local(env.EMOMO_CATALOG, 'EMOMO_CATALOG');
  if (env.EMOMO_API_URL) return remote(env.EMOMO_API_URL, 'EMOMO_API_URL');
  const config = await readConfig(env);
  if (config.catalog) return local(config.catalog, 'config');
  return remote(config.apiUrl || DEFAULT_API_URL, config.apiUrl ? 'config' : 'default');
}
export async function useCatalog(path, env) {
  const remote = /^https?:\/\//i.test(path);
  const selection = remote ? { apiUrl: apiUrl(path).href.replace(/\/+$/, '') } : { catalog: resolve(path) };
  if (!remote) {
    const catalog = await LocalCatalog.open(path);
    try { await catalog.verify(); } finally { catalog.close(); }
  }
  const directory = configDirectory(env), config = await readConfig(env), temporary = join(directory, `.config-${randomUUID()}.tmp`);
  delete config.catalog;
  delete config.apiUrl;
  await mkdir(directory, { recursive: true });
  try {
    await writeFile(temporary, JSON.stringify({ ...config, ...selection }) + '\n', { mode: 0o600, flag: 'wx' });
    await rename(temporary, join(directory, 'config.json'));
  } finally { await unlink(temporary).catch(() => {}); }
  return { ...(remote ? selection : { path: selection.catalog }), configPath: join(directory, 'config.json'), source: remote ? 'remote' : 'local' };
}
