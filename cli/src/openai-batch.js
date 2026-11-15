import { openAsBlob } from 'node:fs';
import { open, readFile, writeFile, mkdir, rename, unlink } from 'node:fs/promises';
import { join } from 'node:path';
import { createHash, randomUUID } from 'node:crypto';
import { canonical } from './library.js';
import { EmomoError, publicError } from './error.js';
import { withImportLock } from './import-lock.js';
import { VISION_PROMPT_VERSION } from './openai-vision.js';

const MAX_FILE_BYTES = 190_000_000;
const MAX_REQUESTS = 50_000;
const TERMINAL = new Set(['completed', 'failed', 'expired', 'cancelled']);
const STATE_FILE = 'openai-batch.json';
const digest = value => createHash('sha256').update(canonical(value)).digest('hex');
const fail = message => new EmomoError('INVALID_OPENAI_BATCH', message);
async function load(directory) {
  try { return JSON.parse(await readFile(join(directory, STATE_FILE), 'utf8')); }
  catch (error) { if (error.code === 'ENOENT') return null; throw error; }
}
async function save(directory, state) {
  const path = join(directory, STATE_FILE), temporary = `${path}.${randomUUID()}`;
  try { await writeFile(temporary, JSON.stringify(state, null, 2) + '\n', { flag: 'wx', mode: 0o600 }); await rename(temporary, path); }
  finally { await unlink(temporary).catch(() => {}); }
}
function validateState(state, vision) {
  if (state?.schemaVersion !== 1 || state.model !== vision.model || state.reasoningEffort !== vision.reasoningEffort || state.promptVersion !== VISION_PROMPT_VERSION || !Array.isArray(state.parts)) throw fail('The stored Batch does not match the configured model and prompt version.');
}

async function prepare(directory, pending, vision, readImage) {
  const state = { schemaVersion: 1, model: vision.model, reasoningEffort: vision.reasoningEffort, promptVersion: VISION_PROMPT_VERSION, parts: [] };
  const queueBudget = Math.floor(vision.scheduler.limits.batchQueueTokens * vision.scheduler.limits.utilization);
  await mkdir(join(directory, 'batch-inputs'), { recursive: true, mode: 0o700 });
  let handle;
  let part;
  try {
    for (const row of pending) {
      const request = vision.prepareAnnotation(await readImage(row.sha256));
      const customId = `annotation_${row.sha256}`;
      const line = JSON.stringify({ custom_id: customId, method: 'POST', url: '/v1/responses', body: request.payload }) + '\n';
      const size = Buffer.byteLength(line);
      if (size > MAX_FILE_BYTES || request.inputTokens > queueBudget) throw fail('An image request exceeds the Batch file or configured queue budget.');
      if (!part || part.bytes + size > MAX_FILE_BYTES || part.jobs.length >= MAX_REQUESTS || part.inputTokens + request.inputTokens > queueBudget) {
        await handle?.close();
        const submission = randomUUID();
        part = { submission, file: `batch-inputs/${submission}.jsonl`, bytes: 0, inputTokens: 0, status: 'prepared', jobs: [] };
        state.parts.push(part);
        handle = await open(join(directory, part.file), 'wx', 0o600);
      }
      await handle.writeFile(line);
      part.bytes += size;
      part.inputTokens += request.inputTokens;
      part.jobs.push({ customId, sha256: row.sha256, annotationDigest: digest(row), operation: request.operation, hashes: request.hashes, imageInfo: request.imageInfo });
    }
  } finally { await handle?.close(); }
  await save(directory, state);
  return state;
}

