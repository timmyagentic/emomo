import { readAccess, configureAccess, accessStatus, clearAccess, canonicalApi } from './auth.js';
import { readFile } from 'node:fs/promises';
import { EmomoClient, DEFAULT_API_URL, OFFICIAL_ACCESS_GUIDANCE } from './client.js';
import { EmomoError, publicError } from './error.js';
import { installSkill, SKILL_SOURCE } from './skill.js';
import { LocalCatalog } from './local.js';
import { selectLibrary, useCatalog } from './config.js';

const PACKAGE = JSON.parse(await readFile(new URL('../package.json', import.meta.url), 'utf8'));
const GLOBAL = ['api-url', 'timeout', 'catalog'];
const COMMANDS = {
  'auth configure': { flags: ['key-file'], usage: 'emomo auth configure --key-file /private/path/access-key.txt [--api-url https://api.timmyagentic.si/agent/v1]' },
  'auth status': { flags: [], usage: 'emomo auth status' },
  'auth clear': { flags: [], usage: 'emomo auth clear' },
  search: { positional: 'query', flags: ['limit', 'category', 'text', 'profile', 'collection', 'media', 'subject', 'intent', 'include-objects'], usage: 'emomo search "下班 开会" --limit 8' },
  get: { positional: 'id', flags: [], usage: 'emomo get <meme-id>' },
  download: { positional: 'id', flags: ['dir'], usage: 'emomo download <meme-id> --dir /tmp/emomo' },
  categories: { flags: [], usage: 'emomo categories' },
  stats: { flags: [], usage: 'emomo stats' },
  doctor: { flags: [], usage: 'emomo doctor' },
  capabilities: { flags: [], usage: 'emomo capabilities' },
  'catalog import': { positional: 'metadata.json', flags: ['dir', 'vocabulary'], usage: 'emomo catalog import <metadata.json> --dir <new-catalog-directory> [--vocabulary <vocabulary.json>]' },
  'catalog import-reviewed': { positional: 'metadata.jsonl', flags: ['dir'], usage: 'emomo catalog import-reviewed <metadata.jsonl> --dir <new-catalog-directory>' },
  'catalog use': { positional: 'directory-or-api-url', flags: [], usage: 'emomo catalog use <catalog-directory-or-api-url>' },
  'catalog path': { flags: [], usage: 'emomo catalog path' },
  'skill install': { flags: ['agent', 'dir'], usage: 'emomo skill install --agent codex' },
  'skill path': { flags: [], usage: 'emomo skill path' },
};
const DAILY = ['search', 'get', 'download', 'auth configure', 'doctor'];

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
    if (arg === '--all') { flags.all = true; continue; }
    if (arg === '--version') { flags.version = true; continue; }
    if (arg === '--include-objects') { if (flags['include-objects']) throw new EmomoError('INVALID_ARGUMENT', 'Duplicate options are not allowed.'); flags['include-objects'] = true; continue; }
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
  if (command === 'skill' || command === 'catalog' || command === 'auth') command += ` ${words.shift() || ''}`;
  if (flags.version) return { command: 'version', flags, words: [] };
  if (flags.help || command === 'help') return { command: 'help', flags, words: [] };
  const spec = COMMANDS[command];
  if (!spec) throw new EmomoError('INVALID_ARGUMENT', 'Unknown command. Run emomo --help.');
  const allowed = new Set([...GLOBAL, ...spec.flags, 'json']);
  if (Object.keys(flags).some(flag => !allowed.has(flag))) throw new EmomoError('INVALID_ARGUMENT', 'This command does not accept one of the supplied options.');
  if (flags['include-objects'] !== undefined && typeof flags['include-objects'] !== 'boolean') throw new EmomoError('INVALID_ARGUMENT', '--include-objects is a boolean switch.');
  if (words.length !== (spec.positional ? 1 : 0)) throw new EmomoError('INVALID_ARGUMENT', `Usage: ${spec.usage}`);
  return { command, flags, words };
}

