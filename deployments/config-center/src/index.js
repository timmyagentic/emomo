const MAX_BODY_BYTES = 128 * 1024;

function jsonResponse(body, init = {}) {
  return new Response(JSON.stringify(body, null, 2), {
    ...init,
    headers: {
      'content-type': 'application/json; charset=utf-8',
      ...(init.headers || {}),
    },
  });
}

function bearerToken(request) {
  const authorization = request.headers.get('authorization') || '';
  const match = authorization.match(/^Bearer\s+(.+)$/i);
  return match ? match[1] : '';
}

function requireToken(request, env, allowedTokens) {
  const token = bearerToken(request);
  return token !== '' && allowedTokens.some((allowed) => allowed && token === allowed);
}

function parseConfigPath(pathname) {
  const parts = pathname.split('/').filter(Boolean);
  if (parts.length !== 5 || parts[0] !== 'v1' || parts[1] !== 'config') {
    return null;
  }

  const [, , project, environment, service] = parts;
  const valid = /^[a-zA-Z0-9._-]+$/;
  if (![project, environment, service].every((part) => valid.test(part))) {
    return null;
  }

  return {
    key: `${project}/${environment}/${service}`,
    project,
    environment,
    service,
  };
}

async function readJsonBody(request) {
  const contentLength = Number(request.headers.get('content-length') || '0');
  if (contentLength > MAX_BODY_BYTES) {
    throw new Error('request body too large');
  }

  const text = await request.text();
  if (new TextEncoder().encode(text).byteLength > MAX_BODY_BYTES) {
    throw new Error('request body too large');
  }

  return JSON.parse(text);
}

function pathMatches(pattern, path) {
  const patternParts = pattern.split('.');
  const pathParts = path.split('.');
  if (patternParts.length !== pathParts.length) {
    return false;
  }
  return patternParts.every((part, index) => part === '*' || part === pathParts[index]);
}

function isSensitiveRawPath(path) {
  const sensitivePaths = [
    'query_expansion.api_key',
    'config.database.url',
    'config.database.password',
    'config.qdrant.api_key',
    'config.storage.access_key',
    'config.storage.secret_key',
    'config.vlm.api_key',
    'config.embeddings.*.api_key',
    'config.search.query_expansion.api_key',
    'config.config_center.token',
    'config.logging.loki_password',
  ];
  return sensitivePaths.some((pattern) => pathMatches(pattern, path));
}

function isSensitiveFieldName(field) {
  const normalized = field.toLowerCase();
  return normalized === 'api_key' ||
    normalized.endsWith('_api_key') ||
    normalized === 'access_key' ||
    normalized.endsWith('_access_key') ||
    normalized === 'secret_key' ||
    normalized.endsWith('_secret_key') ||
    normalized === 'password' ||
    normalized.endsWith('_password') ||
    normalized === 'token' ||
    normalized.endsWith('_token');
}

function assertAllowedSecretPath(secretPath, rawPath) {
  if (!isSensitiveRawPath(rawPath)) {
    throw new Error(`${secretPath} is not an allowed Secrets Store reference path`);
  }
}

function validateSecretReferences(value, path = '') {
  if (Array.isArray(value)) {
    value.forEach((item) => validateSecretReferences(item, `${path}.*`));
    return;
  }
  if (value === null || typeof value !== 'object') {
    return;
  }

  for (const [field, fieldValue] of Object.entries(value)) {
    const fieldPath = path ? `${path}.${field}` : field;
    if (isSensitiveRawPath(fieldPath) && fieldValue !== '') {
      throw new Error(`sensitive field ${fieldPath} must use ${field}_secret`);
    }
    if (!isSensitiveRawPath(fieldPath) && isSensitiveFieldName(field) && fieldValue !== '') {
      throw new Error(`sensitive-like field ${fieldPath} is not allowlisted for raw values`);
    }
    if (field.endsWith('_secret')) {
      if (typeof fieldValue !== 'string') {
        throw new Error(`${fieldPath} must be a string`);
      }
      if (!/^[A-Z][A-Z0-9_]*$/.test(fieldValue)) {
        throw new Error(`${fieldPath} must be a Worker binding name`);
      }
      const rawField = field.slice(0, -'_secret'.length);
      const rawPath = path ? `${path}.${rawField}` : rawField;
      assertAllowedSecretPath(fieldPath, rawPath);
      if (Object.prototype.hasOwnProperty.call(value, rawField) && value[rawField] !== '') {
        throw new Error(`${fieldPath} cannot be used with sibling ${rawField}`);
      }
      continue;
    }
    validateSecretReferences(fieldValue, fieldPath);
  }
}

