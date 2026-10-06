import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtemp, readFile, readdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { main, run } from '../src/cli.js';
import { EmomoClient } from '../src/client.js';
import { PNG, serve, json, wireMeme } from './helpers.js';

async function cli(args, env = {}) {
  let output = '';
  const exit = await main(args, { env, write: text => { output += text; } });
  assert.equal(output.trim().split('\n').length, 1);
  return { exit, output: JSON.parse(output) };
}

test('Chinese search maps filters to existing protojson and gives flat candidates', async t => {
  let origin;
  const api = await serve((_request, response, body) => {
    assert.deepEqual(JSON.parse(body), { query: '想下班但还要开会', top_k: 3, category: 'reaction', text_presence: 2 });
    json(response, { results: [{ meme: wireMeme(origin), description: '测试协议样本', score: 0.87, text_presence: 2 }], total: 1, query: '想下班但还要开会', expanded_query: '开会 无奈' });
  }, t);
  origin = api.origin;
  const result = await cli(['search', '想下班但还要开会', '--limit', '3', '--category', 'reaction', '--text', 'with'], { EMOMO_API_URL: `${origin}/agent/v1` });
  assert.equal(api.requests[0].url, '/agent/v1/search');
  assert.equal(api.requests[0].method, 'POST');
  assert.equal(result.exit, 0);
  assert.deepEqual(result.output.data.results[0], { id: 'meme-1', url: `${origin}/image.png`, description: '测试协议样本', category: 'reaction', tags: ['打工', '开会'], score: 0.87, textPresence: 'with_text', image: { width: 1, height: 1, format: 'png' } });
  assert.equal(result.output.data.expandedQuery, '开会 无奈');
});

test('protojson omitted repeated/default fields represent an empty search', async t => {
  const api = await serve((_request, response) => json(response, { query: '没有匹配' }), t);
  const { data } = await run(['search', '没有匹配', '--api-url', `${api.origin}/api/v1`], { env: {} });
  assert.deepEqual(data, { query: '没有匹配', expandedQuery: '', total: 0, results: [] });
});

test('camelCase protojson compatibility and numeric image enums are preserved', async t => {
  const api = await serve((_request, response) => json(response, { results: [{ meme: { id: 'meme-2', url: 'https://r2.emomo.net/2.jpg', imageInfo: { width: 2, height: 3, format: 1 } }, textPresence: 3 }], expandedQuery: '惊讶' }), t);
  const { data } = await run(['search', '震惊', '--api-url', `${api.origin}/api/v1`], { env: {} });
  assert.equal(data.results[0].textPresence, 'without_text');
  assert.equal(data.results[0].image.format, 'jpeg');
});

test('doctor calls stats only and preserves protojson int64 as a decimal string', async t => {
  const api = await serve((request, response) => {
    assert.equal(request.url, '/agent/v1/stats');
    json(response, { total_memes: '12900', total_categories: 12, available_profiles: ['qwen3vl'] });
  }, t);
  const result = await cli(['doctor', '--api-url', `${api.origin}/agent/v1`]);
  assert.equal(result.output.data.stats.totalMemes, '12900');
  assert.equal(api.requests.length, 1);
});

test('unknown options, invalid limits, blank/long queries, and path-like IDs fail before network work', async () => {
  for (const args of [['search', 'a', '--limit', '0'], ['search', 'a', '--limit', '2.5'], ['search', 'a', '--limit', '101'], ['search', ' '], ['search', 'x'.repeat(161)], ['search', 'a', '--typo', 'b'], ['search', 'a', '--text', 'yes'], ['get', '../stats'], ['download', 'meme-1'], ['search', 'a', '--limit'], ['search', 'a', '--limit', '2', '--limit', '3']]) {
    const result = await cli(args);
    assert.equal(result.exit, 1, args.join(' '));
    assert.equal(result.output.error.code, 'INVALID_ARGUMENT');
  }
});

