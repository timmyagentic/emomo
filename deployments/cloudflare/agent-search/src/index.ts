import { create, fromJson, toJson, type JsonValue } from '@bufbuild/protobuf';
import { SearchRequestSchema, SearchResponseSchema, GetMemeResponseSchema, GetCategoriesResponseSchema, GetStatsResponseSchema } from '../gen/emomo/v1/api_pb.js';
import { MemeSchema, SearchResultSchema } from '../gen/emomo/v1/meme_pb.js';
import { TextPresence } from '../gen/emomo/v1/types_pb.js';
import { ID, validateStorageKey } from './metadata.js';
import { matchExpression } from './terms.js';

const JSON_OPTIONS = { useProtoFieldName: true, enumAsInteger: true, alwaysEmitImplicit: true };
const MAX_BODY = 8192;
const CACHE_SECONDS = 60;
type SearchRow = { meme_json: string; description: string; text_presence: TextPresence; rank: number };
class HttpError extends Error {
  constructor(readonly status: number, message: string) { super(message); }
}

function json(value: JsonValue, status = 200): Response {
  return Response.json(value, { status, headers: {
    'cache-control': status === 200 ? `public, max-age=${CACHE_SECONDS}` : 'no-store',
    'x-emomo-retrieval': 'keyword-fts5',
    'x-content-type-options': 'nosniff',
  } });
}

async function body(request: Request): Promise<string> {
  if (!request.headers.get('content-type')?.toLowerCase().startsWith('application/json')) throw new HttpError(415, 'Use application/json.');
  if (!request.body) throw new HttpError(400, 'Missing search body.');
  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let length = 0;
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      length += value.byteLength;
      if (length > MAX_BODY) { await reader.cancel(); throw new HttpError(413, 'Search body is too large.'); }
      chunks.push(value);
    }
  } finally { reader.releaseLock(); }
  const bytes = new Uint8Array(length);
  let offset = 0;
  for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.byteLength; }
  try { return new TextDecoder('utf-8', { fatal: true, ignoreBOM: false }).decode(bytes); }
  catch { throw new HttpError(400, 'Invalid UTF-8 search body.'); }
}

function publicMeme(raw: string, base: string) {
  const meme = fromJson(MemeSchema, JSON.parse(raw));
  const key = validateStorageKey(meme.storageKey);
  const root = new URL(base);
  const local = root.protocol === 'http:' && ['127.0.0.1', 'localhost', '[::1]'].includes(root.hostname);
  if ((!local && root.protocol !== 'https:') || root.username || root.password || root.search || root.hash) throw new Error('Invalid image base.');
  meme.url = `${root.href.replace(/\/$/, '')}/${key.split('/').map(part => encodeURIComponent(part)).join('/')}`;
  return meme;
}

async function search(request: Request, env: Env): Promise<Response> {
  let parsed;
  let hasTopK = false;
  try {
    const raw: JsonValue = JSON.parse(await body(request));
    hasTopK = !!raw && typeof raw === 'object' && (Object.hasOwn(raw, 'top_k') || Object.hasOwn(raw, 'topK'));
    parsed = fromJson(SearchRequestSchema, raw);
  }
  catch (error) { if (error instanceof HttpError) throw error; throw new HttpError(400, 'Invalid search body.'); }
  const query = parsed.query.trim();
  if (!query || [...query].length > 160 || [...parsed.category].length > 80) throw new HttpError(400, 'Use a query of 1–160 characters and a short category.');
  const topK = hasTopK ? parsed.topK : 8;
  if (topK < 1 || topK > 100 || !Number.isInteger(topK)) throw new HttpError(400, 'top_k must be 1–100.');
  if (![0, 1, 2, 3].includes(parsed.textPresence)) throw new HttpError(400, 'Invalid text_presence.');
  if (parsed.profile && parsed.profile !== 'keyword' || parsed.collection && parsed.collection !== 'keyword') throw new HttpError(400, 'Only the keyword profile is available.');
  let expression: string;
  try { expression = matchExpression(query); }
  catch { throw new HttpError(400, 'Use 1–64 short keyword terms.'); }
  const rows = await env.DB.prepare(`
    SELECT m.meme_json, m.description, m.text_presence,
      bm25(meme_fts, 4.0, 1.0, 2.0) AS rank
    FROM meme_fts JOIN memes m ON m.rowid=meme_fts.rowid
    WHERE meme_fts MATCH ?
      AND (?='' OR m.category=?) AND (?=0 OR m.text_presence=?)
    ORDER BY rank ASC, m.id ASC LIMIT ?
  `).bind(expression, parsed.category, parsed.category, parsed.textPresence, parsed.textPresence, topK).all<SearchRow>();
  const results = rows.results.map(row => create(SearchResultSchema, {
    meme: publicMeme(row.meme_json, env.IMAGE_BASE_URL),
    description: row.description,
    textPresence: row.text_presence,
    // Monotonic normalized BM25 relevance, not a semantic probability.
    score: Math.max(0, -row.rank) / (1 + Math.max(0, -row.rank)),
  }));
  return json(toJson(SearchResponseSchema, create(SearchResponseSchema, { query, profile: 'keyword', results, total: results.length }), JSON_OPTIONS));
}

