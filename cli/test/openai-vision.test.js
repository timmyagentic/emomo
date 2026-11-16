import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtemp, mkdir, writeFile, readFile, rm, chmod } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { OpenAIVision, configuredVision } from '../src/openai-vision.js';
import { ingestDirectory } from '../src/directory-import.js';
import { publicError } from '../src/error.js';
import { PNG } from './helpers.js';
const key = 'sk-test-placeholder-for-local-tests-only';
const annotation = { decision: 'keep', description: '猫咪开心', ocr_text: '', text_presence: 3,
  tags: ['猫', '开心'], search_aliases: ['高兴'], category: 'usable', preview_only: false,
  queries: ['猫 开心'], reason: '', needs_review: false };
const receipt = { model: 'gpt-6.1-sol', reasoningEffort: 'xhigh', promptVersion: 1 };
const response = (value, model = 'gpt-6.1-sol') => Response.json({ id: 'response-test', status: 'completed', model, service_tier: 'default',
  output: [{ type: 'message', content: [{ type: 'output_text', text: JSON.stringify(value) }] }],
  usage: { input_tokens: 10, output_tokens: 20, total_tokens: 30,
    input_tokens_details: { cached_tokens: 4 }, output_tokens_details: { reasoning_tokens: 8 } } });

test('annotations use Sol low and repair xhigh while duplicate review uses Luna/medium with accurate receipts', async () => {
  const requests = [];
  const client = new OpenAIVision({ apiKey: key }, async (url, options) => {
    assert.equal(url, 'https://api.openai.com/v1/responses');
    assert.equal(options.redirect, 'manual');
    const body = JSON.parse(options.body); requests.push(body);
    const comparison = body.text.format.name === 'meme_duplicate_review';
    assert.equal(body.model, comparison ? 'gpt-6-luna' : 'gpt-6.1-sol');
    assert.equal(body.reasoning.effort, comparison ? 'medium' : body.text.format.name === 'meme_annotation_repair' ? 'xhigh' : 'low');
    assert.equal(body.store, false); assert.equal(body.text.format.strict, true);
    assert.ok(!options.body.includes(key));
    assert.ok(body.input[0].content.some(part => part.type === 'input_image' && part.image_url.startsWith('data:image/png;base64,')));
    return response(comparison ? { duplicate: true, reason: 'Same picture and wording', needs_review: false } : annotation, body.model);
  });
  const first = await client.annotate(PNG);
  assert.equal(first.value.description, annotation.description);
  await client.annotate(PNG, { previous: annotation, failedQueries: ['wrong query'] });
  const reviewed = await client.compare(PNG, PNG);
  assert.equal(reviewed.value.duplicate, true);
  assert.equal(reviewed.receipt.model, 'gpt-6-luna');
  assert.equal(reviewed.receipt.requestedModel, 'gpt-6-luna');
  assert.equal(reviewed.receipt.reasoningEffort, 'medium');
  assert.deepEqual(requests.map(body => body.text.format.name), ['meme_annotation', 'meme_annotation_repair', 'meme_duplicate_review']);
  assert.ok(!JSON.stringify(client).includes(key));
  assert.equal(first.receipt.imageHashes.length, 1);
  assert.equal(first.receipt.totalTokens, 30);
  assert.equal(first.receipt.requestedModel, 'gpt-6.1-sol');
  assert.equal(first.receipt.reasoningEffort, 'low');
  assert.equal(first.receipt.cachedInputTokens, 4);
  assert.equal(first.receipt.reasoningTokens, 8);
  assert.equal(first.receipt.serviceTier, 'default');
  assert.equal(client.prepareAnnotation(PNG).payload.model, 'gpt-6.1-sol');
});

