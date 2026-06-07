type Route = {
  method: 'GET' | 'POST';
  pattern: RegExp;
  queryParams: ReadonlySet<string>;
  cacheable?: boolean;
};

const ALLOWED_ROUTES: readonly Route[] = [
  {
    method: 'GET',
    pattern: /^\/health$/,
    queryParams: new Set(),
  },
  {
    method: 'GET',
    pattern: /^\/api\/v1\/stats$/,
    queryParams: new Set(),
    cacheable: true,
  },
  {
    method: 'GET',
    pattern: /^\/api\/v1\/categories$/,
    queryParams: new Set(),
    cacheable: true,
  },
  {
    method: 'GET',
    pattern: /^\/api\/v1\/memes$/,
    queryParams: new Set(['category', 'limit', 'offset']),
  },
  {
    method: 'GET',
    pattern: /^\/api\/v1\/memes\/[^/]+$/,
    queryParams: new Set(),
  },
  {
    method: 'POST',
    pattern: /^\/api\/v1\/search$/,
    queryParams: new Set(['collection', 'profile']),
  },
  {
    method: 'POST',
    pattern: /^\/api\/v1\/search\/stream$/,
    queryParams: new Set(['collection', 'profile']),
  },
];

const HOP_BY_HOP_REQUEST_HEADERS = new Set([
  'connection',
  'content-length',
  'host',
  'keep-alive',
  'proxy-authenticate',
  'proxy-authorization',
  'te',
  'trailer',
  'transfer-encoding',
  'upgrade',
]);

const HOP_BY_HOP_RESPONSE_HEADERS = new Set([
  'connection',
  'keep-alive',
  'proxy-authenticate',
  'proxy-authorization',
  'te',
  'trailer',
  'transfer-encoding',
  'upgrade',
]);

const UPSTREAM_RESPONSE_HEADERS_TO_STRIP = [
  /^access-control-/i,
  /^x-proxied-/i,
];

export default {
  async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    const corsHeaders = getCorsHeaders(request, env);

    if (request.method === 'OPTIONS') {
      return handleOptions(request, corsHeaders);
    }

    if (!env.HF_TOKEN) {
      return jsonError(500, 'missing_gateway_secret', 'Gateway is missing HF_TOKEN.', corsHeaders);
    }

    const route = matchRoute(request);
    if (!route) {
      return jsonError(404, 'not_found', 'This API route is not exposed by the gateway.', corsHeaders);
    }

    const validationError = validateRequest(request, route, env);
    if (validationError) {
      return jsonError(validationError.status, validationError.code, validationError.message, corsHeaders);
    }

    if (route.cacheable) {
      const cached = await caches.default.match(request);
      if (cached) {
        return withCorsHeaders(cached, corsHeaders, 'HIT');
      }
    }

    const upstreamRequest = buildUpstreamRequest(request, env);
    const upstreamResponse = await fetch(upstreamRequest);
    const response = buildClientResponse(
      upstreamResponse,
      corsHeaders,
      Boolean(route.cacheable),
      parsePositiveInteger(env.CACHE_TTL_SECONDS, 30)
    );

    if (route.cacheable && response.ok) {
      ctx.waitUntil(caches.default.put(request, response.clone()));
    }

    return response;
  },
};

function handleOptions(request: Request, corsHeaders: HeadersInit): Response {
  const requestedMethod = request.headers.get('Access-Control-Request-Method')?.toUpperCase();
  const syntheticRequest = new Request(request.url, {
    method: requestedMethod || 'GET',
  });

  if (!requestedMethod || !matchRoute(syntheticRequest)) {
    return new Response(null, { status: 404, headers: corsHeaders });
  }

  return new Response(null, {
    status: 204,
    headers: corsHeaders,
  });
}

function matchRoute(request: Request): Route | undefined {
  const url = new URL(request.url);
  const method = request.method.toUpperCase();
  return ALLOWED_ROUTES.find((route) => route.method === method && route.pattern.test(url.pathname));
}

function validateRequest(
  request: Request,
  route: Route,
  env: Env
): { status: number; code: string; message: string } | undefined {
  const url = new URL(request.url);

  for (const key of url.searchParams.keys()) {
    if (!route.queryParams.has(key)) {
      return {
        status: 400,
        code: 'unsupported_query_parameter',
        message: `Query parameter "${key}" is not supported for this endpoint.`,
      };
    }
  }

  if (request.method === 'POST') {
    const contentType = request.headers.get('content-type') || '';
    if (!contentType.toLowerCase().startsWith('application/json')) {
      return {
        status: 415,
        code: 'unsupported_media_type',
        message: 'POST requests must use application/json.',
      };
    }

    const maxBodyBytes = parsePositiveInteger(env.MAX_REQUEST_BODY_BYTES, 65536);
    const contentLength = request.headers.get('content-length');
    if (contentLength && Number(contentLength) > maxBodyBytes) {
      return {
        status: 413,
        code: 'request_too_large',
        message: 'Request body exceeds the gateway limit.',
      };
    }
  }

  return undefined;
}

