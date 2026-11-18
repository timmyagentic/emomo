import { fromJson, type DescMessage, type MessageShape, type JsonValue } from '@bufbuild/protobuf';
import { SHA256 } from '../../../../cli/src/import-metadata.js';

export const OPTIONS = { useProtoFieldName: true, enumAsInteger: true, alwaysEmitImplicit: true };
export const HASH_SQL = "json_extract(meme_json, '$.content_hash')";
export class ImportError extends Error {
  constructor(readonly status: number, readonly code: string, message: string) { super(message); }
}
export const json = (value: JsonValue, status = 200) => Response.json(value, { status, headers: { 'cache-control': 'no-store', 'x-content-type-options': 'nosniff' } });
export const hex = (bytes: ArrayBuffer) => [...new Uint8Array(bytes)].map(b => b.toString(16).padStart(2, '0')).join('');

export async function authenticate(request: Request, expected: string, scope: 'import' | 'takedown') {
  if (!SHA256.test(expected ?? '')) throw new ImportError(503, 'IMPORT_UNAVAILABLE', 'Maintenance access is not configured.');
  const token = request.headers.get('authorization')?.match(/^Bearer (emomo_(import|takedown)_[a-f0-9]{64})$/);
  if (!token || token[2] !== scope) throw new ImportError(401, 'UNAUTHORIZED', 'A maintenance key is required.');
  const hash = hex(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(token[1])));
  if (!crypto.subtle.timingSafeEqual(new TextEncoder().encode(hash), new TextEncoder().encode(expected))) {
    throw new ImportError(401, 'UNAUTHORIZED', 'A maintenance key is required.');
  }
}
export async function body(request: Request, maximum: number): Promise<Uint8Array> {
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
export async function jsonBody<T extends DescMessage>(request: Request, schema: T): Promise<MessageShape<T>> {
  try {
    return fromJson(schema, JSON.parse(new TextDecoder('utf-8', { fatal: true, ignoreBOM: false }).decode(await body(request, 8192))));
  } catch (error) {
    if (error instanceof ImportError) throw error;
    throw new ImportError(400, 'INVALID_BODY', 'Invalid maintenance request body.');
  }
}