function help(all = false) {
  const data = {
    name: 'emomo',
    version: PACKAGE.version,
    description: 'Search real memes with keywords, inspect candidates, and get complete original images or animations for an Agent.',
    output: 'One JSON object on stdout. Exit 0 means success; exit 1 means error. --json is optional.',
    commands: (all ? Object.keys(COMMANDS) : DAILY).map(command => ({ command, usage: COMMANDS[command].usage, flags: !all && command === 'search' ? ['limit', 'category', 'text'] : COMMANDS[command].flags })),
    advanced: 'emomo --help --all lists offline, installation and compatibility commands. emomo-library help lists local maintenance commands.',
    globalOptions: { '--catalog': 'Explicit portable local catalog; no network or model calls. --api-url overrides the saved catalog to select a remote API.', '--api-url': 'HTTPS REST API base; defaults to EMOMO_API_URL or /agent/v1 on the public Emomo API. Local text search uses http://127.0.0.1:8787/agent/v1.', '--timeout': 'Request timeout in milliseconds, 100–120000; default 30000.', '--help': 'Print this JSON command catalog.' },
    environment: ['EMOMO_API_URL', 'EMOMO_API_TOKEN', 'EMOMO_API_TOKEN_URL', 'EMOMO_IMAGE_HOSTS', 'CODEX_HOME', 'EMOMO_CATALOG', 'EMOMO_CONFIG_DIR', 'XDG_CONFIG_HOME'],
    authentication: 'For a private API, set EMOMO_API_TOKEN to a credential intended for that endpoint and bind it with EMOMO_API_TOKEN_URL (or EMOMO_API_URL when EMOMO_API_TOKEN_URL is unset). The binding must match the selected API base URL origin and path; trailing slashes are ignored. Unbound or mismatched tokens are not sent. The hosted API requires an issued access key. Configure it with auth configure --key-file. For the official API only, the key is also sent to canonical HTTPS images.timmyagentic.si downloads; authenticated image redirects are rejected. Other image hosts never receive the key.',
    officialAccess: OFFICIAL_ACCESS_GUIDANCE,
    defaultApiUrl: DEFAULT_API_URL,
  };
  if (!all) {
    data.globalOptions = { '--help --all': 'Offline, installation and compatibility commands.', '--version': 'Installed version.', '--timeout': 'Remote request timeout in milliseconds.' };
    delete data.environment;
    data.authentication = 'Configure a private key file with auth configure. doctor reports the selected library and access state.';
  }
  return data;
}