async function serve(request: Request, path: string, env: Env): Promise<Response> {
  if (path === '/agent/v1/search') return search(request, env);
  if (path === '/agent/v1/categories') {
    const rows = await env.DB.prepare("SELECT DISTINCT category FROM memes WHERE category<>'' ORDER BY category LIMIT 100").all<{ category: string }>();
    const categories = rows.results.map(row => row.category);
    return json(toJson(GetCategoriesResponseSchema, create(GetCategoriesResponseSchema, { categories, total: categories.length }), JSON_OPTIONS));
  }
  if (path === '/agent/v1/stats') {
    const stats = await env.DB.prepare("SELECT COUNT(*) AS total, COUNT(DISTINCT NULLIF(category, '')) AS categories FROM memes").first<{ total: number; categories: number }>();
    if (!stats) throw new Error('Missing index stats.');
    return json(toJson(GetStatsResponseSchema, create(GetStatsResponseSchema, { totalMemes: BigInt(stats.total), totalCategories: stats.categories, availableProfiles: ['keyword'] }), JSON_OPTIONS));
  }
  const id = path.slice('/agent/v1/memes/'.length);
  const row = await env.DB.prepare('SELECT meme_json FROM memes WHERE id=?').bind(id).first<{ meme_json: string }>();
  if (!row) throw new HttpError(404, 'Meme not found.');
  return json(toJson(GetMemeResponseSchema, create(GetMemeResponseSchema, { meme: publicMeme(row.meme_json, env.IMAGE_BASE_URL) }), JSON_OPTIONS));
}

export default {
  async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    const url = new URL(request.url);
    if (url.pathname === '/api/v1' || url.pathname.startsWith('/api/v1/')) return json({ error: 'Legacy API retired.' }, 410);
    if (env.AGENT_API_ENABLED !== 'true') return json({ error: 'Agent API paused.' }, 410);
    const path = url.pathname;
    const expected = path === '/agent/v1/search' ? 'POST' :
      ['/agent/v1/stats', '/agent/v1/categories'].includes(path) ||
      path.startsWith('/agent/v1/memes/') && ID.test(path.slice('/agent/v1/memes/'.length)) ? 'GET' : '';
    if (!expected || url.search) return json({ error: 'Route not found.' }, 404);
    if (request.method !== expected) return json({ error: 'Method not allowed.' }, 405);
    // CLI-only API. Reject browser origins, including cached responses.
    if (request.headers.has('origin')) return json({ error: 'Browser access disabled.' }, 403);
    try {
      const limited = await env.EMOMO_RATE_LIMITER.limit({ key: request.headers.get('cf-connecting-ip') || 'unknown' });
      if (!limited.success) {
        const response = json({ error: 'Rate limited.' }, 429);
        response.headers.set('retry-after', '60');
        return response;
      }
      // Query text is not put in cache URLs or logs. Bound before hashing.
      const content = expected === 'POST' ? await body(request) : '';
      const normalized = expected === 'POST' ? new Request(request.url, { method: 'POST', headers: request.headers, body: content }) : request;
      const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(`${env.IMAGE_BASE_URL}\n${path}\n${content}`));
      const hash = [...new Uint8Array(digest)].map(byte => byte.toString(16).padStart(2, '0')).join('');
      const key = new Request(`${url.origin}/__emomo-cache/${hash}`);
      const cached = await caches.default.match(key);
      if (cached) return cached;
      const response = await serve(normalized, path, env);
      if (response.ok) ctx.waitUntil(caches.default.put(key, response.clone()).catch(() => { console.warn(JSON.stringify({ event: 'cache_write_failed' })); }));
      return response;
    } catch (error) {
      if (error instanceof HttpError) return json({ error: error.message }, error.status);
      console.error(JSON.stringify({ event: 'index_unavailable' }));
      return json({ error: 'Text index unavailable; no fallback service is called.' }, 503);
    }
  },
} satisfies ExportedHandler<Env>;
