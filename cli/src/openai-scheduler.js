import { setTimeout as delay } from 'node:timers/promises';
import { EmomoError } from './error.js';

export const OPENAI_LIMITS = Object.freeze({
  tokensPerMinute: 4_000_000,
  requestsPerMinute: 10_000,
  imagesPerMinute: 50_000,
  batchQueueTokens: 200_000_000,
  concurrency: 16,
  imageBytesInFlight: 64 * 1024 * 1024,
  utilization: 0.8,
  maxRetries: 3
});

export function durationMs(value) {
  if (!value) return 0;
  const parts = [...value.matchAll(/([\d.]+)(ms|s|m|h)/g)];
  if (!parts.length) return 0;
  return parts.reduce((sum, part) => sum + Number(part[1]) * { ms: 1, s: 1000, m: 60000, h: 3600000 }[part[2]], 0);
}
export function retryDelay(headers, attempt, now = Date.now()) {
  const retry = headers.get('retry-after');
  const minimum = retry && /^\d+(\.\d+)?$/.test(retry) ? Number(retry) * 1000 : Math.max(0, Date.parse(retry ?? '') - now) || 0;
  return Math.max(minimum, durationMs(headers.get('x-ratelimit-reset-tokens')), 1000 * 2 ** attempt);
}

export class OpenAIScheduler {
  constructor(options = {}, { now = Date.now, sleep = delay } = {}) {
    this.limits = Object.fromEntries(Object.entries(OPENAI_LIMITS).map(([key, value]) => [key, options[key] === undefined ? value : options[key]]));
    for (const field of ['tokensPerMinute', 'requestsPerMinute', 'imagesPerMinute', 'batchQueueTokens', 'concurrency', 'imageBytesInFlight']) {
      if (!Number.isSafeInteger(this.limits[field]) || this.limits[field] < 1) throw new EmomoError('INVALID_OPENAI_CONFIG', 'OpenAI rate limits must be positive integers.');
    }
    if (this.limits.concurrency > 256 || !(this.limits.utilization > 0 && this.limits.utilization <= 1) || !Number.isInteger(this.limits.maxRetries) || this.limits.maxRetries < 0 || this.limits.maxRetries > 5) throw new EmomoError('INVALID_OPENAI_CONFIG', 'Use concurrency 1–256, utilization (0,1], and retries 0–5.');
    this.now = now;
    this.sleep = sleep;
    this.active = 0;
    this.parallelism = this.limits.concurrency;
    this.waiters = [];
    this.nextStart = 0;
    this.blockedUntil = 0;
    this.admissions = Promise.resolve();
    this.tokensPerMinute = this.limits.tokensPerMinute;
    this.requestsPerMinute = this.limits.requestsPerMinute;
    this.successes = 0;
  }
  async run(cost, action) {
    const admit = this.admissions.then(async () => {
      for (;;) {
        if (this.active >= this.parallelism) {
          await new Promise(resolve => this.waiters.push(resolve));
          continue;
        }
        const wait = Math.max(this.nextStart, this.blockedUntil) - this.now();
        if (wait > 0) { await this.sleep(wait); continue; }
        break;
      }
      const interval = Math.max(60000 / this.requestsPerMinute, 60000 * cost.tokens / this.tokensPerMinute, 60000 * cost.images / this.limits.imagesPerMinute) / this.limits.utilization;
      this.nextStart = this.now() + interval;
      this.active++;
    });
    this.admissions = admit.catch(() => {});
    await admit;
    try { return await action(); }
    finally {
      this.active--;
      for (const resume of this.waiters.splice(0)) resume();
    }
  }

  observe(headers, cost) {
    const lower = (current, name) => {
      const value = Number(headers.get(name));
      return value > 0 ? Math.min(current, value) : current;
    };
    this.tokensPerMinute = lower(lower(this.tokensPerMinute, 'x-ratelimit-limit-tokens'), 'x-ratelimit-limit-project-tokens');
    this.requestsPerMinute = lower(this.requestsPerMinute, 'x-ratelimit-limit-requests');
    for (const [remaining, reset, required] of [
      ['x-ratelimit-remaining-tokens', 'x-ratelimit-reset-tokens', cost.tokens],
      ['x-ratelimit-remaining-project-tokens', 'x-ratelimit-reset-project-tokens', cost.tokens],
      ['x-ratelimit-remaining-requests', 'x-ratelimit-reset-requests', 1]
    ]) {
      if (headers.has(remaining) && Number(headers.get(remaining)) < required) this.blockedUntil = Math.max(this.blockedUntil, this.now() + durationMs(headers.get(reset)));
    }
  }
  backoff(headers, attempt) {
    this.parallelism = Math.max(1, Math.floor(this.parallelism / 2));
    this.successes = 0;
    this.blockedUntil = Math.max(this.blockedUntil, this.now() + retryDelay(headers, attempt, this.now()));
  }
  succeeded() {
    if (++this.successes >= 20) {
      this.parallelism = Math.min(this.limits.concurrency, this.parallelism + 1);
      this.successes = 0;
    }
  }
}

// Stop starting work after a failure, but wait for in-flight checkpoints before
// returning. This prevents background writes after the caller reports failure.
export async function mapConcurrent(items, concurrency, action) {
  let cursor = 0;
  let error;
  const results = new Array(items.length);
  async function worker() {
    while (!error && cursor < items.length) {
      const index = cursor++;
      try { results[index] = await action(items[index], index); }
      catch (caught) { error ??= caught; }
    }
  }
  await Promise.all(Array.from({ length: Math.min(concurrency, items.length) }, worker));
  if (error) throw error;
  return results;
}

// Reserve raw image bytes before the directory workflow reads them. Serialized
// data URLs and request JSON add overhead, so the default raw budget is 64 MiB.
export class ImageMemoryBudget {
  constructor(maximum) { this.maximum = maximum; this.used = 0; this.waiters = []; this.admissions = Promise.resolve(); }
  async run(bytes, action) {
    if (!Number.isSafeInteger(bytes) || bytes < 0 || bytes > this.maximum) throw new EmomoError('IMAGE_MEMORY_LIMIT', 'This image operation exceeds the configured in-flight image budget.');
    const admit = this.admissions.then(async () => {
      while (this.used + bytes > this.maximum) await new Promise(resolve => this.waiters.push(resolve));
      this.used += bytes;
    });
    this.admissions = admit.catch(() => {});
    await admit;
    try { return await action(); }
    finally { this.used -= bytes; for (const resume of this.waiters.splice(0)) resume(); }
  }
}
