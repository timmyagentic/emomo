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
    SERVICE_MODE: 'legacy',
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

test('Agent mode keeps every legacy API method paused, even without secrets', async () => {
  const calls = mockUpstream();
  for (const method of ['GET', 'POST', 'OPTIONS', 'DELETE']) {
    const response = await worker.fetch(
      new Request('https://api.emomo.net/api/v1/search', { method }),
      makeEnv({ SERVICE_MODE: 'agent', AGENT_API_ENABLED: 'true', HF_TOKEN: undefined }),
      makeCtx()
    );
    assert.equal(response.status, 410);
    assert.equal(response.headers.get('X-Emomo-Service-State'), 'paused');
  }
  assert.equal(calls.length, 0);
});

test('missing or misspelled mode variables never reopen legacy clients', async () => {
  const calls = mockUpstream();
  for (const mode of [undefined, '', 'agnt']) {
    for (const enabled of [undefined, 'false']) {
      for (const path of ['/api/v1/search', '/agent/v1/search']) {
        const response = await worker.fetch(
          new Request(`https://api.emomo.net${path}`, { method: 'POST' }),
          makeEnv({ SERVICE_MODE: mode, AGENT_API_ENABLED: enabled, HF_TOKEN: undefined }),
          makeCtx()
        );
        assert.equal(response.status, 410);
      }
    }
  }
  assert.equal(calls.length, 0);
});

test('Agent API remains paused until explicitly enabled', async () => {
  const calls = mockUpstream();
  const response = await worker.fetch(
    new Request('https://api.emomo.net/agent/v1/search', { method: 'POST' }),
    makeEnv({ SERVICE_MODE: 'agent', AGENT_API_ENABLED: 'false' }),
    makeCtx()
  );
  assert.equal(response.status, 410);
  assert.equal(calls.length, 0);
});

test('Agent search maps to existing DTOs and keeps HF credentials server-side', async () => {
  const calls = mockUpstream();
  const response = await worker.fetch(
    new Request('https://api.emomo.net/agent/v1/search', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: 'Bearer caller-token', 'CF-Connecting-IP': '203.0.113.20' },
      body: JSON.stringify({ query: '  想下班  ', top_k: 8, text_presence: 2 }),
    }),
    makeEnv({ SERVICE_MODE: 'agent', AGENT_API_ENABLED: 'true' }),
    makeCtx()
  );
  assert.equal(response.status, 200);
  assert.equal(calls.length, 1);
  const upstream = calls[0].input as Request;
  assert.equal(upstream.url, 'https://upstream.example/api/v1/search');
  assert.equal(upstream.headers.get('Authorization'), 'Bearer test-token');
  assert.deepEqual(await upstream.json(), { query: '想下班', top_k: 8, text_presence: 2 });
  assert.equal(response.headers.get('Authorization'), null);
});

test('Agent detail is exposed but list, stream, health, and admin remain blocked', async () => {
  const calls = mockUpstream();
  for (const path of ['/agent/v1/memes', '/agent/v1/search/stream', '/health', '/', '/agent/v1/admin']) {
    const response = await worker.fetch(new Request(`https://api.emomo.net${path}`), makeEnv({ SERVICE_MODE: 'agent', AGENT_API_ENABLED: 'true' }), makeCtx());
    assert.equal(response.status, 404, path);
  }
  assert.equal(calls.length, 0);
  const response = await worker.fetch(new Request('https://api.emomo.net/agent/v1/memes/meme-1'), makeEnv({ SERVICE_MODE: 'agent', AGENT_API_ENABLED: 'true' }), makeCtx());
  assert.equal(response.status, 200);
  assert.equal((calls[0].input as Request).url, 'https://upstream.example/api/v1/memes/meme-1');
});

test('Agent requests obey rate limits before making any upstream call', async () => {
  const calls = mockUpstream();
  const response = await worker.fetch(new Request('https://api.emomo.net/agent/v1/search', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ query: '开会', top_k: 8 }),
  }), makeEnv({ SERVICE_MODE: 'agent', AGENT_API_ENABLED: 'true', EMOMO_RATE_LIMITER: { limit: async () => ({ success: false }) } }), makeCtx());
  assert.equal(response.status, 429);
  assert.equal(calls.length, 0);
});

test('Agent search rejects invalid queries and top_k without paid upstream work', async () => {
  const calls = mockUpstream();
  for (const payload of [{ query: '' }, { query: 'x'.repeat(161) }, { query: 'a', top_k: 0 }, { query: 'a', top_k: 1.5 }, { query: 'a', top_k: '8' }, []]) {
    const response = await worker.fetch(new Request('https://api.emomo.net/agent/v1/search', {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload),
    }), makeEnv({ SERVICE_MODE: 'agent', AGENT_API_ENABLED: 'true', MAX_REQUEST_BODY_BYTES: '65536' }), makeCtx());
    assert.equal(response.status, 400);
  }
  assert.equal(calls.length, 0);
});

test('Agent search supplies a small default and normalizes camelCase topK', async () => {
  const calls = mockUpstream();
  for (const body of [{ query: '开会' }, { query: '开会', topK: 3 }]) {
    const response = await worker.fetch(new Request('https://api.emomo.net/agent/v1/search', {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
    }), makeEnv({ SERVICE_MODE: 'agent', AGENT_API_ENABLED: 'true' }), makeCtx());
    assert.equal(response.status, 200);
  }
  assert.deepEqual(await (calls[0].input as Request).json(), { query: '开会', top_k: 8 });
  assert.deepEqual(await (calls[1].input as Request).json(), { query: '开会', top_k: 3 });
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