function validateConfig(config) {
  if (config === null || typeof config !== 'object' || Array.isArray(config)) {
    throw new Error('config must be a JSON object');
  }

  const allowedTopLevelFields = new Set(['version', 'updated_at', 'config', 'query_expansion']);
  for (const field of Object.keys(config)) {
    if (!allowedTopLevelFields.has(field)) {
      throw new Error(`unknown top-level field: ${field}`);
    }
  }

  if (config.version !== undefined && typeof config.version !== 'string') {
    throw new Error('version must be a string');
  }
  if (config.updated_at !== undefined && typeof config.updated_at !== 'string') {
    throw new Error('updated_at must be a string');
  }
  if (config.config !== undefined) {
    if (config.config === null || typeof config.config !== 'object' || Array.isArray(config.config)) {
      throw new Error('config must be an object');
    }
  }

  if (config.query_expansion !== undefined) {
    const qe = config.query_expansion;
    if (qe === null || typeof qe !== 'object' || Array.isArray(qe)) {
      throw new Error('query_expansion must be an object');
    }
    const allowedQueryExpansionFields = new Set(['enabled', 'model', 'api_key', 'api_key_secret', 'base_url']);
    for (const field of Object.keys(qe)) {
      if (!allowedQueryExpansionFields.has(field)) {
        throw new Error(`unknown query_expansion field: ${field}`);
      }
    }
    if (qe.api_key !== undefined && qe.api_key_secret !== undefined) {
      throw new Error('query_expansion cannot include both api_key and api_key_secret');
    }
    if (qe.enabled !== undefined && typeof qe.enabled !== 'boolean') {
      throw new Error('query_expansion.enabled must be a boolean');
    }
    for (const field of ['model', 'api_key', 'api_key_secret', 'base_url']) {
      if (qe[field] !== undefined && typeof qe[field] !== 'string') {
        throw new Error(`query_expansion.${field} must be a string`);
      }
    }
    if (qe.api_key_secret !== undefined && !/^[A-Z][A-Z0-9_]*$/.test(qe.api_key_secret)) {
      throw new Error('query_expansion.api_key_secret must be a Worker binding name');
    }
  }

  validateSecretReferences(config);
}

async function resolveSecretsInPlace(value, env, path = '') {
  if (Array.isArray(value)) {
    await Promise.all(value.map((item) => resolveSecretsInPlace(item, env, `${path}.*`)));
    return;
  }
  if (value === null || typeof value !== 'object') {
    return;
  }

  for (const [field, fieldValue] of Object.entries(value)) {
    const fieldPath = path ? `${path}.${field}` : field;
    if (field.endsWith('_secret')) {
      const bindingName = fieldValue;
      const rawField = field.slice(0, -'_secret'.length);
      const rawPath = path ? `${path}.${rawField}` : rawField;
      assertAllowedSecretPath(fieldPath, rawPath);
      const binding = env[bindingName];
      if (!binding || typeof binding.get !== 'function') {
        throw new Error(`missing Secrets Store binding: ${bindingName}`);
      }

      const secretValue = await binding.get();
      if (!secretValue) {
        throw new Error(`empty Secrets Store value: ${bindingName}`);
      }

      value[rawField] = secretValue;
      delete value[field];
      continue;
    }
    await resolveSecretsInPlace(fieldValue, env, fieldPath);
  }
}

async function resolveSecrets(config, env) {
  const resolved = JSON.parse(JSON.stringify(config));
  await resolveSecretsInPlace(resolved, env);
  return resolved;
}

async function handleGet(request, env, parsed) {
  if (!requireToken(request, env, [env.READ_TOKEN, env.ADMIN_TOKEN])) {
    return jsonResponse({ error: 'unauthorized' }, { status: 401 });
  }

  const storedConfig = await env.CONFIG_KV.get(parsed.key, 'json');
  if (!storedConfig) {
    return jsonResponse({ error: 'config not found', key: parsed.key }, { status: 404 });
  }

  let resolvedConfig;
  try {
    resolvedConfig = await resolveSecrets(storedConfig, env);
  } catch (error) {
    return jsonResponse({ error: 'secret resolution failed', message: String(error.message || error) }, { status: 500 });
  }

  return jsonResponse(resolvedConfig, {
    headers: {
      'cache-control': 'no-store',
    },
  });
}

async function handlePut(request, env, parsed) {
  if (!requireToken(request, env, [env.ADMIN_TOKEN])) {
    return jsonResponse({ error: 'unauthorized' }, { status: 401 });
  }

  let config;
  try {
    config = await readJsonBody(request);
    validateConfig(config);
  } catch (error) {
    return jsonResponse({ error: String(error.message || error) }, { status: 400 });
  }

  const now = new Date().toISOString();
  const stored = {
    ...config,
    version: config.version || now,
    updated_at: now,
  };

  await env.CONFIG_KV.put(parsed.key, JSON.stringify(stored));
  return jsonResponse({
    ok: true,
    key: parsed.key,
    version: stored.version,
    updated_at: stored.updated_at,
  });
}

export default {
  async fetch(request, env) {
    const url = new URL(request.url);

    if (url.pathname === '/healthz') {
      return jsonResponse({ ok: true });
    }

    const parsed = parseConfigPath(url.pathname);
    if (!parsed) {
      return jsonResponse({
        error: 'not found',
        expected: '/v1/config/{project}/{environment}/{service}',
      }, { status: 404 });
    }

    if (request.method === 'GET') {
      return handleGet(request, env, parsed);
    }
    if (request.method === 'PUT') {
      return handlePut(request, env, parsed);
    }

    return jsonResponse({ error: 'method not allowed' }, {
      status: 405,
      headers: { allow: 'GET, PUT' },
    });
  },
};
