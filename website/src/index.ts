import { fromJson, toJson, type DescMessage, type JsonValue } from '@bufbuild/protobuf';
import { RequestLoginCodeSchema, LoginCodeResponseSchema, VerifyLoginCodeSchema, AccountActionResponseSchema, CreateAccountKeySchema, AccountKeyCreatedSchema, AccountProfileSchema } from '../gen/emomo/v1/account_pb.js';
import { Account, digest, random, SESSION_MS } from './account.js';
import { DEFAULT_QUOTAS, type Operation } from '../../deployments/cloudflare/agent-search/src/policy.js';
export { Account } from './account.js';

class HttpError extends Error {
  constructor(public status: number, message: string, public retry = 0) { super(message); }
}
const COOKIE = '__Host-emomo_session';
const response = (data: unknown, status = 200, schema?: DescMessage) => Response.json(schema ? toJson(schema, fromJson(schema, data as JsonValue), { alwaysEmitImplicit: true }) : data, { status, headers: {
  'cache-control': 'no-store', 'cdn-cache-control': 'no-store', 'x-content-type-options': 'nosniff',
  'referrer-policy': 'no-referrer', 'x-frame-options': 'DENY',
} });
const hmac = async (secret: string, value: string) => {
  if (!secret || secret.length < 32) throw new HttpError(503, '登录服务暂不可用，请稍后再试。');
  const key = await crypto.subtle.importKey('raw', new TextEncoder().encode(secret), { name: 'HMAC', hash: 'SHA-256' }, false, ['sign']);
  return [...new Uint8Array(await crypto.subtle.sign('HMAC', key, new TextEncoder().encode(value)))].map(b => b.toString(16).padStart(2, '0')).join('');
};
function emailAddress(value: unknown): string {
  if (typeof value !== 'string') throw new HttpError(400, '请输入有效邮箱。');
  const email = value.trim().toLowerCase();
  if (email.length > 254 || !/^[a-z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?\.[a-z]{2,63}$/.test(email)) throw new HttpError(400, '请输入有效邮箱。');
  return email;
}
async function body(request: Request, schema: DescMessage): Promise<Record<string, unknown>> {
  if (!request.headers.get('content-type')?.startsWith('application/json')) throw new HttpError(415, '请使用 JSON 请求。');
  if (!request.body) throw new HttpError(400, '请求为空。');
  const reader = request.body.getReader(); let length = 0, text = '', expired = false;
  const decoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM: false });
  const timer = setTimeout(() => { expired = true; void reader.cancel().catch(() => {}); }, 5000);
  try {
    for (;;) { const part = await reader.read(); if (expired) throw new HttpError(408, '请求超时。'); if (part.done) break; length += part.value.length; if (length > 2048) { void reader.cancel(); throw new HttpError(413, '请求过大。'); } text += decoder.decode(part.value, { stream: true }); }
    text += decoder.decode(); const parsed = JSON.parse(text);
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error(); return toJson(schema, fromJson(schema, parsed), { alwaysEmitImplicit: true }) as Record<string, unknown>;
  } catch (e) { if (e instanceof HttpError) throw e; throw new HttpError(400, '请求格式不正确。'); }
  finally { clearTimeout(timer); reader.releaseLock(); }
}
function identity(request: Request) {
  const value = request.headers.get('cookie')?.split(';').map(s => s.trim()).find(s => s.startsWith(COOKIE + '='))?.slice(COOKIE.length + 1);
  const match = /^([a-f0-9]{24})\.([a-f0-9]{64})$/.exec(value ?? '');
  if (!match) throw new HttpError(401, '请先登录。');
  return { id: match[1], secret: match[2] };
}
function cookie(value: string, maxAge: number) { return `${COOKIE}=${value}; Path=/; Max-Age=${maxAge}; HttpOnly; Secure; SameSite=Lax`; }
async function limited(env: WebsiteEnv, name: string, hour: number, day: number) {
  const retry = await env.ACCOUNT_STORE.getByName(name).reserve(hour, day);
  if (retry) throw new HttpError(429, '操作频繁，请稍后再试。', retry);
}
function otp() { let n: number; do { n = crypto.getRandomValues(new Uint32Array(1))[0]; } while (n >= 4_294_000_000); return String(n % 1_000_000).padStart(6, '0'); }