test('an API URL cannot carry credentials, query parameters, or public HTTP', async () => {
  for (const url of ['http://example.com/api/v1', 'https://user:do-not-leak-123@example.com/api/v1', 'https://example.com/api/v1?secret=value']) {
    const result = await cli(['stats', '--api-url', url]);
    assert.equal(result.output.error.code, 'INVALID_URL');
    assert.equal(JSON.stringify(result.output).includes('do-not-leak-123'), false);
    assert.equal(JSON.stringify(result.output).includes('secret=value'), false);
  }
  assert.equal((await cli(['stats', '--api-url', 'http://127.0.0.1:1/api/v1'], { EMOMO_API_TOKEN: 'test-only-token' })).output.error.code, 'INSECURE_TOKEN');
});

test('offline, missing, forbidden, and rate-limited responses have stable error codes without leaking upstream bodies', async t => {
  const expected = new Map([[401, 'UNAUTHORIZED'], [403, 'FORBIDDEN'], [404, 'NOT_FOUND'], [410, 'SERVICE_PAUSED'], [429, 'RATE_LIMITED'], [503, 'SERVICE_UNAVAILABLE']]);
  let status = 410;
  const api = await serve((_request, response) => json(response, { error: 'private-upstream-detail' }, status, { 'Retry-After': '45' }), t);
  for (const [value, code] of expected) {
    status = value;
    const result = await cli(['search', '测试', '--api-url', `${api.origin}/agent/v1`]);
    assert.equal(result.exit, 1);
    assert.equal(result.output.error.code, code);
    assert.equal(JSON.stringify(result.output).includes('private-upstream-detail'), false);
    if (status === 429) assert.equal(result.output.error.retryAfterSeconds, 45);
  }
  assert.equal(api.requests.length, expected.size); // no implicit retry
});

test('API redirects are never followed to a second host', async t => {
  const api = await serve((_request, response) => { response.writeHead(302, { Location: 'https://other.example/collect' }); response.end(); }, t);
  const result = await cli(['stats', '--api-url', `${api.origin}/agent/v1`]);
  assert.equal(result.output.error.code, 'API_REDIRECT');
  assert.equal(api.requests.length, 1);
});

test('malformed JSON, HTML, and incompatible meme shapes are rejected', async t => {
  let mode = 'json';
  const api = await serve((_request, response) => {
    if (mode === 'html') { response.writeHead(200, { 'Content-Type': 'text/html' }); response.end('<html>login</html>'); }
    else if (mode === 'json') { response.writeHead(200, { 'Content-Type': 'application/json' }); response.end('{bad'); }
    else json(response, { results: [{ meme: { id: 'missing-image-url' } }] });
  }, t);
  for (mode of ['json', 'html', 'shape']) {
    const result = await cli(['search', '测试', '--api-url', `${api.origin}/agent/v1`]);
    assert.equal(result.output.error.code, 'INVALID_RESPONSE');
  }
});

test('timeout includes slow response bodies and gives a transient error', async t => {
  const api = await serve((_request, response) => {
    response.writeHead(200, { 'Content-Type': 'application/json' }); response.flushHeaders();
    const timer = setTimeout(() => response.end('{}'), 500);
    response.on('close', () => clearTimeout(timer));
  }, t);
  const result = await cli(['stats', '--api-url', `${api.origin}/agent/v1`, '--timeout', '100']);
  assert.equal(result.output.error.code, 'TIMEOUT');
  assert.equal(result.output.error.retryable, true);
});

test('oversized JSON is rejected using the response size bound', async t => {
  const api = await serve((_request, response) => { response.writeHead(200, { 'Content-Type': 'application/json', 'Content-Length': 3000000 }); response.end('{}'); }, t);
  const result = await cli(['stats', '--api-url', `${api.origin}/agent/v1`]);
  assert.equal(result.output.error.code, 'RESPONSE_TOO_LARGE');
});

