import { DurableObject } from 'cloudflare:workers';
import { uncached } from './rate-limit.js';

export type Operation = 'search' | 'read' | 'download';
export type Quota = { hour: number; day: number };
export type GlobalLimits = { perSecond: number; perDay: number; inFlight: number; failures: number; cooldownSeconds: number };
type KeyRecord = { hash: string; disabled?: boolean; expiresAt?: number; quotas?: Partial<Record<Operation, Quota>> };
type Counter = { hour: number; hourly: number; day: number; daily: number; next: number };
type GlobalState = { second: number; count: number; day: number; daily: number; leases: Record<string, number>; errors: number[]; openUntil: number; probe?: string };
export const DEFAULT_QUOTAS: Record<Operation, Quota> = {
  search: { hour: 120, day: 500 }, read: { hour: 600, day: 2000 }, download: { hour: 120, day: 500 },
};
const HOUR = 3_600_000;
const DAY = 24 * HOUR;
const LEASE_MS = 30_000;
const positive = (n: unknown): n is number => Number.isSafeInteger(n) && Number(n) > 0 && Number(n) <= 1_000_000;

export class AccessError extends Error {
  constructor(public status: number, message: string, public retry = 0, public reason = '') { super(message); }
}
export function accessError(error: AccessError): Response {
  const response = uncached(Response.json({ error: error.message }, { status: error.status }));
  if (error.retry) response.headers.set('retry-after', String(error.retry));
  if (error.reason) response.headers.set('x-emomo-limit', error.reason);
  if (error.status === 401) response.headers.set('www-authenticate', 'Bearer realm="emomo"');
  return response;
}

export async function authenticate(request: Request, registry: string, op: Operation): Promise<{ id: string; quota: Quota }> {
  const match = /^Bearer (emomo_([a-zA-Z0-9-]{1,48})_[a-f0-9]{64})$/.exec(request.headers.get('authorization') ?? '');
  if (!match) throw new AccessError(401, 'A valid Emomo access key is required.');
  if (!registry || registry.length > 131072) throw new AccessError(503, 'Access service unavailable.', 30);
  let keys: Record<string, KeyRecord>;
  try { keys = JSON.parse(registry); } catch { throw new AccessError(503, 'Access service unavailable.', 30); }
  if (!keys || typeof keys !== 'object' || Array.isArray(keys)) throw new AccessError(503, 'Access service unavailable.', 30);
  const key = Object.hasOwn(keys, match[2]) ? keys[match[2]] : undefined;
  const digest = [...new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(match[1])))].map(b => b.toString(16).padStart(2, '0')).join('');
  if (!key || key.disabled || !/^[a-f0-9]{64}$/.test(key.hash) || key.hash !== digest || key.expiresAt !== undefined && (!Number.isSafeInteger(key.expiresAt) || key.expiresAt <= Date.now())) {
    throw new AccessError(401, 'A valid Emomo access key is required.');
  }
  const quota = key.quotas?.[op] ?? DEFAULT_QUOTAS[op];
  if (!positive(quota.hour) || !positive(quota.day)) throw new AccessError(503, 'Access policy unavailable.', 30);
  return { id: match[2], quota };
}

// Only trusted Worker bindings call these RPCs; no public HTTP management route.
export class Admission extends DurableObject {
  async quota(policy: Quota, interval: number): Promise<{ retry: number; reason: string }> {
    return this.ctx.storage.transaction(async tx => {
      const now = Date.now(), hour = Math.floor(now / HOUR), day = Math.floor(now / DAY);
      const s = await tx.get<Counter>('quota') ?? { hour, hourly: 0, day, daily: 0, next: 0 };
      if (s.hour !== hour) { s.hour = hour; s.hourly = 0; }
      if (s.day !== day) { s.day = day; s.daily = 0; }
      if (s.daily >= policy.day) return { retry: Math.ceil(((day + 1) * DAY - now) / 1000), reason: 'day' };
      if (s.hourly >= policy.hour) return { retry: Math.ceil(((hour + 1) * HOUR - now) / 1000), reason: 'hour' };
      if (s.next > now) return { retry: Math.ceil((s.next - now) / 1000), reason: 'rate' };
      s.hourly++; s.daily++; s.next = now + interval;
      await tx.put('quota', s);
      await tx.setAlarm((day + 1) * DAY + 60_000);
      return { retry: 0, reason: '' };
    });
  }

