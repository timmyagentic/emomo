import { createHash } from 'node:crypto';
import type { Miniflare } from 'miniflare';
// Public deterministic test credentials; never accepted by production.
export const token = (id: number) => `emomo_test-${id}_${createHash('sha256').update(`fixture-${id}`).digest('hex')}`;
export const registry = Object.fromEntries(Array.from({ length: 512 }, (_, id) => [`test-${id}`, { hash: createHash('sha256').update(token(id)).digest('hex') }]));
export const policies = Object.fromEntries(['search', 'read', 'download'].map(op => [op, { perSecond: 100000, perDay: 100000, inFlight: 10000, failures: 10000, cooldownSeconds: 1 }]));
export const bindings = { ACCESS_KEYS_JSON: JSON.stringify(registry), SERVICE_PAUSED: 'false', GLOBAL_LIMITS_JSON: JSON.stringify(policies) };
export const admission = { ADMISSION: { className: 'Admission', useSQLite: true } };
let counter = 0;
export function authorizedFetch(mf: Miniflare, url: string, init: Parameters<Miniflare['dispatchFetch']>[1] = {}) {
  return mf.dispatchFetch(url, { ...init, headers: { authorization: `Bearer ${token(counter++ % 512)}`, ...init.headers as Record<string, string> } });
}
export const downloadEntry = "export { default, PerIpLimiter } from './src/download.ts'; export { Admission } from './src/access.ts';";
