import { DurableObject } from 'cloudflare:workers';

const INTERVAL_MS = 1000;
const IDLE_TTL_MS = 60_000;

// Each namespace has one object per hashed, Cloudflare-provided client IP.
// SQLite persists the reservation across eviction; no edge-local clock/counter.
export class PerIpLimiter extends DurableObject {
  async take(): Promise<{ allowed: boolean; retryAfterSeconds: number }> {
    // Storage input/output gates serialize this read-modify-write operation.
    const next = await this.ctx.storage.get<number>('next_ms');
    const now = Date.now();
    if (next && now < next) return { allowed: false, retryAfterSeconds: Math.max(1, Math.ceil((next - now) / 1000)) };
    await this.ctx.storage.put('next_ms', now + INTERVAL_MS);
    await this.ctx.storage.setAlarm(now + INTERVAL_MS + IDLE_TTL_MS);
    return { allowed: true, retryAfterSeconds: 0 };
  }

  async alarm(): Promise<void> {
    const next = await this.ctx.storage.get<number>('next_ms');
    if (next && Date.now() < next + IDLE_TTL_MS) {
      await this.ctx.storage.setAlarm(next + IDLE_TTL_MS);
    } else {
      await this.ctx.storage.deleteAll();
    }
  }
}

export function rateLimited(seconds: number): Response {
  return Response.json({ error: 'Rate limited.' }, { status: 429, headers: { 'retry-after': String(seconds), 'cache-control': 'no-store', 'cdn-cache-control': 'no-store' } });
}

export async function enforceInterval(request: Request, limiter: DurableObjectNamespace<PerIpLimiter>): Promise<Response | null> {
  const ip = request.headers.get('cf-connecting-ip');
  // No spoofable X-Forwarded-For fallback, and no silent bypass without the binding.
  if (!ip || !limiter) throw new Error('Client identity or strict limiter unavailable.');
  const bytes = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(ip));
  const name = [...new Uint8Array(bytes)].map(b => b.toString(16).padStart(2, '0')).join('');
  const result = await limiter.getByName(name).take();
  return result.allowed ? null : rateLimited(result.retryAfterSeconds);
}

// Internal cache entries are shared; every external request must reach guards.
export function uncached(response: Response): Response {
  const out = new Response(response.body, response);
  out.headers.set('cache-control', 'no-store');
  out.headers.set('cdn-cache-control', 'no-store');
  out.headers.set('cloudflare-cdn-cache-control', 'no-store');
  return out;
}