async function api(request: Request, env: WebsiteEnv): Promise<Response> {
  const url = new URL(request.url);
  if (env.ACCOUNTS_ENABLED !== 'true') throw new HttpError(503, '登录服务暂不可用，请稍后再试。');
  if (url.search) throw new HttpError(400, '请求地址不正确。');
  const isPost = request.method === 'POST';
  if (!['GET', 'POST'].includes(request.method)) throw new HttpError(405, '不支持的请求方式。');
  const allowed = env.SITE_ORIGINS.split(',');
  if (!allowed.includes(url.origin) || isPost && request.headers.get('origin') !== url.origin) throw new HttpError(403, '请从 Emomo 官网操作。');
  const ip = request.headers.get('cf-connecting-ip');
  if (!ip) throw new HttpError(503, '登录服务暂不可用，请稍后再试。');
  const rate = await env.ACCOUNT_RATE_LIMITER.limit({ key: ip });
  if (!rate.success) throw new HttpError(429, '操作频繁，请稍后再试。', 60);
  const path = url.pathname;
  if (path === '/api/account/code' && isPost) {
    const data = await body(request, RequestLoginCodeSchema), email = emailAddress(data.email);
    const accountId = (await hmac(env.ACCOUNT_SECRET, `account:${email}`)).slice(0, 24);
    const ipHash = await hmac(env.ACCOUNT_SECRET, `ip:${ip}`);
    const challenge = random(16), code = otp();
    const hash = await hmac(env.ACCOUNT_SECRET, `code:${accountId}:${challenge}:${code}`);
    const account = env.ACCOUNT_STORE.getByName(`user/${accountId}`);
    const retry = await account.claimCode(challenge);
    if (retry) throw new HttpError(429, '验证码已发送，请稍后再试。', retry);
    try {
      // Keep conservative, non-refundable reservations, including partial
      // admission and delivery failure. Cooldown rejects never reach budgets.
      await limited(env, `send-ip/${ipHash}`, 5, 20);
      await limited(env, `send-email/${accountId}`, 5, 10);
      await limited(env, 'send-global', 100, 500);
      if (!await account.beginCode(email, challenge, hash)) throw new HttpError(503, '请求已过期，请重新获取验证码。');
    } finally { await account.releaseCode(challenge); }
    try {
      await env.EMAIL.send({ from: { email: env.EMAIL_FROM, name: 'Emomo' }, to: email,
        subject: '你的 Emomo 登录验证码',
        text: `你的 Emomo 验证码是 ${code}，10 分钟内有效。首次验证会自动创建账户。如果不是你本人操作，请忽略这封邮件。请勿把验证码告诉他人。`,
        html: `<p>你的 Emomo 登录验证码：</p><p style="font-size:28px;letter-spacing:6px;font-weight:bold">${code}</p><p>10 分钟内有效。首次验证会自动创建账户。</p><p>如果不是你本人操作，请忽略这封邮件。请勿把验证码告诉他人。</p>` });
    } catch { await account.cancelCode(challenge); throw new HttpError(503, '验证码发送失败，请稍后重试。'); }
    return response({ challenge, expiresIn: 600, retryAfter: 60 }, 200, LoginCodeResponseSchema);
  }
  if (path === '/api/account/verify' && isPost) {
    const data = await body(request, VerifyLoginCodeSchema), email = emailAddress(data.email);
    if (typeof data.challenge !== 'string' || !/^[a-f0-9]{32}$/.test(data.challenge) || typeof data.code !== 'string' || !/^\d{6}$/.test(data.code)) throw new HttpError(400, '请输入 6 位验证码。');
    const id = (await hmac(env.ACCOUNT_SECRET, `account:${email}`)).slice(0, 24);
    const ipHash = await hmac(env.ACCOUNT_SECRET, `ip:${ip}`);
    await limited(env, `verify-ip/${ipHash}`, 50, 200);
    const hash = await hmac(env.ACCOUNT_SECRET, `code:${id}:${data.challenge}:${data.code}`), secret = random(32);
    if (!await env.ACCOUNT_STORE.getByName(`user/${id}`).verify(data.challenge, hash, await digest(secret))) throw new HttpError(400, '验证码错误或已过期，请重新获取。');
    const out = response({ ok: true }, 200, AccountActionResponseSchema); out.headers.set('set-cookie', cookie(`${id}.${secret}`, SESSION_MS / 1000)); return out;
  }
  const auth = identity(request), account = env.ACCOUNT_STORE.getByName(`user/${auth.id}`), sessionHash = await digest(auth.secret);
  const profile = await account.profile(sessionHash);
  if (!profile) throw new HttpError(401, '登录已过期，请重新登录。');
  if (path === '/api/account/me' && request.method === 'GET') {
    const usage: Record<string, unknown> = {};
    const admission = env.ADMISSION as unknown as { getByName(name: string): { usage(policy: { hour: number; day: number }): Promise<unknown> } };
    await Promise.all((Object.keys(DEFAULT_QUOTAS) as Operation[]).map(async op => { usage[op] = await admission.getByName(`key/web/${auth.id}/${op}`).usage(DEFAULT_QUOTAS[op]); }));
    return response({ ...profile, usage }, 200, AccountProfileSchema);
  }
  if (path === '/api/account/logout' && isPost) {
    await account.logout(sessionHash); const out = response({ ok: true }, 200, AccountActionResponseSchema); out.headers.set('set-cookie', cookie('', 0)); return out;
  }
  if (path === '/api/account/keys' && isPost) {
    const data = await body(request, CreateAccountKeySchema);
    if (typeof data.name !== 'string' || !data.name.trim() || [...data.name.trim()].length > 40 || /[\r\n\x00-\x1f]/.test(data.name)) throw new HttpError(400, '密钥名称需为 1–40 个字符。');
    const result = await account.createKey(sessionHash, auth.id, data.name.trim());
    if (result.error === 'unauthorized') throw new HttpError(401, '请重新登录。');
    if (result.error) throw new HttpError(429, result.error === 'active-limit' ? '最多保留 3 把有效密钥，请先撤销一把。' : '今天创建密钥次数过多，请明天再试。');
    return response(result, 201, AccountKeyCreatedSchema);
  }
  const revoked = /^\/api\/account\/keys\/([a-f0-9]{16})\/revoke$/.exec(path);
  if (revoked && isPost) {
    if (!await account.revokeKey(sessionHash, revoked[1])) throw new HttpError(404, '密钥不存在。'); return response({ ok: true }, 200, AccountActionResponseSchema);
  }
  throw new HttpError(404, '页面不存在。');
}

export default {
  async fetch(request: Request, env: WebsiteEnv): Promise<Response> {
    if (!new URL(request.url).pathname.startsWith('/api/account/')) return env.ASSETS.fetch(request);
    try { return await api(request, env); }
    catch (e) {
      const error = e instanceof HttpError ? e : new HttpError(503, '服务暂不可用，请稍后再试。');
      const out = response({ error: error.message }, error.status); if (error.retry) out.headers.set('retry-after', String(error.retry)); return out;
    }
  },
} satisfies ExportedHandler<WebsiteEnv>;
