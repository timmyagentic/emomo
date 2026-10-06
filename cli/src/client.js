import { createHash, randomUUID } from 'node:crypto';
import { link, mkdir, open, unlink } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import { EmomoError } from './error.js';

export const DEFAULT_API_URL = 'https://api.emomo.net/agent/v1';
const MAX_JSON_BYTES = 2 * 1024 * 1024;
const MAX_IMAGE_BYTES = 25 * 1024 * 1024;
const FORMATS = new Map([[1, 'jpeg'], [2, 'png'], [3, 'webp'], ['IMAGE_FORMAT_JPEG', 'jpeg'], ['IMAGE_FORMAT_PNG', 'png'], ['IMAGE_FORMAT_WEBP', 'webp']]);
const TEXT_PRESENCE = new Map([[0, 'unknown'], [1, 'unknown'], [2, 'with_text'], [3, 'without_text']]);

function object(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function field(value, snake, camel = snake) {
  return value[snake] ?? value[camel];
}

function invalidResponse() {
  return new EmomoError('INVALID_RESPONSE', 'The service returned an incompatible response.');
}

function publicUrl(raw, { api = false } = {}) {
  let url;
  try { url = new URL(raw); } catch { throw new EmomoError('INVALID_URL', 'Use an absolute HTTPS URL (HTTP is only allowed for a local development API).'); }
  const local = ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname);
  if (url.username || url.password || url.hash || (url.protocol !== 'https:' && !(url.protocol === 'http:' && api && local))) {
    throw new EmomoError('INVALID_URL', 'Use HTTPS without embedded credentials; local development APIs may use HTTP.');
  }
  return url;
}

function apiUrl(raw) {
  const url = publicUrl(raw, { api: true });
  if (url.search) throw new EmomoError('INVALID_URL', 'The API base URL cannot contain query parameters.');
  url.pathname = url.pathname.replace(/\/+$/, '');
  return url;
}

function imageUrl(raw, api, allowedHosts) {
  let url;
  try { url = new URL(raw); } catch { throw invalidResponse(); }
  // Local image fixtures are permitted only on the explicitly selected local API.
  if (api.protocol === 'http:' && url.origin === api.origin && !url.username && !url.password && !url.hash) return url;
  url = publicUrl(raw);
  const allowed = allowedHosts.some(host => host.startsWith('*.') ? url.hostname.endsWith(host.slice(1)) && url.hostname !== host.slice(2) : url.hostname === host);
  if (!allowed) throw new EmomoError('IMAGE_HOST_NOT_ALLOWED', 'The image host is not trusted. Review it before adding it to EMOMO_IMAGE_HOSTS.');
  return url;
}

function transportError(error) {
  if (error.name === 'TimeoutError' || error.name === 'AbortError') return new EmomoError('TIMEOUT', 'The request timed out.', { retryable: true });
  return new EmomoError('NETWORK_ERROR', 'The service could not be reached.', { retryable: true });
}

function stringArray(value) {
  if (value === undefined) return [];
  if (!Array.isArray(value) || value.some(item => typeof item !== 'string')) throw invalidResponse();
  return value;
}

function nonnegative(value, fallback = 0) {
  if (value === undefined) return fallback;
  if (!Number.isSafeInteger(value) || value < 0) throw invalidResponse();
  return value;
}

function normalizeMeme(meme, result = {}) {
  if (!object(meme) || typeof meme.id !== 'string' || !meme.id || typeof meme.url !== 'string' || !meme.url) throw invalidResponse();
  const info = field(meme, 'image_info', 'imageInfo') ?? {};
  if (!object(info)) throw invalidResponse();
  const score = result.score ?? 0;
  if (typeof score !== 'number' || !Number.isFinite(score)) throw invalidResponse();
  for (const value of [meme.category, result.description]) {
    if (value !== undefined && typeof value !== 'string') throw invalidResponse();
  }
  return {
    id: meme.id,
    url: meme.url,
    description: result.description ?? '',
    category: meme.category ?? '',
    tags: stringArray(meme.tags),
    score,
    textPresence: TEXT_PRESENCE.get(field(result, 'text_presence', 'textPresence')) ?? 'unknown',
    image: { width: nonnegative(info.width), height: nonnegative(info.height), format: FORMATS.get(info.format) ?? 'unknown' },
  };
}