export async function run(argv, { env = process.env } = {}) {
  const { command, flags, words } = parse(argv);
  if (command === 'help' || command === 'capabilities') return { command, data: help(command === 'capabilities' || flags.all) };
  if (command === 'version') return { command, data: { version: PACKAGE.version } };
  if (command === 'auth configure') return { command, data: await configureAccess(flags['key-file'], flags['api-url'] || DEFAULT_API_URL, env) };
  if (command === 'auth status') return { command, data: await accessStatus(env) };
  if (command === 'auth clear') return { command, data: await clearAccess(env) };
  if (command === 'skill path') return { command, data: { path: SKILL_SOURCE } };
  if (command === 'skill install') return { command, data: await installSkill({ agent: flags.agent, directory: flags.dir, env }) };
  if (command === 'catalog import' || command === 'catalog import-reviewed') {
    if (!flags.dir?.trim()) throw new EmomoError('INVALID_ARGUMENT', 'Choose a new catalog directory with --dir.');
    const { importMetadata } = await import('./refined-import.js');
    return { command, data: await importMetadata(words[0], flags.dir, flags.vocabulary) };
  }
  if (command === 'catalog use') return { command, data: await useCatalog(words[0], env) };
  const library = await selectLibrary(flags, env);
  const catalogPath = library.type === 'local' ? library.path : undefined;
  if (command === 'catalog path') return { command, data: { path: catalogPath ?? null } };
  if (!catalogPath && ['media', 'subject', 'intent', 'include-objects'].some(f => flags[f] !== undefined)) throw new EmomoError('INVALID_ARGUMENT', 'Media, subject, intent and object filters require a local catalog.');
  const imageHosts = env.EMOMO_IMAGE_HOSTS?.split(',').map(host => host.trim().toLowerCase()).filter(Boolean);
  const selectedApi = library.apiUrl || DEFAULT_API_URL;
  let token = boundToken(selectedApi, env);
  let saved;
  if (!catalogPath && !env.EMOMO_API_TOKEN) {
    saved = await readAccess(env);
    if (saved) { try { if (saved.apiUrl === canonicalApi(selectedApi)) token = saved.token; } catch { /* mismatched endpoint receives no key */ } }
  }
  const client = catalogPath ? await LocalCatalog.open(catalogPath) : new EmomoClient({ baseUrl: selectedApi, token, imageHosts, timeoutMs: integer(flags.timeout, 30000, 100, 120000, 'timeout') });
  let data;
  try {
  if (command === 'search') {
    const query = words[0].trim();
    if (!query || [...query].length > 160) throw new EmomoError('INVALID_ARGUMENT', 'The search query must contain 1–160 characters.');
    const text = { any: 0, unknown: 1, with: 2, without: 3 };
    if (flags.text !== undefined && !Object.hasOwn(text, flags.text)) throw new EmomoError('INVALID_ARGUMENT', '--text accepts any, unknown, with, or without.');
    if (flags.media && !['image', 'preview', 'animation'].includes(flags.media)) throw new EmomoError('INVALID_ARGUMENT', '--media accepts image, preview, or animation.');
    data = await client.search(query, { limit: integer(flags.limit, 8, 1, 100, 'limit'), category: flags.category, profile: flags.profile, collection: flags.collection, textPresence: flags.text === undefined ? undefined : text[flags.text], media: flags.media, subject: flags.subject, intent: flags.intent, includeObjects: flags['include-objects'] ?? false });
  } else if (command === 'get' || command === 'download') {
    const id = words[0].trim();
    if (!/^[a-zA-Z0-9_-]{1,200}$/.test(id)) throw new EmomoError('INVALID_ARGUMENT', 'Meme IDs may contain letters, digits, underscores, and hyphens.');
    if (command === 'download' && !flags.dir?.trim()) throw new EmomoError('INVALID_ARGUMENT', 'Choose a download directory with --dir.');
    data = command === 'get' ? await client.get(id) : await client.download(id, flags.dir);
  } else if (command === 'doctor') {
    data = catalogPath ? await client.verify() : { apiUrl: client.base.href, reachable: true, stats: await client.stats() };
    let access = env.EMOMO_API_TOKEN ? { configured: true, source: 'environment' } : { configured: !!saved, ...(saved ? { apiUrl: saved.apiUrl } : {}), source: 'saved' };
    if (catalogPath && !env.EMOMO_API_TOKEN) {
      try { access = { ...await accessStatus(env), source: 'saved' }; }
      catch (error) { access = { configured: null, error: publicError(error).code, source: 'saved' }; }
    }
    data.auth = { ...access, used: !catalogPath && !!token };
  } else data = await client[command]();
  if (['search', 'get', 'download', 'doctor'].includes(command)) data.library = library;
  return { command, data };
  } finally { if (catalogPath) client.close(); }
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

// Ambient credentials belong to the configured endpoint, never an arbitrary override.
export function boundToken(selected, env) {
  if (!env.EMOMO_API_TOKEN) return undefined;
  const binding = env.EMOMO_API_TOKEN_URL || env.EMOMO_API_URL;
  if (!binding) return undefined;
  try {
    const canonical = value => {
      const url = new URL(value);
      if (url.username || url.password || url.search || url.hash || !['https:', 'http:'].includes(url.protocol)) throw new Error('Invalid token binding');
      return url.origin + url.pathname.replace(/\/+$/, '');
    };
    return canonical(selected) === canonical(binding) ? env.EMOMO_API_TOKEN : undefined;
  } catch { return undefined; }
}