test('API failures, redirects, malformed metadata and incomplete output cannot become annotations or leak keys', async () => {
  for (const status of [302, 401, 429, 503]) {
    const client = new OpenAIVision({ apiKey: key, limits: { maxRetries: 0 } }, async () => new Response(JSON.stringify({ error: { message: key } }), { status }));
    try { await client.annotate(PNG); assert.fail(); }
    catch (error) { assert.equal(error.code, 'OPENAI_REQUEST_FAILED'); assert.ok(!JSON.stringify(publicError(error)).includes(key)); }
  }
  const invalid = new OpenAIVision({ apiKey: key }, async () => response({ ...annotation, tags: [] }));
  await assert.rejects(invalid.annotate(PNG), { code: 'OPENAI_RESPONSE_INVALID' });
  const incomplete = new OpenAIVision({ apiKey: key }, async () => Response.json({ status: 'incomplete', output: [] }));
  await assert.rejects(incomplete.annotate(PNG), { code: 'OPENAI_RESPONSE_INVALID' });
});

test('private OpenAI config is optional and rejects readable credentials', async t => {
  const dir = await mkdtemp(join(tmpdir(), 'emomo-openai-config-'));
  t.after(() => rm(dir, { recursive: true, force: true }));
  const env = { EMOMO_CONFIG_DIR: dir };
  assert.equal(await configuredVision(env), null);
  const file = join(dir, 'openai.json');
  await writeFile(file, JSON.stringify({ apiKey: key, model: 'gpt-6.1-sol', reasoningEffort: 'xhigh' }), { mode: 0o600 });
  assert.equal((await configuredVision(env)).model, 'gpt-6.1-sol');
  if (process.platform !== 'win32') {
    await chmod(file, 0o644);
    await assert.rejects(configuredVision(env), { code: 'INVALID_OPENAI_CONFIG' });
  }
});

test('normal directory preparation annotates, repairs failed queries once and reuses saved results', async t => {
  const dir = await mkdtemp(join(tmpdir(), 'emomo-openai-ingest-'));
  t.after(() => rm(dir, { recursive: true, force: true }));
  const source = join(dir, 'images'), batch = join(dir, 'batch');
  await mkdir(source); await writeFile(join(source, 'cat.png'), PNG);
  let calls = 0;
  const vision = {
    annotate: async (bytes, feedback) => {
      calls++; assert.deepEqual(bytes, PNG);
      return { value: { ...annotation, queries: feedback ? ['猫 开心'] : ['zzzxqmissing'] }, receipt };
    }, compare: async () => assert.fail('Only one image')
  };
  const ready = await ingestDirectory(source, batch, null, vision);
  assert.equal(ready.status, 'ready'); assert.equal(calls, 2); assert.equal(ready.modelReviewedImages, 1);
  assert.equal((await ingestDirectory(source, batch, null, vision)).confirmation, ready.confirmation);
  assert.equal(calls, 2);
  const stored = JSON.parse(await readFile(join(batch, 'annotations.json'), 'utf8'));
  assert.equal(stored[0].llm.repair.model, 'gpt-6.1-sol');
});

test('uncertain model review stays pending across retries and errors keep the pending row intact', async t => {
  const dir = await mkdtemp(join(tmpdir(), 'emomo-openai-review-'));
  t.after(() => rm(dir, { recursive: true, force: true }));
  const source = join(dir, 'images'), batch = join(dir, 'batch');
  await mkdir(source); await writeFile(join(source, 'cat.png'), PNG);
  let calls = 0;
  const vision = { annotate: async () => { calls++; return { value: { ...annotation, needs_review: true, reason: 'Text unclear' }, receipt }; } };
  assert.equal((await ingestDirectory(source, batch, null, vision)).status, 'needs_review');
  assert.equal((await ingestDirectory(source, batch, null, vision)).status, 'needs_review');
  assert.equal(calls, 1);
  const failBatch = join(dir, 'failed');
  await assert.rejects(ingestDirectory(source, failBatch, null, { annotate: async () => { throw new Error('unavailable'); } }));
  assert.equal(JSON.parse(await readFile(join(failBatch, 'annotations.json'), 'utf8'))[0].decision, 'pending');
});