function statusError(response, resource = 'API') {
  const status = response.status;
  const errors = {
    400: ['BAD_REQUEST', 'The service rejected the request. Check the search arguments.'],
    401: ['UNAUTHORIZED', 'Authentication is required. Set EMOMO_API_TOKEN if this API requires a token.'],
    403: ['FORBIDDEN', 'Access was denied. Check the API endpoint and its access policy.'],
    404: ['NOT_FOUND', `${resource === 'image' ? 'The image' : 'The requested resource'} was not found.`],
    410: ['SERVICE_PAUSED', 'Emomo is offline. Do not retry searches or restart infrastructure automatically.'],
    429: ['RATE_LIMITED', 'The service rate limit was reached. Wait before trying again.'],
    503: ['SERVICE_UNAVAILABLE', 'The service is unavailable.'],
  };
  const [code, message] = errors[status] ?? ['HTTP_ERROR', `The ${resource} request failed.`];
  const details = { httpStatus: status, retryable: status === 429 || status >= 500 };
  const retry = response.headers.get('retry-after');
  if (status === 429 && /^\d+$/.test(retry ?? '')) details.retryAfterSeconds = Number(retry);
  return new EmomoError(code, message, details);
}

async function readBounded(response, maximum) {
  const size = response.headers.get('content-length');
  if (size && Number(size) > maximum) {
    await response.body?.cancel();
    throw new EmomoError('RESPONSE_TOO_LARGE', 'The response exceeds the size limit.');
  }
  const chunks = [];
  let length = 0;
  try {
    for await (const chunk of response.body ?? []) {
      length += chunk.length;
      if (length > maximum) throw new EmomoError('RESPONSE_TOO_LARGE', 'The response exceeds the size limit.');
      chunks.push(Buffer.from(chunk));
    }
  } catch (error) {
    if (error instanceof EmomoError) throw error;
    throw transportError(error);
  }
  return Buffer.concat(chunks);
}

function identifyImage(bytes) {
  if (bytes.length >= 8 && bytes.subarray(0, 8).equals(Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]))) return { extension: 'png', mimeType: 'image/png' };
  if (bytes.length >= 3 && bytes[0] === 255 && bytes[1] === 216 && bytes[2] === 255) return { extension: 'jpg', mimeType: 'image/jpeg' };
  if (bytes.length >= 12 && bytes.toString('ascii', 0, 4) === 'RIFF' && bytes.toString('ascii', 8, 12) === 'WEBP') return { extension: 'webp', mimeType: 'image/webp' };
  throw new EmomoError('INVALID_IMAGE', 'The download is not a supported static PNG, JPEG, or WebP image.');
}

export class EmomoClient {
  constructor({ baseUrl = DEFAULT_API_URL, token, timeoutMs = 30000, imageHosts = ['r2.emomo.net', '*.r2.dev', '*.r2.cloudflarestorage.com'] } = {}) {
    this.base = apiUrl(baseUrl);
    this.token = token;
    this.timeoutMs = timeoutMs;
    this.imageHosts = imageHosts;
    if (token && this.base.protocol !== 'https:') throw new EmomoError('INSECURE_TOKEN', 'API tokens cannot be sent over HTTP.');
  }

  async request(path, { method = 'GET', body } = {}) {
    const url = new URL(`${this.base.href.replace(/\/$/, '')}/${path}`);
    const headers = { Accept: 'application/json', 'User-Agent': 'emomo-cli/0.1.0' };
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    if (this.token) headers.Authorization = `Bearer ${this.token}`;
    const response = await this.fetch(url, { method, headers, body: body === undefined ? undefined : JSON.stringify(body), redirect: 'manual' });
    if (response.status >= 300 && response.status < 400) {
      await response.body?.cancel();
      throw new EmomoError('API_REDIRECT', 'API redirects are not followed. Configure the final API URL explicitly.');
    }
    if (!response.ok) {
      await response.body?.cancel();
      throw statusError(response);
    }
    if (!response.headers.get('content-type')?.toLowerCase().includes('application/json')) {
      await response.body?.cancel();
      throw invalidResponse();
    }
    let result;
    try { result = JSON.parse((await readBounded(response, MAX_JSON_BYTES)).toString('utf8')); } catch (error) {
      if (error instanceof EmomoError) throw error;
      throw invalidResponse();
    }
    if (!object(result)) throw invalidResponse();
    return result;
  }

  async fetch(url, options) {
    try { return await fetch(url, { ...options, signal: AbortSignal.timeout(this.timeoutMs) }); } catch (error) {
      throw transportError(error);
    }
  }

