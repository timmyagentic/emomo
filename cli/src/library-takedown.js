import { createHash, randomUUID } from 'node:crypto';
import { lstat, readFile, rename, unlink, writeFile } from 'node:fs/promises';
import { dirname, isAbsolute, resolve } from 'node:path';
import { connectTakedown, configuredTakedownClient } from './import-client.js';
import { SHA256, MAX_IMPORT_IMAGE_BYTES } from './import-metadata.js';
import { EmomoError, publicError } from './error.js';
import { canonical } from './library.js';
import { withImportLock } from './import-lock.js';

const ID = /^[a-zA-Z0-9_-]{1,120}$/;
const MAX_SELECTION = 100;
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const fail = message => new EmomoError('INVALID_TAKEDOWN', message);
const isString = value => typeof value === 'string';

async function readRegular(path, max) {
  const info = await lstat(path);
  if (!info.isFile() || info.size > max) throw fail('Choose a regular file within the size limit.');
  const bytes = await readFile(path);
  if (bytes.length > max) throw fail('The input file exceeds the size limit.');
  return bytes;
}

async function loadJSON(file) {
  const bytes = await readRegular(file, 32 * 1024 * 1024);
  try { return JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes)); }
  catch { throw fail('The takedown file must contain valid JSON.'); }
}

async function select(input) {
  const source = await loadJSON(input);
  if (!Array.isArray(source) || !source.length || source.length > MAX_SELECTION) {
    throw fail('Select 1–100 production image IDs, SHA-256 values or original files.');
  }
  const selectors = new Map();
  for (const item of source) {
    if (!item || Object.keys(item).length !== 1) throw fail('Each selection must contain exactly one id, sha256 or file.');
    let selector;
    if (isString(item.sha256) && SHA256.test(item.sha256)) selector = { sha256: item.sha256 };
    else if (isString(item.id) && ID.test(item.id)) selector = { id: item.id };
    else if (isString(item.file) && isAbsolute(item.file)) {
      selector = { sha256: hash(await readRegular(item.file, MAX_IMPORT_IMAGE_BYTES)) };
    } else throw fail('Use a production image ID, a lowercase SHA-256 or an absolute path to the original.');
    selectors.set(canonical(selector), selector);
  }
  return [...selectors.values()];
}

function validateTarget(result) {
  const valid = result && isString(result.sha256) && SHA256.test(result.sha256)
    && isString(result.confirmation) && SHA256.test(result.confirmation)
    && isString(result.taken_down_at) && (!result.taken_down_at || /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z$/.test(result.taken_down_at))
    && Array.isArray(result.targets) && result.targets.length && result.targets.length <= 100
    && result.targets.every(row => row && isString(row.id) && ID.test(row.id)
      && isString(row.storage_key) && row.storage_key.length > 0 && row.storage_key.length <= 1024);
  if (!valid) throw fail('Invalid takedown preview or receipt.');
  const { sha256, confirmation, taken_down_at, targets } = result;
  if (new Set(targets.map(row => row.id)).size !== targets.length
      || hash(JSON.stringify([sha256, targets.map(row => [row.id, row.storage_key])])) !== confirmation) {
    throw fail('Takedown confirmation does not match the affected records.');
  }
  return { sha256, targets, confirmation, taken_down_at };
}

async function prepare(input, output, client) {
  const selectors = await select(input);
  const items = new Map();
  for (const selector of selectors) {
    const item = validateTarget(await client.request('takedowns/plan', selector));
    if (selector.sha256 && selector.sha256 !== item.sha256 || selector.id && !item.targets.some(row => row.id === selector.id)) {
      throw fail('Takedown preview does not match the selected image.');
    }
    const previous = items.get(item.sha256);
    if (previous && previous.confirmation !== item.confirmation) throw fail('The affected records changed while preparing this plan.');
    items.set(item.sha256, item);
  }
  const plan = { schemaVersion: 1, apiUrl: client.apiUrl, items: [...items.values()] };
  const confirmation = hash(canonical(plan));
  const report = { ...plan, confirmation };
  try { await writeFile(output, JSON.stringify(report, null, 2) + '\n', { flag: 'wx', mode: 0o600 }); }
  catch (error) {
    if (error.code === 'EEXIST') throw fail('Choose a new plan filename.');
    throw error;
  }
  return { status: 'ready', plan: output, ...report };
}

