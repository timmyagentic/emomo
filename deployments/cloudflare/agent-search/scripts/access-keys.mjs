#!/usr/bin/env node
// Offline operator utility. Prints identifiers only; never prints credentials.
import { randomBytes, createHash } from 'node:crypto';
import { readFile, lstat, mkdir, writeFile, rename, rm } from 'node:fs/promises';
import { resolve, dirname, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
const repo = resolve(dirname(fileURLToPath(import.meta.url)), '../../../..');
const [command, ...args] = process.argv.slice(2);
const flags = {};
for (let i = 0; i < args.length; i += 2) {
  if (!args[i].startsWith('--') || !args[i + 1] || flags[args[i]]) throw Error('Use named options with one value each.');
  flags[args[i]] = args[i + 1];
}
const privatePath = value => {
  if (!value) throw Error('Missing private file path.');
  const path = resolve(value);
  if (path === repo || path.startsWith(repo + sep)) throw Error('Store credentials outside the repository.');
  return path;
};
const store = privatePath(flags['--store']);
async function readStore() {
  try {
    const stat = await lstat(store);
    if (!stat.isFile() || stat.isSymbolicLink() || stat.size > 131072 || stat.mode & 0o077) throw Error('Registry must be a private regular file (0600).');
    return JSON.parse(await readFile(store, 'utf8'));
  } catch (e) { if (e.code === 'ENOENT') return {}; throw e; }
}
async function saveStore(data) {
  await mkdir(dirname(store), { recursive: true, mode: 0o700 });
  const temp = store + '.' + randomBytes(8).toString('hex');
  try { await writeFile(temp, JSON.stringify(data) + '\n', { mode: 0o600, flag: 'wx' }); await rename(temp, store); }
  finally { await rm(temp, { force: true }); }
}
const data = await readStore();
const id = flags['--id'];
if (command === 'create') {
  if (!/^[a-zA-Z0-9-]{1,48}$/.test(id || '') || Object.hasOwn(data, id)) throw Error('Use a new key id (1–48 letters, digits or hyphens). Never reuse revoked ids.');
  const token = `emomo_${id}_${randomBytes(32).toString('hex')}`;
  const record = { hash: createHash('sha256').update(token).digest('hex') };
  if (flags['--expires-days']) {
    const days = Number(flags['--expires-days']);
    if (!Number.isFinite(days) || days <= 0 || days > 3650) throw Error('Invalid expiry.');
    record.expiresAt = Date.now() + Math.floor(days * 86400000);
  }
  if (flags['--quota']) {
    const quota = Number(flags['--quota']);
    if (!Number.isSafeInteger(quota) || quota < 1 || quota > 1000000) throw Error('Invalid quota.');
    record.quotas = Object.fromEntries(['search','read','download'].map(op => [op, { hour: quota, day: quota }]));
  }
  const keyPath = privatePath(flags['--key-file']);
  await mkdir(dirname(keyPath), { recursive: true, mode: 0o700 });
  await writeFile(keyPath, token + '\n', { mode: 0o600, flag: 'wx' });
  data[id] = record;
  await saveStore(data);
  console.log(JSON.stringify({ created: id, keyFile: keyPath }));
} else if (command === 'revoke') {
  if (!Object.hasOwn(data, id)) throw Error('Unknown key id.');
  data[id].disabled = true;
  await saveStore(data); console.log(JSON.stringify({ revoked: id }));
} else if (command === 'list') {
  console.log(JSON.stringify(Object.entries(data).map(([id, r]) => ({ id, disabled: !!r.disabled, expiresAt: r.expiresAt, quotas: r.quotas }))));
} else if (command === 'export') {
  const output = privatePath(flags['--output']);
  await writeFile(output, JSON.stringify({ ACCESS_KEYS_JSON: JSON.stringify(data) }) + '\n', { mode: 0o600, flag: 'wx' });
  console.log(JSON.stringify({ secretFile: output }));
} else throw Error('Use create, revoke, list or export with --store <private-registry-file>.');
