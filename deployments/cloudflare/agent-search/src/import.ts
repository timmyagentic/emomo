import { create, fromJson, toJson, type JsonValue } from '@bufbuild/protobuf';
import { ImportMetadataSchema, ImportResponseSchema, ImportLookupRequestSchema, ImportLookupResponseSchema, ImportErrorResponseSchema } from '../gen/emomo/v1/import_pb.js';
import { MemeSchema } from '../gen/emomo/v1/meme_pb.js';
import { keywordSearch, HttpError } from './index.js';
import { indexRow, rowValues, validateStorageKey } from './metadata.js';
import { importMetadata, SHA256, MAX_IMPORT_IMAGE_BYTES } from '../../../../cli/src/import-metadata.js';
import { staticImageInfo } from '../../../../cli/src/image-info.js';

const OPTIONS = { useProtoFieldName: true, enumAsInteger: true, alwaysEmitImplicit: true };
const HASH_SQL = "json_extract(meme_json, '$.content_hash')";
type Stored = { meme_json: string; description: string; text_presence: number };
class ImportError extends Error {
  constructor(readonly status: number, readonly code: string, message: string) { super(message); }
}
const json = (value: JsonValue, status = 200) => Response.json(value, { status, headers: { 'cache-control': 'no-store', 'x-content-type-options': 'nosniff' } });
const hex = (bytes: ArrayBuffer) => [...new Uint8Array(bytes)].map(b => b.toString(16).padStart(2, '0')).join('');

