import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { createServer, type IncomingMessage, type ServerResponse } from 'node:http';
import { once } from 'node:events';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import { test } from 'node:test';
import worker from './index';

const exec = promisify(execFile);
const binary = fileURLToPath(new URL('../../../../cli/bin/emomo.js', import.meta.url).href);

async function listen(handler: (request: IncomingMessage, response: ServerResponse) => Promise<void>) {
  const server = createServer((request, response) => {
    handler(request, response).catch(() => {
      response.statusCode = 500;
      response.end('{}');
    });
  });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  const address = server.address();
  assert.ok(address && typeof address !== 'string');
  return { server, origin: `http://127.0.0.1:${address.port}` };
}

test('real CLI process -> Agent gateway -> private upstream DTO, while old API remains paused', async t => {
  const requests: { path: string; authorization: string | undefined; body: string }[] = [];
  const upstream = await listen(async (request, response) => {
    const chunks = [];
    for await (const chunk of request) chunks.push(Buffer.from(chunk));
    const body = Buffer.concat(chunks).toString();
    requests.push({ path: request.url || '', authorization: request.headers.authorization, body });
    response.setHeader('Content-Type', 'application/json');
    if (request.url === '/api/v1/search') {
      response.end(JSON.stringify({ results: [{ meme: { id: 'fixture-1', url: 'https://r2.emomo.net/fixture-1.png', image_info: { format: 2, width: 1, height: 1 }, tags: ['协议测试样本'] }, description: '本地协议样本，不是真实图库搜索', score: 0.9, text_presence: 2 }], total: 1 }));
    } else if (request.url === '/api/v1/memes/fixture-1') {
      response.end(JSON.stringify({ meme: { id: 'fixture-1', url: 'https://r2.emomo.net/fixture-1.png', image_info: { format: 2 } } }));
    } else {
      response.statusCode = 404;
      response.end('{}');
    }
  });
  t.after(() => { upstream.server.closeAllConnections(); upstream.server.close(); });
  let enabled = 'false';
  let edgeOrigin = '';
  const edge = await listen(async (request, response) => {
    const headers = new Headers();
    for (const [key, value] of Object.entries(request.headers)) {
      if (value !== undefined) headers.set(key, Array.isArray(value) ? value.join(',') : value);
    }
    const chunks = [];
    for await (const chunk of request) chunks.push(Buffer.from(chunk));
    const body = Buffer.concat(chunks);
    const method = request.method || 'GET';
    const workerRequest = new Request(edgeOrigin + request.url, { method, headers, body: method === 'GET' || method === 'HEAD' ? undefined : body });
    const env = {
      SERVICE_MODE: 'agent', AGENT_API_ENABLED: enabled, UPSTREAM_ORIGIN: upstream.origin,
      HF_TOKEN: 'local-fixture-hf-token', CORS_ALLOWED_ORIGINS: '', CACHE_TTL_SECONDS: '30',
      MAX_REQUEST_BODY_BYTES: '65536', MAX_LIST_WINDOW: '120', MAX_SEARCH_TOP_K: '100',
      EMOMO_RATE_LIMITER: { limit: async () => ({ success: true }) },
    } as unknown as Env;
    const result = await worker.fetch(workerRequest, env, { waitUntil() {}, passThroughOnException() {} } as unknown as ExecutionContext);
    response.statusCode = result.status;
    for (const [key, value] of result.headers) response.setHeader(key, value);
    response.end(Buffer.from(await result.arrayBuffer()));
  });
  edgeOrigin = edge.origin;
  t.after(() => { edge.server.closeAllConnections(); edge.server.close(); });
  const env = { ...process.env, EMOMO_API_URL: `${edge.origin}/agent/v1`, EMOMO_API_TOKEN: '' };
  await assert.rejects(exec(process.execPath, [binary, 'search', '想下班'], { env }), error => {
    if (!(error instanceof Error) || !('stdout' in error)) return false;
    assert.equal(JSON.parse(String(error.stdout)).error.code, 'SERVICE_PAUSED');
    return true;
  });
  assert.equal(requests.length, 0);
  enabled = 'true';
  const result = JSON.parse((await exec(process.execPath, [binary, 'search', '想下班', '--limit', '4', '--text', 'with'], { env })).stdout);
  assert.equal(result.data.results[0].id, 'fixture-1');
  assert.deepEqual(JSON.parse(requests[0].body), { query: '想下班', top_k: 4, text_presence: 2 });
  assert.equal(requests[0].authorization, 'Bearer local-fixture-hf-token');
  const detail = JSON.parse((await exec(process.execPath, [binary, 'get', 'fixture-1'], { env })).stdout);
  assert.equal(detail.data.meme.id, 'fixture-1');
  const legacy = await fetch(`${edge.origin}/api/v1/search`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ query: '想下班' }) });
  assert.equal(legacy.status, 410);
  assert.equal(requests.length, 2);
});
