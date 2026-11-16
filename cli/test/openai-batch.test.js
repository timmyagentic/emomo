import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtemp, mkdir, writeFile, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { OpenAIVision } from '../src/openai-vision.js';
import { ingestDirectory } from '../src/directory-import.js';
import { batchStatus, retryAnnotationBatch } from '../src/openai-batch.js';
import { withImportLock } from '../src/import-lock.js';
import { PNG } from './helpers.js';
const SECOND = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAIAAAD91JpzAAAAEElEQVR4nGP8zwACTGCSAQANHQEDgslx/wAAAABJRU5ErkJggg==', 'base64');
const annotation = { decision: 'keep', description: '猫咪开心', ocr_text: '', text_presence: 3,
  tags: ['猫', '开心'], search_aliases: [], category: 'usable', preview_only: false, queries: ['猫 开心'], reason: '', needs_review: false };
function provider() {
  const files = new Map(), jobs = new Map();
  const state = { files, jobs, creates: 0, ready: false, failOne: false, loseCreateResponse: false };
  const vision = new OpenAIVision({ apiKey: 'sk-test-placeholder-for-local-tests-only' }, async (url, options) => {
    const path = new URL(url).pathname;
    if (path === '/v1/files' && options.method === 'POST') {
      const id = `file-${files.size}`;
      files.set(id, (await options.body.get('file').text()).trim().split('\n').map(JSON.parse));
      for (const request of files.get(id)) {
        assert.equal(request.body.model, 'gpt-6.1-sol');
        assert.equal(request.body.reasoning.effort, 'low');
      }
      return Response.json({ id });
    }
    if (path === '/v1/batches' && options.method === 'POST') {
      state.creates++;
      const input = JSON.parse(options.body);
      const job = { id: `batch_${state.creates}`, ...input, status: 'in_progress' };
      jobs.set(job.id, job);
      if (state.loseCreateResponse) { state.loseCreateResponse = false; throw new Error('connection lost after creation'); }
      return Response.json(job);
    }
    if (path === '/v1/batches') return Response.json({ data: [...jobs.values()], has_more: false });
    if (path.startsWith('/v1/batches/')) {
      const job = jobs.get(path.split('/').at(-1));
      return Response.json({ ...job, status: state.ready ? 'completed' : 'in_progress', output_file_id: state.ready ? `file-output-${job.id}` : null });
    }
    if (path.includes('/content')) {
      const id = path.split('/')[3].replace('file-output-', '');
      const job = jobs.get(id), requests = files.get(job.input_file_id);
      const output = requests.map((request, index) => ({ custom_id: request.custom_id,
        ...(state.failOne && index === 1 ? { error: { code: 'batch_expired' }, response: null } : {
          response: { status_code: 200, body: { status: 'completed', id: `resp-${request.custom_id}`, model: 'gpt-6.1-sol',
            output: [{ type: 'message', content: [{ type: 'output_text', text: JSON.stringify(annotation) }] }] } }, error: null }) })).reverse();
      return new Response(output.map(JSON.stringify).join('\n')+'\n');
    }
    assert.fail(`Unexpected API request ${path}`);
  });
  vision.compare = async () => ({ value: { duplicate: false, reason: 'Different fixtures', needs_review: false }, receipt: { model: vision.model } });
  return { vision, state };
}
async function inputs(t) {
  const root = await mkdtemp(join(tmpdir(), 'emomo-batch-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const source = join(root, 'images'), batch = join(root, 'batch');
  await mkdir(source); await writeFile(join(source, 'a.png'), PNG); await writeFile(join(source, 'b.png'), SECOND);
  return { source, batch };
}

test('Batch submits once, resumes unordered results by image ID and enters normal validation', async t => {
  const { source, batch } = await inputs(t), { vision, state } = provider();
  const queued = await ingestDirectory(source, batch, null, vision, null, { mode: 'batch' });
  assert.equal(queued.status, 'batch_pending'); assert.equal(queued.total, 2); assert.equal(state.creates, 1);
  const waiting = await ingestDirectory(source, batch, null, vision);
  assert.equal(waiting.status, 'batch_pending'); assert.equal(state.creates, 1);
  const before = JSON.parse(await readFile(join(batch, 'annotations.json'), 'utf8'));
  assert.ok(before.every(row => row.decision === 'pending'));
  state.ready = true;
  const ready = await ingestDirectory(source, batch, null, vision);
  assert.equal(ready.status, 'ready'); assert.equal(ready.newImages, 2); assert.ok(ready.checks.every(check => check.passed));
  const after = JSON.parse(await readFile(join(batch, 'annotations.json'), 'utf8'));
  assert.ok(after.every(row => row.llm.annotation.customId === `annotation_${row.sha256}`));
  assert.ok(after.every(row => row.llm.annotation.requestedModel === 'gpt-6.1-sol' && row.llm.annotation.reasoningEffort === 'low'));
  assert.equal((await ingestDirectory(source, batch, null, vision)).status, 'ready');
  assert.equal(state.creates, 1);
});
test('partial Batch failure retains successes and explicit retry submits only missing items', async t => {
  const { source, batch } = await inputs(t), { vision, state } = provider();
  await ingestDirectory(source, batch, null, vision, null, { mode: 'batch' });
  state.ready = true; state.failOne = true;
  const failed = await ingestDirectory(source, batch, null, vision);
  assert.equal(failed.status, 'batch_failed'); assert.equal(failed.completed, 1); assert.equal(failed.failed.length, 1);
  await retryAnnotationBatch(batch, vision);
  state.ready = false; state.failOne = false;
  assert.equal((await ingestDirectory(source, batch, null, vision, null, { mode: 'batch' })).total, 1);
  assert.equal([...state.files.values()].at(-1).length, 1);
  state.ready = true;
  assert.equal((await ingestDirectory(source, batch, null, vision)).status, 'ready');
});
test('lost creation responses are recovered through the persisted marker without duplicate jobs', async t => {
  const { source, batch } = await inputs(t), { vision, state } = provider();
  state.loseCreateResponse = true;
  await assert.rejects(ingestDirectory(source, batch, null, vision, null, { mode: 'batch' }), { code: 'OPENAI_NETWORK_ERROR' });
  assert.equal(state.creates, 1);
  assert.equal((await ingestDirectory(source, batch, null, vision)).status, 'batch_pending');
  assert.equal(state.creates, 1);
  state.ready = true;
  assert.equal((await ingestDirectory(source, batch, null, vision)).status, 'ready');
});
test('edited annotations survive Batch completion and a same-directory process lock excludes concurrent commands', async t => {
  const { source, batch } = await inputs(t), { vision, state } = provider();
  await ingestDirectory(source, batch, null, vision, null, { mode: 'batch' });
  const path = join(batch, 'annotations.json');
  const rows = JSON.parse(await readFile(path, 'utf8'));
  rows[0].reason = 'User edited this row';
  await writeFile(path, JSON.stringify(rows));
  state.ready = true;
  const failed = await ingestDirectory(source, batch, null, vision);
  assert.equal(failed.status, 'batch_failed');
  assert.equal(JSON.parse(await readFile(path, 'utf8'))[0].reason, rows[0].reason);
  await withImportLock(batch, async () => {
    await assert.rejects(batchStatus(batch, vision), { code: 'IMPORT_BUSY' });
  });
  assert.equal((await batchStatus(batch, vision)).status, 'batch_failed');
});
test('the estimated queue budget defers later shards until the prior job completes', async t => {
  const { source, batch } = await inputs(t), { vision, state } = provider();
  const size = vision.prepareAnnotation(PNG).inputTokens;
  vision.scheduler.limits.batchQueueTokens = Math.ceil(size * 1.5);
  const initial = await ingestDirectory(source, batch, null, vision, null, { mode: 'batch' });
  assert.equal(initial.batches.length, 2); assert.equal(state.creates, 1);
  state.ready = true;
  assert.equal((await ingestDirectory(source, batch, null, vision)).status, 'batch_pending');
  assert.equal(state.creates, 2);
  assert.equal((await ingestDirectory(source, batch, null, vision)).status, 'ready');
});

test('definitively rejected submissions can rebuild pending inputs without retrying successful jobs', async t => {
  const { source, batch } = await inputs(t), { vision, state } = provider();
  const original = vision.apiJSON.bind(vision);
  let reject = true;
  vision.apiJSON = async (path, options) => {
    if (path === '/batches' && reject) {
      const error = new Error('Rejected'); error.httpStatus = 400; throw error;
    }
    return original(path, options);
  };
  await assert.rejects(ingestDirectory(source, batch, null, vision, null, { mode: 'batch' }));
  assert.equal((await batchStatus(batch, vision)).status, 'batch_failed');
  await retryAnnotationBatch(batch, vision);
  reject = false;
  const retry = await ingestDirectory(source, batch, null, vision, null, { mode: 'batch' });
  assert.equal(retry.status, 'batch_pending'); assert.equal(state.creates, 1);
});

test('Batch input paths cannot escape the batch directory', async t => {
  const { source, batch } = await inputs(t), { vision } = provider();
  await ingestDirectory(source, batch, null, vision, null, { mode: 'batch' });
  const path = join(batch, 'openai-batch.json');
  const state = JSON.parse(await readFile(path, 'utf8'));
  state.parts[0].file = '../private-file.json';
  await writeFile(path, JSON.stringify(state));
  await assert.rejects(batchStatus(batch, vision), { code: 'INVALID_OPENAI_BATCH' });
});

test('an existing v1 xhigh Batch keeps its original prompt and effort when resumed', async t => {
  const { source, batch } = await inputs(t), { vision, state } = provider();
  const prepare = vision.prepareAnnotation.bind(vision);
  vision.reasoningEffort = 'xhigh';
  vision.prepareAnnotation = bytes => prepare(bytes, null, { effort: 'xhigh', promptVersion: 1 });
  // The provider fixture assertion is specific to new low submissions.
  const originalTransport = vision.transport;
  vision.transport = async (url, options) => {
    if (new URL(url).pathname === '/v1/files' && options.method === 'POST') {
      const file = options.body.get('file');
      const lines = (await file.text()).trim().split('\n').map(JSON.parse);
      for (const line of lines) assert.equal(line.body.reasoning.effort, 'xhigh');
      const id = `file-${state.files.size}`; state.files.set(id, lines);
      return Response.json({ id });
    }
    return originalTransport(url, options);
  };
  await ingestDirectory(source, batch, null, vision, null, { mode: 'batch' });
  const path = join(batch, 'openai-batch.json');
  const saved = JSON.parse(await readFile(path, 'utf8')); saved.promptVersion = 1;
  await writeFile(path, JSON.stringify(saved));
  vision.reasoningEffort = 'low'; vision.prepareAnnotation = prepare; state.ready = true;
  const result = await ingestDirectory(source, batch, null, vision);
  assert.equal(result.status, 'ready');
  const rows = JSON.parse(await readFile(join(batch, 'annotations.json'), 'utf8'));
  assert.ok(rows.every(row => row.llm.annotation.reasoningEffort === 'xhigh' && row.llm.annotation.promptVersion === 1));
});