test('download obtains canonical metadata, saves valid bytes, and never overwrites', async t => {
  let origin;
  const api = await serve((request, response) => {
    if (request.url === '/agent/v1/memes/meme-1') json(response, { meme: wireMeme(origin) });
    else { assert.equal(request.url, '/image.png'); assert.equal(request.headers.authorization, undefined); response.writeHead(200, { 'Content-Type': 'image/png' }); response.end(PNG); }
  }, t);
  origin = api.origin;
  const directory = await mkdtemp(join(tmpdir(), 'emomo-download-test-'));
  const args = ['download', 'meme-1', '--dir', directory, '--api-url', `${origin}/agent/v1`];
  const result = await cli(args);
  assert.equal(result.exit, 0);
  assert.equal(result.output.data.path, join(directory, 'meme-1.png'));
  assert.deepEqual(await readFile(result.output.data.path), PNG);
  assert.equal(result.output.data.sha256.length, 64);
  assert.equal((await cli(args)).output.error.code, 'FILE_EXISTS');
  assert.deepEqual(await readdir(directory), ['meme-1.png']);
});

test('HTML disguised as an image and oversized images leave no output file', async t => {
  let origin, mode;
  const api = await serve((request, response) => {
    if (request.url.includes('/memes/')) json(response, { meme: wireMeme(origin) });
    else if (mode === 'large') { response.writeHead(200, { 'Content-Type': 'image/png', 'Content-Length': 30000000 }); response.end(PNG); }
    else { response.writeHead(200, { 'Content-Type': 'image/png' }); response.end('<html>login</html>'); }
  }, t);
  origin = api.origin;
  const directory = await mkdtemp(join(tmpdir(), 'emomo-invalid-image-'));
  for (mode of ['html', 'large']) {
    const result = await cli(['download', 'meme-1', '--dir', directory, '--api-url', `${origin}/agent/v1`]);
    assert.equal(result.output.error.code, mode === 'large' ? 'RESPONSE_TOO_LARGE' : 'INVALID_IMAGE');
  }
  assert.deepEqual(await readdir(directory), []);
});

test('untrusted remote image hosts and redirected image hosts are rejected before access', async t => {
  let origin, remote = true;
  const api = await serve((request, response) => {
    if (request.url.includes('/memes/')) json(response, { meme: { ...wireMeme(origin), url: remote ? 'https://localhost/private' : `${origin}/image.png` } });
    else { response.writeHead(302, { Location: 'https://attacker.example/collect' }); response.end(); }
  }, t);
  origin = api.origin;
  const directory = await mkdtemp(join(tmpdir(), 'emomo-image-host-'));
  for (remote of [true, false]) {
    const result = await cli(['download', 'meme-1', '--dir', directory, '--api-url', `${origin}/agent/v1`]);
    assert.equal(result.output.error.code, 'IMAGE_HOST_NOT_ALLOWED');
  }
  assert.equal(api.requests.length, 3);
});

test('API tokens are sent only to the API, never the image host', async t => {
  const original = globalThis.fetch;
  t.after(() => { globalThis.fetch = original; });
  const calls = [];
  globalThis.fetch = async (url, options) => {
    calls.push({ url: String(url), headers: options.headers });
    if (String(url).includes('/memes/')) return new Response(JSON.stringify({ meme: { id: 'meme-1', url: 'https://r2.emomo.net/1.png' } }), { headers: { 'Content-Type': 'application/json' } });
    return new Response(PNG, { headers: { 'Content-Type': 'image/png' } });
  };
  const directory = await mkdtemp(join(tmpdir(), 'emomo-token-boundary-'));
  await new EmomoClient({ token: 'test-caller-token' }).download('meme-1', directory);
  assert.equal(calls[0].headers.Authorization, 'Bearer test-caller-token');
  assert.equal(calls[1].headers.Authorization, undefined);
});