async function recoverSubmission(vision, part) {
  let after;
  do {
    const page = await vision.apiJSON('/batches?limit=100' + (after ? `&after=${encodeURIComponent(after)}` : ''));
    if (!Array.isArray(page.data)) throw fail('The Batch list response is invalid.');
    const found = page.data.find(batch => batch.metadata?.emomo_submission === part.submission && batch.input_file_id === part.inputFileId);
    if (found) return found;
    if (page.has_more && page.last_id === after) throw fail('The Batch list cursor did not advance.');
    after = page.has_more ? page.last_id : null;
    if (page.has_more && !after) throw fail('The Batch list cursor is missing.');
  } while (after);
  return null;
}
async function submit(directory, state, vision) {
  const queueBudget = Math.floor(vision.scheduler.limits.batchQueueTokens * vision.scheduler.limits.utilization);
  let queued = state.parts.filter(part => part.status === 'submission_unknown' || part.batchId && !TERMINAL.has(part.status)).reduce((sum, part) => sum + part.inputTokens, 0);
  for (const part of state.parts) {
    if (part.batchId || part.status === 'submission_unknown' || part.collected) continue;
    if (part.inputTokens > queueBudget) throw fail('A saved shard exceeds the current queue budget. Collect active jobs, then use batch-retry to rebuild pending inputs.');
    if (queued + part.inputTokens > queueBudget) continue;
    if (!part.inputFileId) {
      const form = new FormData();
      form.set('purpose', 'batch');
      form.set('file', await openAsBlob(join(directory, part.file), { type: 'application/jsonl' }), 'images.jsonl');
      const file = await vision.apiJSON('/files', { method: 'POST', body: form });
      if (typeof file.id !== 'string') throw fail('OpenAI did not return an input file ID.');
      part.inputFileId = file.id;
      await save(directory, state);
    }
    // Persist uncertainty before POST. A lost response is recovered by the
    // unique metadata marker, never by blindly creating another charged job.
    part.status = 'submission_unknown';
    await save(directory, state);
    let created;
    try {
      created = await vision.apiJSON('/batches', { method: 'POST', body: {
        input_file_id: part.inputFileId, endpoint: '/v1/responses', completion_window: '24h',
        metadata: { emomo_submission: part.submission, estimated_input_tokens: String(part.inputTokens) }
      } });
    } catch (error) {
      if (error.httpStatus >= 400 && error.httpStatus < 500) {
        part.status = 'submission_rejected';
        part.collected = true;
        for (const job of part.jobs) job.error = publicError(error);
      }
      part.error = publicError(error);
      await save(directory, state);
      throw error;
    }
    if (typeof created.id !== 'string') throw fail('OpenAI did not return a Batch ID. Use batch-resume to recover its status.');
    Object.assign(part, { batchId: created.id, status: created.status });
    delete part.error;
    queued += part.inputTokens;
    await save(directory, state);
  }
}
async function refresh(directory, state, vision) {
  for (const part of state.parts) {
    let remote;
    if (part.status === 'submission_unknown') remote = await recoverSubmission(vision, part);
    else if (part.batchId && !part.collected) remote = await vision.apiJSON(`/batches/${encodeURIComponent(part.batchId)}`);
    if (!remote) continue;
    if (remote.input_file_id !== part.inputFileId || remote.metadata?.emomo_submission !== part.submission) throw fail('The remote Batch does not match its saved input file and submission marker.');
    Object.assign(part, { batchId: remote.id, status: remote.status, outputFileId: remote.output_file_id, errorFileId: remote.error_file_id, counts: remote.request_counts });
    delete part.error;
  }
  await save(directory, state);
}
async function* lines(vision, fileId) {
  const response = await vision.fetchAPI(`/files/${encodeURIComponent(fileId)}/content`);
  if (!response.ok) { await response.body?.cancel(); throw new EmomoError('OPENAI_REQUEST_FAILED', `Batch output download failed (HTTP ${response.status}).`); }
  const decoder = new TextDecoder('utf-8', { fatal: true });
  let buffered = '';
  for await (const chunk of response.body ?? []) {
    buffered += decoder.decode(chunk, { stream: true });
    let end;
    while ((end = buffered.indexOf('\n')) >= 0) {
      const line = buffered.slice(0, end); buffered = buffered.slice(end + 1);
      if (line.length > 2 * 1024 * 1024) throw fail('A Batch output line is too large.');
      if (line.trim()) yield JSON.parse(line);
    }
    if (buffered.length > 2 * 1024 * 1024) throw fail('A Batch output line is too large.');
  }
  buffered += decoder.decode();
  if (buffered.trim()) yield JSON.parse(buffered);
}
async function collect(directory, state, annotations, vision, apply, checkpoint) {
  const byHash = new Map(annotations.map(row => [row.sha256, row]));
  for (const part of state.parts) {
    if (!TERMINAL.has(part.status) || part.collected) continue;
    const jobs = new Map(part.jobs.map(job => [job.customId, job]));
    for (const fileId of [part.outputFileId, part.errorFileId].filter(Boolean)) {
      for await (const output of lines(vision, fileId)) {
        const job = jobs.get(output.custom_id);
        if (!job) throw fail('Batch output contains an unknown custom_id.');
        const row = byHash.get(job.sha256);
        if (!row) throw fail('A Batch result no longer has a matching local image.');
        if (job.applied || row.llm?.annotation?.batchId === part.batchId && row.llm.annotation.customId === job.customId) { job.applied = true; continue; }
        if (digest(row) !== job.annotationDigest) {
          job.error = { code: 'ANNOTATION_CHANGED', message: 'Local annotation changed while the Batch was running.' };
          continue;
        }
        let parsed;
        try {
          if (output.error || output.response?.status_code !== 200) throw new EmomoError('BATCH_ITEM_FAILED', 'This Batch item failed or expired. Retry pending items explicitly.');
          parsed = vision.annotationResult(output.response.body, job);
        } catch (error) {
          job.error = publicError(error);
          await save(directory, state);
          continue;
        }
        parsed.receipt.batchId = part.batchId;
        parsed.receipt.customId = job.customId;
        apply(row, parsed);
        await checkpoint();
        job.applied = true;
        delete job.error;
        await save(directory, state);
      }
    }
    for (const job of part.jobs) if (!job.applied && !job.error) job.error = { code: 'BATCH_RESULT_MISSING', message: 'No completed result was returned for this image.' };
    part.collected = true;
    await save(directory, state);
  }
}
function summary(directory, state) {
  const jobs = state.parts.flatMap(part => part.jobs);
  const unknown = state.parts.some(part => part.status === 'submission_unknown');
  const pending = state.parts.some(part => !part.collected);
  const readyToCollect = state.parts.some(part => TERMINAL.has(part.status) && !part.collected);
  return { status: unknown ? 'batch_submission_unknown' : readyToCollect ? 'batch_ready' : pending ? 'batch_pending' : jobs.some(job => !job.applied) ? 'batch_failed' : 'batch_completed',
    batch: directory, total: jobs.length, completed: jobs.filter(job => job.applied).length,
    failed: jobs.filter(job => job.error).map(job => ({ sha256: job.sha256, error: job.error })),
    batches: state.parts.map(part => ({ id: part.batchId, status: part.status, requests: part.jobs.length, estimatedInputTokens: part.inputTokens })),
    resumeCommand: `emomo-library batch-resume "${directory}"` };
}