test('similar candidates use the shared model and preserve canonical references across resume', async t => {
  const dir = await mkdtemp(join(tmpdir(), 'emomo-openai-duplicates-'));
  t.after(() => rm(dir, { recursive: true, force: true }));
  const source = join(dir, 'images'), batch = join(dir, 'batch');
  await mkdir(source);
  const second = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAIAAAD91JpzAAAAEElEQVR4nGP8zwACTGCSAQANHQEDgslx/wAAAABJRU5ErkJggg==', 'base64');
  await writeFile(join(source, 'a.png'), PNG); await writeFile(join(source, 'b.png'), second);
  let annotations = 0, comparisons = 0;
  const vision = {
    annotate: async () => { annotations++; return { value: annotation, receipt }; },
    compare: async (first, other) => { comparisons++; assert.notDeepEqual(first, other); return { value: { duplicate: true, needs_review: false, reason: 'Same content fixture decision' }, receipt }; }
  };
  const ready = await ingestDirectory(source, batch, null, vision);
  assert.equal(ready.status, 'ready'); assert.equal(ready.newImages, 1); assert.equal(ready.similarDuplicates, 1);
  const rows = JSON.parse(await readFile(join(batch, 'annotations.json'), 'utf8'));
  assert.equal(rows[1].duplicate_of, rows[0].sha256);
  await ingestDirectory(source, batch, null, vision);
  assert.equal(annotations, 2); assert.equal(comparisons, 1);
});

test('repairing away a canonical target reopens and rechecks its duplicate images', async t => {
  const dir = await mkdtemp(join(tmpdir(), 'emomo-openai-reconcile-'));
  t.after(() => rm(dir, { recursive: true, force: true }));
  const source = join(dir, 'images'), batch = join(dir, 'batch');
  await mkdir(source);
  const second = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAIAAAD91JpzAAAAEElEQVR4nGP8zwACTGCSAQANHQEDgslx/wAAAABJRU5ErkJggg==', 'base64');
  await writeFile(join(source, 'a.png'), PNG); await writeFile(join(source, 'b.png'), second);
  let calls = 0;
  const vision = {
    annotate: async (bytes, feedback) => {
      calls++;
      if (bytes.equals(PNG)) return { value: feedback ? { ...annotation, decision: 'exclude', reason: 'Canonical image is unusable' } : { ...annotation, queries: ['zzzxqmissing'] }, receipt };
      return { value: annotation, receipt };
    },
    compare: async () => ({ value: { duplicate: true, needs_review: false, reason: 'Duplicate fixture decision' }, receipt })
  };
  const ready = await ingestDirectory(source, batch, null, vision);
  assert.equal(ready.status, 'ready'); assert.equal(ready.newImages, 1); assert.equal(ready.excluded, 1);
  const rows = JSON.parse(await readFile(join(batch, 'annotations.json'), 'utf8'));
  assert.equal(rows[0].decision, 'exclude'); assert.equal(rows[1].decision, 'keep'); assert.equal(rows[1].duplicate_of, '');
  assert.ok(rows[1].llm.previous.reopenedFrom);
  assert.equal(calls, 4);
  await ingestDirectory(source, batch, null, vision);
  assert.equal(calls, 4);
});

test('local preparation stays usable with vision and read access but no maintenance access', async t => {
  const dir = await mkdtemp(join(tmpdir(), 'emomo-openai-local-only-'));
  t.after(() => rm(dir, { recursive: true, force: true }));
  const source = join(dir, 'images'), batch = join(dir, 'batch');
  await mkdir(source); await writeFile(join(source, 'cat.png'), PNG);
  let calls = 0;
  const vision = { annotate: async () => { calls++; return { value: annotation, receipt }; } };
  const reader = { search: async () => assert.fail('External duplicate references require maintenance verification access') };
  const ready = await ingestDirectory(source, batch, null, vision, reader);
  assert.equal(ready.status, 'ready'); assert.equal(ready.newImages, 1);
  assert.equal(ready.onlineSimilarCandidatesChecked, false);
  assert.equal((await ingestDirectory(source, batch, null, vision, reader)).status, 'ready');
  assert.equal(calls, 1);
});