async function apply(path, confirmation, client) {
  const { confirmation: expected, ...plan } = await loadJSON(path);
  if (plan.schemaVersion !== 1 || !SHA256.test(confirmation) || confirmation !== expected || hash(canonical(plan)) !== expected
      || plan.apiUrl !== client.apiUrl || !Array.isArray(plan.items) || !plan.items.length || plan.items.length > MAX_SELECTION) {
    throw fail('Confirm the exact prepared plan and maintenance endpoint.');
  }
  const items = plan.items.map(validateTarget);
  if (new Set(items.map(item => item.sha256)).size !== items.length) throw fail('Takedown plan has duplicate images.');
  const receipt = `${path}.receipt.json`;
  const previous = await loadJSON(receipt).catch(error => {
    if (error.code !== 'ENOENT') throw error;
    return null;
  });
  if (previous && (previous.schemaVersion !== 1 || previous.confirmation !== confirmation
      || previous.apiUrl !== client.apiUrl || !Array.isArray(previous.results) || previous.results.length > items.length)) {
    throw fail('The existing receipt belongs to another plan.');
  }
  const results = (previous?.results ?? []).map(validateTarget);
  if (results.some((row, index) => !row.taken_down_at || row.confirmation !== items[index].confirmation)) {
    throw fail('The existing receipt does not match this plan.');
  }
  const report = { schemaVersion: 1, status: results.length === items.length ? 'taken_down' : 'applying',
    confirmation, apiUrl: client.apiUrl, results, remaining: items.length - results.length, receipt };
  const checkpoint = async () => {
    const temporary = `${receipt}.${randomUUID()}.tmp`;
    try {
      await writeFile(temporary, JSON.stringify(report, null, 2) + '\n', { flag: 'wx', mode: 0o600 });
      await rename(temporary, receipt);
    } finally { await unlink(temporary).catch(() => {}); }
  };
  for (const item of items.slice(results.length)) {
    try {
      const result = validateTarget(await client.request('takedowns', { sha256: item.sha256, confirmation: item.confirmation }));
      if (result.sha256 !== item.sha256 || result.confirmation !== item.confirmation || !result.taken_down_at) {
        throw fail('Takedown response does not match the confirmed plan.');
      }
      results.push(result);
      report.remaining--;
    } catch (error) {
      report.status = 'partial';
      report.failed = { sha256: item.sha256, error: publicError(error) };
      await checkpoint();
      return report;
    }
    report.status = report.remaining ? 'applying' : 'taken_down';
    await checkpoint();
  }
  return report;
}

export async function takedownCommand([command, first, option, value, ...extra], env = process.env) {
  if (command === 'connect-takedown') {
    const args = [first, option, value, ...extra].filter(part => part !== undefined);
    const input = args.splice(0, 2);
    if (input[0] !== '--key-file' || !input[1] || input[1].startsWith('--') || (args.length &&
        (args.length !== 2 || args[0] !== '--api-url' || !args[1]))) {
      throw fail('Use connect-takedown --key-file PRIVATE_FILE [--api-url URL].');
    }
    return connectTakedown(input[1], args[1], env);
  }
  if (!first || first.startsWith('--') || !value || value.startsWith('--') || extra.length
      || option !== (command === 'takedown-plan' ? '--out' : '--confirm')) {
    throw fail('Use takedown-plan SELECTION --out PLAN or takedown PLAN --confirm REVISION.');
  }
  const client = await configuredTakedownClient(env);
  if (!client) throw new EmomoError('TAKEDOWN_ACCESS_REQUIRED', 'Configure maintenance access with emomo-library connect-takedown.');
  const path = resolve(first);
  if (command === 'takedown-plan') return prepare(path, resolve(value), client);
  return withImportLock(dirname(path), () => apply(path, value, client));
}