export async function advanceAnnotationBatch({ directory, annotations, vision, readImage, apply, checkpoint, requested = false }) {
  let state = await load(directory);
  if (!state) {
    if (!requested) return null;
    if (!vision) throw new EmomoError('OPENAI_ACCESS_REQUIRED', 'Configure private OpenAI access to submit Batch jobs.');
    const pending = annotations.filter(row => row.decision === 'pending' && !row.llm);
    if (!pending.length) return null;
    state = await prepare(directory, pending, vision, readImage);
  }
  if (!vision) throw new EmomoError('OPENAI_ACCESS_REQUIRED', 'Configure private OpenAI access to resume the saved Batch.');
  validateState(state, vision);
  await refresh(directory, state, vision);
  await collect(directory, state, annotations, vision, apply, checkpoint);
  await submit(directory, state, vision);
  const result = summary(directory, state);
  return result.status === 'batch_completed' ? null : result;
}
export async function batchStatus(directory, vision) {
  return withImportLock(directory, async () => {
  const state = await load(directory);
  if (!state) throw fail('No saved Batch job exists in this directory.');
  validateState(state, vision);
  await refresh(directory, state, vision);
  return summary(directory, state);
  });
}
export async function retryAnnotationBatch(directory, vision) {
  return withImportLock(directory, async () => {
  const state = await load(directory);
  if (!state) throw fail('No saved Batch job exists in this directory.');
  validateState(state, vision);
  await refresh(directory, state, vision);
  if (state.parts.some(part => part.status === 'submission_unknown' || part.batchId && !part.collected)) throw fail('Resume and collect every existing job before retrying failed items.');
  await rename(join(directory, STATE_FILE), join(directory, `openai-batch-${randomUUID()}.json`));
  });
}
