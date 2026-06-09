import assert from 'node:assert/strict';
import { afterEach, test } from 'node:test';
import worker from './index';

type FetchCall = {
  input: RequestInfo | URL;
};

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
});

function makeEnv(overrides: Record<string, unknown> = {}): Env {
  return {
    UPSTREAM_ORIGIN: 'https://upstream.example',
    HF_TOKEN: 'test-token',
    CORS_ALLOWED_ORIGINS: 'https://emomo.net',
    CACHE_TTL_SECONDS: '30',
    MAX_REQUEST_BODY_BYTES: '128',
    MAX_LIST_WINDOW: '120',
    EMOMO_RATE_LIMITER: {
      limit: async () => ({ success: true }),
    } as unknown as RateLimit,
    ...overrides,
  } as unknown as Env;
}

function makeCtx(): ExecutionContext {
  return {
    waitUntil() {},
    passThroughOnException() {},
  } as unknown as ExecutionContext;
}

function mockUpstream(): FetchCall[] {
  const calls: FetchCall[] = [];
  globalThis.fetch = async (input) => {
    calls.push({ input });
    return new Response(JSON.stringify({ ok: true }), {
      headers: { 'Content-Type': 'application/json' },
    });
  };
  return calls;
}

test('blocks meme list pagination past the public window', async () => {
  const calls = mockUpstream();
  const response = await worker.fetch(
    new Request('https://api.emomo.net/api/v1/memes?limit=60&offset=120', {
      headers: { Origin: 'https://emomo.net' },
    }),
    makeEnv(),
    makeCtx()
  );

  assert.equal(response.status, 403);
  assert.equal(calls.length, 0);
  assert.deepEqual(await response.json(), {
    error: {
      code: 'bulk_listing_blocked',
      message: 'Meme listing is limited to the public browse window.',
    },
  });
});

test('allows meme list requests within the public window', async () => {
  const calls = mockUpstream();
  const response = await worker.fetch(
    new Request('https://api.emomo.net/api/v1/memes?limit=60&offset=60', {
      headers: { Origin: 'https://emomo.net' },
    }),
    makeEnv(),
    makeCtx()
  );

  assert.equal(response.status, 200);
  assert.equal(calls.length, 1);
});

test('rate limits scraper-like request bursts before proxying upstream', async () => {
  const calls = mockUpstream();
  const response = await worker.fetch(
    new Request('https://api.emomo.net/api/v1/search', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'CF-Connecting-IP': '203.0.113.10',
        Origin: 'https://emomo.net',
      },
      body: JSON.stringify({ query: 'cat', topK: 50 }),
    }),
    makeEnv({
      EMOMO_RATE_LIMITER: {
        limit: async ({ key }: { key: string }) => {
          assert.equal(key, 'search:203.0.113.10');
          return { success: false };
        },
      } as unknown as RateLimit,
    }),
    makeCtx()
  );

  assert.equal(response.status, 429);
  assert.equal(calls.length, 0);
  assert.deepEqual(await response.json(), {
    error: {
      code: 'rate_limited',
      message: 'Too many requests. Please slow down.',
    },
  });
});

test('rejects search requests that ask for too many results', async () => {
  const calls = mockUpstream();
  const response = await worker.fetch(
    new Request('https://api.emomo.net/api/v1/search', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Origin: 'https://emomo.net',
      },
      body: JSON.stringify({ query: 'cat', topK: 101 }),
    }),
    makeEnv(),
    makeCtx()
  );

  assert.equal(response.status, 400);
  assert.equal(calls.length, 0);
  assert.deepEqual(await response.json(), {
    error: {
      code: 'search_top_k_too_large',
      message: 'Search topK is limited to 100.',
    },
  });
});

test('rejects oversized POST bodies even when Content-Length is absent', async () => {
  const calls = mockUpstream();
  const response = await worker.fetch(
    new Request('https://api.emomo.net/api/v1/search', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Origin: 'https://emomo.net',
      },
      body: JSON.stringify({ query: 'x'.repeat(256) }),
    }),
    makeEnv(),
    makeCtx()
  );

  assert.equal(response.status, 413);
  assert.equal(calls.length, 0);
  assert.deepEqual(await response.json(), {
    error: {
      code: 'request_too_large',
      message: 'Request body exceeds the gateway limit.',
    },
  });
});
