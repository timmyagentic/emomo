import assert from 'node:assert/strict';
import { test } from 'node:test';
import { setImmediate } from 'node:timers/promises';
import { OpenAIScheduler, mapConcurrent, durationMs, retryDelay } from '../src/openai-scheduler.js';
import { OpenAIVision } from '../src/openai-vision.js';
import { PNG } from './helpers.js';

function clock() {
  let current = 0;
  const waits = [];
  return { now: () => current, sleep: async ms => { waits.push(ms); current += ms; }, waits };
}
test('admission uses token/image/request pacing and the actual concurrency cap', async () => {
  const time = clock();
  const limiter = new OpenAIScheduler({ concurrency: 2, tokensPerMinute: 60000, requestsPerMinute: 60000, imagesPerMinute: 60000, utilization: 1 }, time);
  let active = 0, maximum = 0;
  const starts = [];
  const releases = [];
  const tasks = Array.from({ length: 4 }, () => limiter.run({ tokens: 100, images: 1 }, async () => {
    active++; maximum = Math.max(maximum, active); starts.push(time.now());
    await new Promise(resolve => releases.push(resolve)); active--;
  }));
  await setImmediate(); assert.equal(active, 2); assert.equal(starts.length, 2);
  releases.splice(0).forEach(resolve => resolve());
  await setImmediate(); assert.equal(active, 2);
  releases.splice(0).forEach(resolve => resolve());
  await Promise.all(tasks);
  assert.equal(maximum, 2);
  assert.deepEqual(starts, [0, 100, 200, 300]);
});
test('server token capacity, Retry-After and reduced concurrency constrain later requests', async () => {
  const time = clock();
  const limiter = new OpenAIScheduler({ concurrency: 8 }, time);
  limiter.observe(new Headers({ 'x-ratelimit-limit-tokens': '1000', 'x-ratelimit-limit-project-tokens': '500', 'x-ratelimit-remaining-project-tokens': '0', 'x-ratelimit-reset-project-tokens': '2s' }), { tokens: 10, images: 1 });
  assert.equal(limiter.tokensPerMinute, 500);
  limiter.backoff(new Headers({ 'retry-after': '3' }), 0);
  assert.equal(limiter.parallelism, 4);
  await limiter.run({ tokens: 10, images: 1 }, () => assert.ok(time.now() >= 3000));
  assert.equal(durationMs('1m2.5s'), 62500);
  assert.equal(retryDelay(new Headers({ 'retry-after': '20' }), 0, 0), 20000);
});
test('transient model limits retry with pacing while quota exhaustion stops immediately', async () => {
  const time = clock();
  const limiter = new OpenAIScheduler({ maxRetries: 2 }, time);
  let calls = 0;
  const client = new OpenAIVision({ apiKey: 'sk-test-placeholder-for-local-tests-only' }, async () => {
    calls++;
    if (calls === 1) return Response.json({ error: { code: 'rate_limit_exceeded' } }, { status: 429, headers: { 'retry-after': '2' } });
    return Response.json({ status: 'completed', model: 'gpt-6.1-sol', output: [{ type: 'message', content: [{ type: 'output_text', text: JSON.stringify({ duplicate: true, needs_review: false, reason: 'Same image' }) }] }] });
  }, limiter);
  assert.equal((await client.compare(PNG, PNG)).value.duplicate, true);
  assert.equal(calls, 2); assert.ok(time.waits.some(ms => ms >= 2000));
  const exhausted = new OpenAIVision({ apiKey: 'sk-test-placeholder-for-local-tests-only' }, async () => {
    calls++; return Response.json({ error: { code: 'insufficient_quota' } }, { status: 429 });
  }, new OpenAIScheduler({}, clock()));
  await assert.rejects(exhausted.compare(PNG, PNG), { code: 'OPENAI_REQUEST_FAILED', retryable: false });
  assert.equal(calls, 3);
});
test('parallel work drains in-flight checkpoints before reporting a failure', async () => {
  const completed = [];
  await assert.rejects(mapConcurrent([0,1,2,3], 2, async value => {
    if (value === 0) { await setImmediate(); throw new Error('failed'); }
    await setImmediate(); completed.push(value);
  }));
  assert.deepEqual(completed, [1]);
});

test('synchronous output reservations expand once only when the response is actually truncated', async () => {
  const caps = [];
  const client = new OpenAIVision({ apiKey: 'sk-test-placeholder-for-local-tests-only' }, async (url, options) => {
    caps.push(JSON.parse(options.body).max_output_tokens);
    if (caps.length === 1) return Response.json({ id: 'partial', status: 'incomplete', incomplete_details: { reason: 'max_output_tokens' }, usage: { total_tokens: 4096 } });
    return Response.json({ id: 'complete', status: 'completed', model: 'gpt-6.1-sol', output: [{ type: 'message', content: [{ type: 'output_text', text: JSON.stringify({ duplicate: true, reason: 'Same image', needs_review: false }) }] }] });
  }, new OpenAIScheduler({}, clock()));
  const result = await client.compare(PNG, PNG);
  assert.deepEqual(caps, [4096, 16384]);
  assert.equal(result.receipt.retryUsage[0].totalTokens, 4096);
});
