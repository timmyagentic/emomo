import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdtemp, readdir, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { run, boundToken } from '../src/cli.js';
import { EmomoClient } from '../src/client.js';
import { PNG } from './helpers.js';
const digest = bytes => createHash('sha256').update(bytes).digest('hex');

test('ambient token is sent only to its explicitly configured endpoint', async t => {
  const calls = [];
  t.mock.method(globalThis, 'fetch', async (url, options) => {
    calls.push({ url: String(url), token: options.headers.Authorization });
    return Response.json({ total_memes: '1' });
  });
  const env = { EMOMO_API_URL: 'https://private.invalid/api/v1', EMOMO_API_TOKEN: 'synthetic-token' };
  await run(['stats', '--api-url', 'https://public.invalid/agent/v1'], { env });
  await run(['stats', '--api-url', 'https://private.invalid/agent/v1'], { env });
  await run(['stats', '--api-url', 'https://private.invalid/api/v1/'], { env });
  await run(['stats', '--api-url', 'https://public.invalid/agent/v1'], { env: { EMOMO_API_TOKEN: 'synthetic-unbound' } });
  assert.deepEqual(calls.map(c => c.token), [undefined, undefined, 'Bearer synthetic-token', undefined]);
});

test('remote canonical hashes survive projection and reject different valid image bytes before saving', async t => {
  const root = await mkdtemp(join(tmpdir(), 'emomo-hash-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  let bytes = PNG, contentHash = digest(PNG);
  t.mock.method(globalThis, 'fetch', async url => String(url).includes('/memes/')
    ? Response.json({ meme: { id: 'one', url: 'https://images.timmyagentic.si/one.png', content_hash: contentHash } })
    : new Response(bytes, { headers: { 'Content-Type': 'image/png' } }));
  const client = new EmomoClient();
  assert.equal((await client.get('one')).meme.sha256, contentHash);
  bytes = Buffer.concat([PNG, Buffer.from('different')]);
  await assert.rejects(client.download('one', root), { code: 'IMAGE_INTEGRITY_ERROR' });
  assert.deepEqual(await readdir(root), []);
  bytes = PNG;
  assert.equal((await client.download('one', root)).integrity, 'verified');
  contentHash = createHash('md5').update(PNG).digest('hex');
  assert.equal((await client.download('one', join(root, 'md5'))).integrity, 'verified_legacy_md5');
  bytes = Buffer.concat([PNG, Buffer.from('changed-md5')]);
  await assert.rejects(client.download('one', join(root, 'bad-md5')), { code: 'IMAGE_INTEGRITY_ERROR' });
  bytes = PNG;
  contentHash = undefined;
  assert.equal((await client.download('one', join(root, 'legacy'))).integrity, 'unverified_no_hash');
  contentHash = 'not-a-sha256';
  await assert.rejects(client.get('one'), { code: 'INVALID_RESPONSE' });
});

test('token bindings cannot hide credentials, query parameters or fragments', () => {
  for (const suffix of ['?tenant=private', '#private']) {
    assert.equal(boundToken('https://private.invalid/api', { EMOMO_API_TOKEN: 'fake', EMOMO_API_TOKEN_URL: 'https://private.invalid/api' + suffix }), undefined);
  }
  assert.equal(boundToken('https://private.invalid/api', { EMOMO_API_TOKEN: 'fake', EMOMO_API_TOKEN_URL: 'https://user:fake@private.invalid/api' }), undefined);
});

test('non-string canonical hashes fail as incompatible responses', async t => {
  t.mock.method(globalThis, 'fetch', async () => Response.json({ meme: { id: 'one', url: 'https://images.timmyagentic.si/one.png', content_hash: ['a'.repeat(64)] } }));
  await assert.rejects(new EmomoClient().get('one'), { code: 'INVALID_RESPONSE' });
});
