import { readFile } from 'node:fs/promises';
import { EmomoClient, DEFAULT_API_URL } from './client.js';
import { EmomoError, publicError } from './error.js';
import { installSkill, SKILL_SOURCE } from './skill.js';

const PACKAGE = JSON.parse(await readFile(new URL('../package.json', import.meta.url), 'utf8'));
const GLOBAL = ['api-url', 'timeout'];
const COMMANDS = {
  search: { positional: 'query', flags: ['limit', 'category', 'text', 'profile', 'collection'], usage: 'emomo search "下班 开会" --limit 8' },
  get: { positional: 'id', flags: [], usage: 'emomo get <meme-id>' },
  download: { positional: 'id', flags: ['dir'], usage: 'emomo download <meme-id> --dir /tmp/emomo' },
  categories: { flags: [], usage: 'emomo categories' },
  stats: { flags: [], usage: 'emomo stats' },
  doctor: { flags: [], usage: 'emomo doctor' },
  capabilities: { flags: [], usage: 'emomo capabilities' },
  'skill install': { flags: ['agent', 'dir'], usage: 'emomo skill install --agent codex' },
  'skill path': { flags: [], usage: 'emomo skill path' },
};

function integer(raw, fallback, min, max, label) {
  if (raw === undefined) return fallback;
  if (!/^\d+$/.test(raw) || !Number.isSafeInteger(Number(raw)) || Number(raw) < min || Number(raw) > max) {
    throw new EmomoError('INVALID_ARGUMENT', `${label} must be an integer from ${min} to ${max}.`);
  }
  return Number(raw);
}

function parse(argv) {
  const flags = {};
  const words = [];
  let literal = false;
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (literal) { words.push(arg); continue; }
    if (arg === '--') { literal = true; continue; }
    if (arg === '--help' || arg === '-h') { flags.help = true; continue; }
    if (arg === '--version') { flags.version = true; continue; }
    if (arg === '--json') { flags.json = true; continue; }
    if (arg.startsWith('--')) {
      const equal = arg.indexOf('=');
      const name = arg.slice(2, equal === -1 ? undefined : equal);
      if (Object.hasOwn(flags, name)) throw new EmomoError('INVALID_ARGUMENT', 'Duplicate options are not allowed.');
      const value = equal === -1 ? argv[++i] : arg.slice(equal + 1);
      if (value === undefined || (equal === -1 && value.startsWith('--'))) throw new EmomoError('INVALID_ARGUMENT', 'An option value is missing.');
      flags[name] = value;
    } else if (arg.startsWith('-')) {
      throw new EmomoError('INVALID_ARGUMENT', 'Unknown option. Run emomo --help.');
    } else words.push(arg);
  }
  let command = words.shift() || 'help';
  if (command === 'skill') command += ` ${words.shift() || ''}`;
  if (flags.version) return { command: 'version', flags, words: [] };
  if (flags.help || command === 'help') return { command: 'help', flags, words: [] };
  const spec = COMMANDS[command];
  if (!spec) throw new EmomoError('INVALID_ARGUMENT', 'Unknown command. Run emomo --help.');
  const allowed = new Set([...GLOBAL, ...spec.flags, 'json']);
  if (Object.keys(flags).some(flag => !allowed.has(flag))) throw new EmomoError('INVALID_ARGUMENT', 'This command does not accept one of the supplied options.');
  if (words.length !== (spec.positional ? 1 : 0)) throw new EmomoError('INVALID_ARGUMENT', `Usage: ${spec.usage}`);
  return { command, flags, words };
}

function help() {
  return {
    name: 'emomo',
    version: PACKAGE.version,
    description: 'Search real memes with keywords, inspect candidates, and download static images for an Agent.',
    output: 'One JSON object on stdout. Exit 0 means success; exit 1 means error. --json is optional.',
    commands: Object.entries(COMMANDS).map(([command, spec]) => ({ command, usage: spec.usage, flags: spec.flags })),
    globalOptions: { '--api-url': 'HTTPS REST API base; defaults to EMOMO_API_URL or /agent/v1 on the public Emomo API. Local text search uses http://127.0.0.1:8787/agent/v1.', '--timeout': 'Request timeout in milliseconds, 100–120000; default 30000.', '--help': 'Print this JSON command catalog.' },
    environment: ['EMOMO_API_URL', 'EMOMO_API_TOKEN', 'EMOMO_IMAGE_HOSTS', 'CODEX_HOME'],
    defaultApiUrl: DEFAULT_API_URL,
  };
}

export async function run(argv, { env = process.env } = {}) {
  const { command, flags, words } = parse(argv);
  if (command === 'help' || command === 'capabilities') return { command, data: help() };
  if (command === 'version') return { command, data: { version: PACKAGE.version } };
  if (command === 'skill path') return { command, data: { path: SKILL_SOURCE } };
  if (command === 'skill install') return { command, data: await installSkill({ agent: flags.agent, directory: flags.dir, env }) };
  const imageHosts = env.EMOMO_IMAGE_HOSTS?.split(',').map(host => host.trim().toLowerCase()).filter(Boolean);
  const client = new EmomoClient({ baseUrl: flags['api-url'] || env.EMOMO_API_URL || DEFAULT_API_URL, token: env.EMOMO_API_TOKEN, imageHosts, timeoutMs: integer(flags.timeout, 30000, 100, 120000, 'timeout') });
  let data;
  if (command === 'search') {
    const query = words[0].trim();
    if (!query || [...query].length > 160) throw new EmomoError('INVALID_ARGUMENT', 'The search query must contain 1–160 characters.');
    const text = { any: 0, unknown: 1, with: 2, without: 3 };
    if (flags.text !== undefined && !Object.hasOwn(text, flags.text)) throw new EmomoError('INVALID_ARGUMENT', '--text accepts any, unknown, with, or without.');
    data = await client.search(query, { limit: integer(flags.limit, 8, 1, 100, 'limit'), category: flags.category, profile: flags.profile, collection: flags.collection, textPresence: flags.text === undefined ? undefined : text[flags.text] });
  } else if (command === 'get' || command === 'download') {
    const id = words[0].trim();
    if (!/^[a-zA-Z0-9_-]{1,200}$/.test(id)) throw new EmomoError('INVALID_ARGUMENT', 'Meme IDs may contain letters, digits, underscores, and hyphens.');
    if (command === 'download' && !flags.dir?.trim()) throw new EmomoError('INVALID_ARGUMENT', 'Choose a download directory with --dir.');
    data = command === 'get' ? await client.get(id) : await client.download(id, flags.dir);
  } else if (command === 'doctor') {
    data = { apiUrl: client.base.href, reachable: true, stats: await client.stats() };
  } else data = await client[command]();
  return { command, data };
}

export async function main(argv, { env = process.env, write = text => process.stdout.write(text) } = {}) {
  let envelope;
  let exitCode = 0;
  try {
    envelope = { schemaVersion: 1, ok: true, ...await run(argv, { env }) };
  } catch (error) {
    envelope = { schemaVersion: 1, ok: false, error: publicError(error) };
    exitCode = 1;
  }
  write(`${JSON.stringify(envelope)}\n`);
  return exitCode;
}