  async search(query, { limit = 8, category, textPresence, profile, collection } = {}) {
    const body = { query, top_k: limit };
    if (category) body.category = category;
    if (textPresence !== undefined) body.text_presence = textPresence;
    if (profile) body.profile = profile;
    if (collection) body.collection = collection;
    const response = await this.request('search', { method: 'POST', body });
    const results = response.results ?? [];
    if (!Array.isArray(results) || results.some(item => !object(item))) throw invalidResponse();
    const expandedQuery = field(response, 'expanded_query', 'expandedQuery') ?? '';
    if (typeof expandedQuery !== 'string') throw invalidResponse();
    return { query, expandedQuery, total: nonnegative(response.total, results.length), results: results.map(result => normalizeMeme(result.meme, result)) };
  }

  async get(id) {
    const response = await this.request(`memes/${encodeURIComponent(id)}`);
    const meme = normalizeMeme(response.meme);
    if (meme.id !== id) throw invalidResponse();
    return { meme };
  }

  async categories() {
    const response = await this.request('categories');
    const categories = stringArray(response.categories);
    return { categories, total: nonnegative(response.total, categories.length) };
  }

  async stats() {
    const response = await this.request('stats');
    const total = field(response, 'total_memes', 'totalMemes') ?? '0';
    if (!(typeof total === 'string' && /^\d+$/.test(total)) && !(Number.isSafeInteger(total) && total >= 0)) throw invalidResponse();
    return {
      totalMemes: String(total),
      totalCategories: nonnegative(field(response, 'total_categories', 'totalCategories')),
      availableProfiles: stringArray(field(response, 'available_profiles', 'availableProfiles')),
      availableCollections: stringArray(field(response, 'available_collections', 'availableCollections')),
    };
  }

  async download(id, directory) {
    const { meme } = await this.get(id);
    let url = imageUrl(meme.url, this.base, this.imageHosts);
    let response;
    for (let hop = 0; hop <= 3; hop++) {
      // API Authorization is deliberately never attached to image requests.
      response = await this.fetch(url, { redirect: 'manual', headers: { Accept: 'image/png,image/jpeg,image/webp', 'User-Agent': 'emomo-cli/0.1.0' } });
      if (response.status >= 300 && response.status < 400) {
        const location = response.headers.get('location');
        await response.body?.cancel();
        if (!location || hop === 3) throw new EmomoError('IMAGE_REDIRECT', 'The image redirect could not be resolved.');
        url = imageUrl(new URL(location, url).href, this.base, this.imageHosts);
        continue;
      }
      break;
    }
    if (!response.ok) {
      await response.body?.cancel();
      throw statusError(response, 'image');
    }
    const contentType = (response.headers.get('content-type') ?? '').split(';')[0].trim().toLowerCase();
    if (!['image/png', 'image/jpeg', 'image/webp'].includes(contentType)) {
      await response.body?.cancel();
      throw new EmomoError('INVALID_IMAGE', 'The server did not return a supported static image.');
    }
    const bytes = await readBounded(response, MAX_IMAGE_BYTES);
    const image = identifyImage(bytes);
    if (image.mimeType !== contentType) throw new EmomoError('INVALID_IMAGE', 'The image content does not match its declared type.');
    const destination = resolve(directory);
    const filename = `${id.replace(/[^a-zA-Z0-9_-]/g, '_').slice(0, 100)}.${image.extension}`;
    const path = join(destination, filename);
    await mkdir(destination, { recursive: true });
    const temporary = join(destination, `.emomo-${randomUUID()}.tmp`);
    let file;
    try {
      file = await open(temporary, 'wx', 0o600);
      await file.writeFile(bytes);
      await file.close();
      file = undefined;
      // link() publishes atomically and refuses to overwrite an existing file.
      await link(temporary, path);
    } catch (error) {
      if (error.code === 'EEXIST') throw new EmomoError('FILE_EXISTS', 'The destination image already exists. Choose another directory.');
      throw new EmomoError('FILE_WRITE_FAILED', 'The image could not be saved. Check the destination directory.');
    } finally {
      await file?.close();
      await unlink(temporary).catch(() => {});
    }
    return { id, path, mimeType: image.mimeType, bytes: bytes.length, sha256: createHash('sha256').update(bytes).digest('hex'), sourceUrl: meme.url };
  }
}