  async enter(limits: GlobalLimits): Promise<{ lease: string; retry: number; reason: string }> {
    return this.ctx.storage.transaction(async tx => {
      const now = Date.now(), second = Math.floor(now / 1000), day = Math.floor(now / DAY);
      const s = await tx.get<GlobalState>('global') ?? { second, count: 0, day, daily: 0, leases: {}, errors: [], openUntil: 0 };
      for (const [id, deadline] of Object.entries(s.leases)) if (deadline <= now) delete s.leases[id];
      const deny = (retry: number, reason: string) => ({ lease: '', retry, reason });
      if (s.openUntil > now) return deny(Math.ceil((s.openUntil - now) / 1000), 'circuit');
      if (s.probe && s.leases[s.probe]) return deny(1, 'circuit');
      if (s.second !== second) { s.second = second; s.count = 0; }
      if (s.day !== day) { s.day = day; s.daily = 0; }
      if (s.daily >= limits.perDay) return deny(Math.ceil(((day + 1) * DAY - now) / 1000), 'global-day');
      if (s.count >= limits.perSecond) return deny(1, 'global-rate');
      if (Object.keys(s.leases).length >= limits.inFlight) return deny(1, 'capacity');
      const lease = crypto.randomUUID();
      s.leases[lease] = now + LEASE_MS;
      if (s.openUntil) s.probe = lease;
      s.count++; s.daily++;
      await tx.put('global', s);
      return { lease, retry: 0, reason: '' };
    });
  }

  async finish(lease: string, failed: boolean, limits: GlobalLimits): Promise<void> {
    await this.ctx.storage.transaction(async tx => {
      const s = await tx.get<GlobalState>('global');
      if (!s || !Object.hasOwn(s.leases, lease)) return;
      const now = Date.now();
      delete s.leases[lease];
      s.errors = s.errors.filter(time => time > now - 30_000);
      if (failed) s.errors.push(now);
      if (s.probe === lease) {
        s.probe = undefined;
        s.openUntil = failed ? now + limits.cooldownSeconds * 1000 : 0;
        if (!failed) s.errors = [];
      } else if (s.errors.length >= limits.failures) s.openUntil = Math.max(s.openUntil, now + limits.cooldownSeconds * 1000);
      await tx.put('global', s);
    });
  }

  async alarm(): Promise<void> {
    const s = await this.ctx.storage.get<Counter>('quota');
    if (s && Math.floor(Date.now() / DAY) <= s.day) await this.ctx.storage.setAlarm((s.day + 1) * DAY + 60_000);
    else await this.ctx.storage.deleteAll();
  }
}

type GuardEnv = Pick<Env, 'ADMISSION' | 'GLOBAL_LIMITS_JSON' | 'SERVICE_PAUSED'> | Pick<DownloadEnv, 'ADMISSION' | 'GLOBAL_LIMITS_JSON' | 'SERVICE_PAUSED'>;
export async function admit(env: GuardEnv, op: Operation, identity: { id: string; quota: Quota }) {
  if (env.SERVICE_PAUSED !== 'false') throw new AccessError(503, 'Service temporarily paused.', 30, 'paused');
  // Wrangler cannot infer the class type across a script_name binding.
  const admission = env.ADMISSION as DurableObjectNamespace<Admission>;
  const result = await admission.getByName(`key/${identity.id}/${op}`).quota(identity.quota, op === 'read' ? 0 : 1000);
  if (result.retry) throw new AccessError(429, 'Access key quota reached.', result.retry, result.reason);
  let all: Record<Operation, GlobalLimits>;
  try { all = JSON.parse(env.GLOBAL_LIMITS_JSON); } catch { throw new AccessError(503, 'Service policy unavailable.', 30); }
  const limits = all?.[op];
  if (!limits || !['perSecond', 'perDay', 'inFlight', 'failures', 'cooldownSeconds'].every(k => positive(limits[k as keyof GlobalLimits]))) throw new AccessError(503, 'Service policy unavailable.', 30);
  const gate = admission.getByName(`global/${op}`);
  const reservation = await gate.enter(limits);
  if (reservation.retry) throw new AccessError(503, 'Service busy; try later.', reservation.retry, reservation.reason);
  return { finish: (failed: boolean) => gate.finish(reservation.lease, failed, limits) };
}