test('first-round annotations run concurrently and checkpoint every result before comparison', async t => {
  const dir = await mkdtemp(join(tmpdir(), 'emomo-openai-parallel-'));
  t.after(() => rm(dir, { recursive: true, force: true }));
  const source = join(dir, 'images'), batch = join(dir, 'batch');
  await mkdir(source);
  const second = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAIAAAD91JpzAAAAEElEQVR4nGP8zwACTGCSAQANHQEDgslx/wAAAABJRU5ErkJggg==', 'base64');
  await writeFile(join(source, 'a.png'), PNG); await writeFile(join(source, 'b.png'), second);
  let active = 0, max = 0, announce;
  const started = new Promise(resolve => { announce = resolve; });
  const releases = [];
  const vision = { concurrency: 2,
    annotate: async bytes => {
      active++; max = Math.max(max, active);
      if (active === 2) announce();
      await new Promise(resolve => releases.push(resolve));
      active--;
      const word = bytes.equals(PNG) ? '猫' : '狗';
      return { value: { ...annotation, description: word, tags: [word], queries: [word] }, receipt };
    }, compare: async () => assert.fail('These fixtures have different subjects') };
  const preparation = ingestDirectory(source, batch, null, vision);
  await started;
  releases.reverse().forEach(resolve => resolve());
  const ready = await preparation;
  assert.equal(max, 2); assert.equal(ready.newImages, 2);
  const rows = JSON.parse(await readFile(join(batch, 'annotations.json'), 'utf8'));
  assert.equal(rows.length, 2); assert.ok(rows.every(row => row.decision === 'keep'));
});

test('parallel duplicate decisions verify the final canonical image instead of assuming transitivity', async t => {
  const dir = await mkdtemp(join(tmpdir(), 'emomo-openai-canonical-'));
  t.after(() => rm(dir, { recursive: true, force: true }));
  const source = join(dir, 'images'), batch = join(dir, 'batch');
  await mkdir(source);
  const second = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAIAAAD91JpzAAAAEElEQVR4nGP8zwACTGCSAQANHQEDgslx/wAAAABJRU5ErkJggg==', 'base64');
  const third = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAIAAAD91JpzAAAAEklEQVR4nGNkYPjPwMDAxAAGAAsfAQMU4wsAAAAAAElFTkSuQmCC', 'base64');
  await writeFile(join(source, 'a.png'), PNG); await writeFile(join(source, 'b.png'), second); await writeFile(join(source, 'c.png'), third);
  const vision = { concurrency: 3,
    annotate: async bytes => ({ value: { ...annotation,
      tags: bytes.equals(PNG) ? ['猫','开心'] : bytes.equals(second) ? ['猫','开心','坐着','微笑'] : ['坐着','微笑'] }, receipt }),
    compare: async (first, other) => ({ value: { duplicate: !(first.equals(third) && other.equals(PNG)), needs_review: false, reason: 'Direct visual fixture decision' }, receipt })
  };
  const result = await ingestDirectory(source, batch, null, vision);
  assert.equal(result.status, 'needs_review');
  const rows = JSON.parse(await readFile(join(batch, 'annotations.json'), 'utf8'));
  assert.equal(rows[1].duplicate_of, rows[0].sha256);
  assert.equal(rows[2].decision, 'pending'); assert.equal(rows[2].duplicate_of, '');
});

test('positive duplicate verdicts with changed framing remain pending while resize rounding is allowed', async () => {
  const client = new OpenAIVision({ apiKey: key }, async () => response({ duplicate: true, needs_review: false, reason: 'Same picture' }, 'gpt-6-luna'));
  const square = await readFile(new URL('./fixtures/static-baseline.jpg', import.meta.url));
  const cropped = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAABAAAAAOCAIAAACpTQvdAAAAHElEQVR4nGP8z0AaYCJRPcOoBmIAE1GqGEa4BgDSowEbOGLWswAAAABJRU5ErkJggg==', 'base64');
  const guarded = await client.compare(square, cropped);
  assert.equal(guarded.value.duplicate, false);
  assert.equal(guarded.value.needs_review, true);
  assert.equal(guarded.reviewGuard.type, 'aspect_ratio');
  assert.equal(guarded.reviewGuard.modelValue.duplicate, true);
  assert.equal(guarded.receipt.requestedModel, 'gpt-6-luna');
  const first = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAABAAAAAPCAIAAABiEdh4AAAAH0lEQVR4nGP8z0AaYCJRPcOoBmIAE1GqkMCoBgYiAAAJPwEdPXWFKwAAAABJRU5ErkJggg==', 'base64');
  const resized = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAACAAAAAfCAIAAAAJNFjbAAAANUlEQVR4nO3QwQkAAAjDwOr+O+sIefkQcgO0kJrc6uP9eIBMhEyETIRMhEyETIRMhEyE/idaKuUBPcCOah0AAAAASUVORK5CYII=', 'base64');
  const accepted = await client.compare(first, resized);
  assert.equal(accepted.value.duplicate, true);
  assert.equal(accepted.value.needs_review, false);
});