function buildUpstreamRequest(request: Request, env: Env): Request {
  const incomingUrl = new URL(request.url);
  const upstreamUrl = new URL(incomingUrl.pathname + incomingUrl.search, env.UPSTREAM_ORIGIN);
  const headers = new Headers();

  for (const [key, value] of request.headers) {
    const normalized = key.toLowerCase();
    if (HOP_BY_HOP_REQUEST_HEADERS.has(normalized) || normalized === 'authorization') {
      continue;
    }
    headers.set(key, value);
  }

  headers.set('Accept', request.headers.get('Accept') || 'application/json');
  headers.set('Authorization', `Bearer ${env.HF_TOKEN}`);
  headers.set('X-Forwarded-Host', incomingUrl.host);
  headers.set('X-Forwarded-Proto', incomingUrl.protocol.replace(':', ''));

  return new Request(upstreamUrl.toString(), {
    method: request.method,
    headers,
    body: request.method === 'GET' || request.method === 'HEAD' ? null : request.body,
    redirect: 'manual',
  });
}

function buildClientResponse(
  upstreamResponse: Response,
  corsHeaders: HeadersInit,
  cacheable: boolean,
  cacheTtlSeconds: number
): Response {
  const headers = new Headers(upstreamResponse.headers);

  for (const header of HOP_BY_HOP_RESPONSE_HEADERS) {
    headers.delete(header);
  }

  for (const header of [...headers.keys()]) {
    if (UPSTREAM_RESPONSE_HEADERS_TO_STRIP.some((pattern) => pattern.test(header))) {
      headers.delete(header);
    }
  }

  headers.delete('set-cookie');
  headers.set('X-Content-Type-Options', 'nosniff');

  for (const [key, value] of new Headers(corsHeaders)) {
    headers.set(key, value);
  }

  if (cacheable && upstreamResponse.ok && isCacheableResponse(upstreamResponse)) {
    headers.set('Cache-Control', `public, max-age=${cacheTtlSeconds}`);
  } else if (!headers.has('Cache-Control')) {
    headers.set('Cache-Control', 'no-store');
  }

  return new Response(upstreamResponse.body, {
    status: upstreamResponse.status,
    statusText: upstreamResponse.statusText,
    headers,
  });
}

function isCacheableResponse(response: Response): boolean {
  const contentType = response.headers.get('content-type') || '';
  return contentType.includes('application/json');
}

function getCorsHeaders(request: Request, env: Env): HeadersInit {
  const origin = request.headers.get('Origin');
  const allowedOrigin = resolveAllowedOrigin(origin, env.CORS_ALLOWED_ORIGINS);
  const headers: Record<string, string> = {
    'Access-Control-Allow-Methods': 'GET,POST,OPTIONS',
    'Access-Control-Allow-Headers': 'Accept,Content-Type',
    'Access-Control-Max-Age': '86400',
  };

  if (allowedOrigin) {
    headers['Access-Control-Allow-Origin'] = allowedOrigin;
    headers['Vary'] = 'Origin';
  }

  return headers;
}

function resolveAllowedOrigin(origin: string | null, configuredOrigins: string): string | undefined {
  if (!origin) {
    return undefined;
  }

  const origins = configuredOrigins
    .split(',')
    .map((entry) => entry.trim())
    .filter(Boolean);

  if (origins.includes('*')) {
    return '*';
  }

  return origins.includes(origin) ? origin : undefined;
}

function withCorsHeaders(response: Response, corsHeaders: HeadersInit, cacheStatus: 'HIT' | 'MISS'): Response {
  const headers = new Headers(response.headers);
  for (const [key, value] of new Headers(corsHeaders)) {
    headers.set(key, value);
  }
  headers.set('X-Emomo-Gateway-Cache', cacheStatus);

  return new Response(response.body, {
    status: response.status,
    statusText: response.statusText,
    headers,
  });
}

function jsonError(status: number, code: string, message: string, corsHeaders: HeadersInit): Response {
  return new Response(JSON.stringify({ error: { code, message } }), {
    status,
    headers: {
      ...Object.fromEntries(new Headers(corsHeaders)),
      'Cache-Control': 'no-store',
      'Content-Type': 'application/json; charset=utf-8',
      'X-Content-Type-Options': 'nosniff',
    },
  });
}

function parsePositiveInteger(value: string | undefined, fallback: number): number {
  if (!value) {
    return fallback;
  }
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}
