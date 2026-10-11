import { readFile, lstat, mkdir, writeFile, rename, unlink } from 'node:fs/promises';
import { homedir } from 'node:os';
import { join, resolve } from 'node:path';
import { randomUUID } from 'node:crypto';
import { EmomoError } from './error.js';
import { DEFAULT_API_URL } from './client.js';

const location = env => join(resolve(env.EMOMO_CONFIG_DIR || join(env.XDG_CONFIG_HOME || join(homedir(), '.config'), 'emomo')), 'access.json');
const fail = () => new EmomoError('INVALID_AUTH_CONFIG', 'Cannot read private access configuration. Use emomo auth configure --key-file with a private key file.');
export function canonicalApi(value) {
  const url = new URL(value);
  if (url.protocol !== 'https:' || url.username || url.password || url.search || url.hash) throw fail();
  return url.origin + url.pathname.replace(/\/+$/, '');
}
async function privateText(path) {
  const stat = await lstat(path);
  if (!stat.isFile() || stat.isSymbolicLink() || stat.size > 8192 || process.platform !== 'win32' && (stat.mode & 0o077)) throw fail();
  return readFile(path, 'utf8');
}
export async function readAccess(env) {
  try {
    const data = JSON.parse(await privateText(location(env)));
    if (!data || typeof data.token !== 'string' || !/^emomo_[a-zA-Z0-9-]{1,48}_[a-f0-9]{64}$/.test(data.token)) throw fail();
    return { apiUrl: canonicalApi(data.apiUrl), token: data.token };
  } catch (error) {
    if (error.code === 'ENOENT') return null;
    throw fail();
  }
}
export async function configureAccess(keyFile, apiUrl = DEFAULT_API_URL, env) {
  if (!keyFile) throw new EmomoError('INVALID_ARGUMENT', 'Provide --key-file pointing to a private access-key file. Do not pass the key on the command line.');
  let token;
  try { token = (await privateText(resolve(keyFile))).trim(); } catch { throw fail(); }
  if (!/^emomo_[a-zA-Z0-9-]{1,48}_[a-f0-9]{64}$/.test(token)) throw fail();
  const api = canonicalApi(apiUrl), target = location(env), directory = resolve(target, '..');
  await mkdir(directory, { recursive: true, mode: 0o700 });
  const temp = join(directory, `.access-${randomUUID()}.tmp`);
  try {
    await writeFile(temp, JSON.stringify({ apiUrl: api, token }) + '\n', { flag: 'wx', mode: 0o600 });
    await rename(temp, target);
  } finally { await unlink(temp).catch(() => {}); }
  return { configured: true, apiUrl: api, configPath: target };
}
export async function accessStatus(env) {
  const data = await readAccess(env);
  return { configured: !!data, ...(data ? { apiUrl: data.apiUrl } : {}) };
}
export async function clearAccess(env) {
  await unlink(location(env)).catch(error => { if (error.code !== 'ENOENT') throw fail(); });
  return { configured: false };
}