async function authenticate(request: Request, env: ImportEnv) {
  if (!SHA256.test(env.IMPORT_KEY_SHA256 ?? '')) throw new ImportError(503, 'IMPORT_UNAVAILABLE', 'Maintenance access is not configured.');
  const token = /^Bearer (emomo_import_[a-f0-9]{64})$/.exec(request.headers.get('authorization') ?? '')?.[1];
  if (!token) throw new ImportError(401, 'UNAUTHORIZED', 'A maintenance key is required.');
  const hash = hex(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(token)));
  if (!crypto.subtle.timingSafeEqual(new TextEncoder().encode(hash), new TextEncoder().encode(env.IMPORT_KEY_SHA256))) throw new ImportError(401, 'UNAUTHORIZED', 'A maintenance key is required.');
}
async function body(request: Request, maximum: number): Promise<Uint8Array> {
  if (!request.body) throw new ImportError(400, 'INVALID_BODY', 'A request body is required.');
  if (Number(request.headers.get('content-length')) > maximum) throw new ImportError(413, 'BODY_TOO_LARGE', 'Import body exceeds the size limit.');
  const reader = request.body.getReader(), chunks: Uint8Array[] = [];
  let length = 0, timedOut = false;
  const timer = setTimeout(() => { timedOut = true; void reader.cancel().catch(() => {}); }, 60000);
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (timedOut) throw new ImportError(408, 'BODY_TIMEOUT', 'Import upload timed out.');
      if (done) break;
      length += value.length;
      if (length > maximum) { await reader.cancel(); throw new ImportError(413, 'BODY_TOO_LARGE', 'Import body exceeds the size limit.'); }
      chunks.push(value);
    }
  } finally { clearTimeout(timer); reader.releaseLock(); }
  const bytes = new Uint8Array(length);
  let offset = 0;
  for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.length; }
  return bytes;
}
function meme(row: Stored, env: ImportEnv) {
  const value = fromJson(MemeSchema, JSON.parse(row.meme_json));
  value.url = `${env.IMAGE_BASE_URL}/${value.storageKey}`;
  return value;
}
async function existing(env: ImportEnv, hash: string): Promise<Stored | null> {
  return env.DB.prepare(`SELECT meme_json, description, text_presence FROM memes WHERE ${HASH_SQL} = ? ORDER BY id LIMIT 1`).bind(hash).first<Stored>();
}
function result(row: Stored, env: ImportEnv, duplicate: boolean) {
  return json(toJson(ImportResponseSchema, create(ImportResponseSchema, { meme: meme(row, env), duplicate, description: row.description, textPresence: row.text_presence }), OPTIONS));
}
async function lookup(request: Request, env: ImportEnv) {
  let hashes: string[];
  try {
    hashes = fromJson(ImportLookupRequestSchema, JSON.parse(new TextDecoder('utf-8', { fatal: true, ignoreBOM: false }).decode(await body(request, 16384)))).sha256;
    if (!hashes.length || hashes.length > 100 || hashes.some(hash => !SHA256.test(hash))) throw new Error();
  } catch (error) {
    if (error instanceof ImportError) throw error;
    throw new ImportError(400, 'INVALID_METADATA', 'Provide between 1 and 100 SHA-256 values.');
  }
  const unique = [...new Set(hashes)];
  const rows = await env.DB.prepare(`SELECT meme_json FROM memes WHERE ${HASH_SQL} IN (${unique.map(() => '?').join(',')}) ORDER BY id`).bind(...unique).all<Stored>();
  return json(toJson(ImportLookupResponseSchema, create(ImportLookupResponseSchema, { existing: rows.results.map(row => meme(row, env)) }), OPTIONS));
}
async function ingest(request: Request, env: ImportEnv) {
  if (!request.headers.get('content-type')?.startsWith('multipart/form-data;')) throw new ImportError(415, 'INVALID_BODY', 'Use multipart/form-data with metadata and image parts.');
  let metadata, image: Uint8Array;
  try {
    const bytes = await body(request, MAX_IMPORT_IMAGE_BYTES + 65536);
    const form = await new Response(bytes, { headers: { 'content-type': request.headers.get('content-type')! } }).formData();
    const raw = form.get('metadata'), file = form.get('image');
    if ([...form.keys()].length !== 2 || typeof raw !== 'string' || raw.length > 32768 || !(file instanceof File)) throw new Error();
    metadata = importMetadata(toJson(ImportMetadataSchema, fromJson(ImportMetadataSchema, JSON.parse(raw)), OPTIONS));
    image = new Uint8Array(await file.arrayBuffer());
    const info = staticImageInfo(image);
    if (JSON.stringify(info) !== JSON.stringify(metadata.image_info)) throw new Error();
    if (hex(await crypto.subtle.digest('SHA-256', image)) !== metadata.sha256) throw new Error();
  } catch (error) {
    if (error instanceof ImportError) throw error;
    throw new ImportError(400, 'INVALID_METADATA', 'Complete metadata, static image dimensions, format and SHA-256 must agree.');
  }
  const found = await existing(env, metadata.sha256);
  if (found) return result(found, env, true);
  const extension = ['', 'jpg', 'png', 'webp'][metadata.image_info.format];
  const key = `collections/reviewed/${metadata.sha256}.${extension}`;
  const id = `reviewed-${metadata.sha256}`;
  const description = metadata.preview_only ? `[静态首帧预览] ${metadata.description}` : metadata.description;
  const row = indexRow({ meme: { id, storage_key: key, content_hash: metadata.sha256, image_info: metadata.image_info, category: metadata.category, tags: metadata.tags },
    annotation: { meme_id: id, description, ocr_text: metadata.ocr_text }, text_presence: metadata.text_presence, search_aliases: metadata.search_aliases });
  // Persist R2 before making the index visible. D1 failures leave reusable,
  // content-addressed objects; deleting here would break concurrent retries.
  await env.IMAGES.put(key, image, { sha256: metadata.sha256, httpMetadata: { contentType: `image/${extension === 'jpg' ? 'jpeg' : extension}` } });
  // One D1 statement serializes competing imports and preserves old metadata.
  const inserted = await env.DB.prepare(`INSERT INTO memes (id,meme_json,category,description,ocr_text,text_presence,ocr_terms,description_terms,tag_terms)
    SELECT ?,?,?,?,?,?,?,?,? WHERE NOT EXISTS (SELECT 1 FROM memes WHERE ${HASH_SQL} = ?)
    ON CONFLICT(id) DO NOTHING RETURNING meme_json,description,text_presence`).bind(...rowValues(row), metadata.sha256).first<Stored>();
  const stored = inserted ?? await existing(env, metadata.sha256);
  if (!stored) throw new ImportError(409, 'ID_CONFLICT', 'The generated ID is already used by another asset.');
  return result(stored, env, !inserted);
}
async function originalImage(hash: string, env: ImportEnv) {
  const row = await existing(env, hash);
  if (!row) throw new ImportError(404, 'NOT_FOUND', 'Image not found.');
  const stored = fromJson(MemeSchema, JSON.parse(row.meme_json));
  const object = await env.IMAGES.get(validateStorageKey(stored.storageKey));
  if (!object) throw new ImportError(404, 'NOT_FOUND', 'Image object not found.');
  if (object.size > MAX_IMPORT_IMAGE_BYTES) {
    await object.body.cancel();
    throw new ImportError(413, 'IMAGE_TOO_LARGE', 'Image exceeds the import size limit.');
  }
  const mime = ['', 'image/jpeg', 'image/png', 'image/webp'][stored.imageInfo?.format ?? 0];
  if (!mime) { await object.body.cancel(); throw new ImportError(415, 'INVALID_IMAGE', 'Unsupported image format.'); }
  return new Response(object.body, { headers: {
    'content-type': mime, 'content-length': String(object.size), 'cache-control': 'no-store', 'x-content-type-options': 'nosniff'
  } });
}
export default {
  async fetch(request: Request, env: ImportEnv): Promise<Response> {
    try {
      const url = new URL(request.url);
      const imageHash = /^\/admin\/v1\/images\/([a-f0-9]{64})$/.exec(url.pathname)?.[1];
      if (url.search || !imageHash && !['/admin/v1/imports', '/admin/v1/lookup', '/admin/v1/search'].includes(url.pathname)) throw new ImportError(404, 'NOT_FOUND', 'Route not found.');
      if (request.headers.has('origin')) throw new ImportError(403, 'FORBIDDEN', 'Use the maintenance script.');
      if (request.method !== (imageHash ? 'GET' : 'POST')) throw new ImportError(405, 'METHOD_NOT_ALLOWED', 'Invalid HTTP method.');
      await authenticate(request, env);
      if (imageHash) return await originalImage(imageHash, env);
      if (url.pathname.endsWith('/search')) {
        const response = await keywordSearch(request, env);
        response.headers.set('cache-control', 'no-store');
        return response;
      }
      return url.pathname.endsWith('/lookup') ? await lookup(request, env) : await ingest(request, env);
    } catch (error) {
      if (error instanceof HttpError) return json(toJson(ImportErrorResponseSchema, create(ImportErrorResponseSchema, { code: 'INVALID_QUERY', message: error.message }), OPTIONS), error.status);
      if (error instanceof ImportError) return json(toJson(ImportErrorResponseSchema, create(ImportErrorResponseSchema, { code: error.code, message: error.message }), OPTIONS), error.status);
      console.error(JSON.stringify({ event: 'import_unavailable' }));
      return json(toJson(ImportErrorResponseSchema, create(ImportErrorResponseSchema, { code: 'IMPORT_UNAVAILABLE', message: 'Import could not finish. Retry the same prepared image.' }), OPTIONS), 503);
    }
  },
} satisfies ExportedHandler<ImportEnv>;
