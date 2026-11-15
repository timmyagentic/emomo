import { readFile, lstat, mkdir, writeFile, rename, unlink } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { homedir } from 'node:os';
import { randomUUID } from 'node:crypto';
import { canonicalApi } from './auth.js';
import { EmomoError } from './error.js';
import { SHA256 } from './import-metadata.js';

export const IMPORT_API = 'https://imports.timmyagentic.si/admin/v1';
const keyPattern = /^emomo_import_[a-f0-9]{64}$/;
const configPath = env => join(resolve(env.EMOMO_CONFIG_DIR || join(env.XDG_CONFIG_HOME || join(homedir(), '.config'), 'emomo')), 'import-access.json');
const invalid = () => new EmomoError('IMPORT_ACCESS_REQUIRED', 'Configure maintenance access with emomo-library connect --key-file PRIVATE_FILE.');
async function privateFile(path) {
  const info = await lstat(path);
  if (!info.isFile() || info.isSymbolicLink() || info.size > 8192 || process.platform !== 'win32' && (info.mode & 0o077)) throw invalid();
  return readFile(path, 'utf8');
}
export async function connectImport(keyFile, api = IMPORT_API, env = process.env) {
  const token = (await privateFile(resolve(keyFile))).trim();
  if (!keyPattern.test(token)) throw invalid();
  const apiUrl = canonicalApi(api), path = configPath(env);
  await mkdir(resolve(path, '..'), { recursive: true, mode: 0o700 });
  const temporary = `${path}.${randomUUID()}`;
  try {
    await writeFile(temporary, JSON.stringify({ apiUrl, token }) + '\n', { flag: 'wx', mode: 0o600 });
    await rename(temporary, path);
  } finally { await unlink(temporary).catch(() => {}); }
  return { apiUrl, configPath: path, configured: true };
}
export async function configuredImportClient(env = process.env) {
  let data;
  try { data = JSON.parse(await privateFile(configPath(env))); }
  catch (error) { if (error.code === 'ENOENT') return null; throw invalid(); }
  if (!keyPattern.test(data.token)) throw invalid();
  return new ImportClient(canonicalApi(data.apiUrl), data.token);
}
export class ImportClient {
  constructor(apiUrl, token, transport = fetch) {
    this.apiUrl = canonicalApi(apiUrl);
    if (!keyPattern.test(token)) throw invalid();
    this.token = token;
    this.transport = transport;
  }
  async request(path, body) {
    const headers = { authorization: `Bearer ${this.token}` };
    if (!(body instanceof FormData)) { headers['content-type'] = 'application/json'; body = JSON.stringify(body); }
    let response;
    try { response = await this.transport(`${this.apiUrl}/${path}`, { method: 'POST', headers, body, redirect: 'manual', signal: AbortSignal.timeout(90000) }); }
    catch { throw new EmomoError('IMPORT_NETWORK_ERROR', 'Import response unavailable. Repeat publish with the same confirmation to resume.', { retryable: true }); }
    if (response.status >= 300 && response.status < 400) { await response.body?.cancel(); throw new EmomoError('IMPORT_REDIRECT', 'Maintenance redirects are rejected. Configure the final endpoint.'); }
    const chunks = [];
    let size = 0;
    for await (const chunk of response.body ?? []) {
      size += chunk.length;
      if (size > 2 * 1024 * 1024) throw new EmomoError('IMPORT_RESPONSE_INVALID', 'Maintenance response exceeds the size limit.');
      chunks.push(Buffer.from(chunk));
    }
    let result;
    try { result = JSON.parse(Buffer.concat(chunks).toString('utf8')); }
    catch { throw new EmomoError('IMPORT_RESPONSE_INVALID', 'Maintenance response is invalid.'); }
    if (!response.ok) throw new EmomoError('IMPORT_REJECTED', `Maintenance request failed (HTTP ${response.status}).`, { httpStatus: response.status, retryable: response.status >= 500 });
    return result;
  }
  async lookup(hashes) {
    const existing = new Map();
    for (let offset = 0; offset < hashes.length; offset += 100) {
      const input = hashes.slice(offset, offset + 100);
      const response = await this.request('lookup', { sha256: input });
      if (!Array.isArray(response.existing)) throw new EmomoError('IMPORT_RESPONSE_INVALID', 'Missing duplicate lookup results.');
      for (const meme of response.existing) {
        if (!SHA256.test(meme.content_hash) || !input.includes(meme.content_hash) || typeof meme.id !== 'string') throw new EmomoError('IMPORT_RESPONSE_INVALID', 'Invalid duplicate lookup result.');
        if (!existing.has(meme.content_hash)) existing.set(meme.content_hash, meme);
      }
    }
    return existing;
  }
  async upload(metadata, bytes) {
    const body = new FormData();
    body.set('metadata', JSON.stringify(metadata));
    body.set('image', new Blob([bytes]), `${metadata.sha256}.${['', 'jpg', 'png', 'webp'][metadata.image_info.format]}`);
    const result = await this.request('imports', body);
    if (result.meme?.content_hash !== metadata.sha256 || typeof result.meme?.id !== 'string' || typeof result.duplicate !== 'boolean') throw new EmomoError('IMPORT_RESPONSE_INVALID', 'Import receipt does not match the uploaded image.');
    return result;
  }
}