test('low annotations escalate small or dense text, uncertainty and conflicting results once', async () => {
  const calls = [];
  const client = new OpenAIVision({ apiKey: key }, async (url, options) => {
    const body = JSON.parse(options.body); calls.push(body);
    return response({ ...annotation, review_reasons: [] });
  });
  const ordinary = { value: { ...annotation, review_reasons: [] }, receipt: { reasoningEffort: 'low' } };
  assert.equal((await client.upgradeAnnotation(PNG, ordinary)).receipt.reviewCompleted, true);
  assert.equal(calls.length, 0);
  for (const value of [
    { ...annotation, text_presence: 2, ocr_text: '文字' },
    { ...annotation, text_presence: 2, ocr_text: '字'.repeat(120) },
    { ...annotation, needs_review: true },
    { ...annotation, review_reasons: ['conflicting_result'] }
  ]) {
    const result = await client.upgradeAnnotation(PNG, { value, receipt: ordinary.receipt });
    assert.equal(result.receipt.reasoningEffort, 'xhigh');
    assert.equal(result.receipt.firstPass.reasoningEffort, 'low');
    assert.ok(result.receipt.reviewReasons.length);
    assert.equal((await client.upgradeAnnotation(PNG, result)), result);
  }
  assert.ok(calls.every(body => body.model === 'gpt-6.1-sol' && body.reasoning.effort === 'xhigh'));
});

test('directory resumes a saved low pass after a failed xhigh review without repeating low', async t => {
  const dir = await mkdtemp(join(tmpdir(), 'emomo-adaptive-'));
  t.after(() => rm(dir, { recursive: true, force: true }));
  const source = join(dir, 'source'), batch = join(dir, 'batch');
  await mkdir(source); await writeFile(join(source, 'sample.png'), PNG);
  const calls = []; let fail = true;
  const client = new OpenAIVision({ apiKey: key }, async (url, options) => {
    const body = JSON.parse(options.body); calls.push(body.reasoning.effort);
    if (body.reasoning.effort === 'xhigh' && fail) return new Response('', { status: 401 });
    return response({ ...annotation, review_reasons: body.reasoning.effort === 'low' ? ['conflicting_result'] : [] });
  });
  await assert.rejects(ingestDirectory(source, batch, null, client), { code: 'OPENAI_REQUEST_FAILED' });
  fail = false;
  assert.equal((await ingestDirectory(source, batch, null, client)).status, 'ready');
  assert.deepEqual(calls, ['low', 'xhigh', 'xhigh']);
  const saved = JSON.parse(await readFile(join(batch, 'annotations.json'), 'utf8'));
  assert.equal(saved[0].llm.annotation.firstPass.reasoningEffort, 'low');
});

test('description coverage applies to new prompts while saved v2 prompts retain their content', () => {
  const client = new OpenAIVision({ apiKey: key });
  const current = client.prepareAnnotation(PNG);
  const old = client.prepareAnnotation(PNG, null, { promptVersion: 2 });
  assert.equal(current.promptVersion, 3);
  assert.equal(old.promptVersion, 2);
  assert.equal(current.payload.reasoning.effort, 'low');
  assert.ok(current.payload.instructions.includes('meaningful changes between panels'));
  assert.ok(current.payload.instructions.includes('facial expression, key objects'));
  assert.ok(!old.payload.instructions.includes('meaningful changes between panels'));
  assert.deepEqual(current.payload.input, old.payload.input);
  assert.deepEqual(current.payload.text, old.payload.text);
});
