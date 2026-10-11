#!/usr/bin/env node
// Secret-free checkout bootstrap/check entry point. No user config is sourced.
import { spawnSync } from 'node:child_process';
import { existsSync, mkdirSync, readdirSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

export function plan(action, profile) {
  if (!['bootstrap', 'check'].includes(action) || !['agent', 'full'].includes(profile)) {
    throw new Error('Usage: node scripts/dev.mjs <bootstrap|check> [agent|full]');
  }
  const search = 'deployments/cloudflare/agent-search';
  const gateway = 'deployments/cloudflare/api-gateway';
  const commands = [];
  const add = (cwd, ...args) => commands.push({ cwd, args });
  if (action === 'bootstrap') {
    add(search, 'npm', 'ci', '--no-audit', '--no-fund');
    if (profile === 'full') {
      for (const cwd of [gateway, 'frontend', 'mobile']) add(cwd, 'npm', 'ci', '--no-audit', '--no-fund');
      add('backend', 'go', 'mod', 'download');
    }
  } else {
    add('.', 'node', '--test', 'scripts/test/dev.test.mjs');
    add('cli', 'npm', 'run', 'check');
    add('cli', 'npm', 'test');
    add(search, 'npm', 'run', 'check');
    if (profile === 'full') {
      add(gateway, 'npm', 'test');
      add(gateway, 'npm', 'run', 'typecheck');
      add('backend', 'go', 'test', './...');
      add('backend', 'go', 'build', './...');
      add('frontend', 'npm', 'run', 'lint');
      add('frontend', 'npm', 'run', 'build');
      add('mobile', 'npm', 'run', 'gen');
      add('mobile', 'git', 'diff', '--exit-code', '--', 'gen');
      add('mobile', 'npm', 'test', '--', '--runInBand');
      add('mobile', 'npm', 'run', 'typecheck');
      add('mobile', 'npm', 'run', 'lint');
      add('mobile', 'npx', '--yes', 'expo-doctor');
      add('.', 'bash', 'scripts/check-mobile-ci.sh');
    }
  }
  return commands;
}

export function assertCleanConfig(root, directories) {
  for (const directory of new Set(['.', ...directories])) {
    for (const name of readdirSync(join(root, directory))) {
      if ((name === '.env' || name.startsWith('.env.') || name === '.dev.vars' || name.startsWith('.dev.vars.')) && !name.endsWith('.example')) {
        throw new Error(`Refusing local environment file: ${directory}/${name}; use a clean checkout (contents were not read).`);
      }
    }
  }
}

export function developmentEnv(root, source = process.env) {
  const state = join(root, '.dev-cache');
  // Keep HOME intact; isolate tool-specific config/cache instead. Do not inherit credentials.
  const env = {};
  for (const key of ['PATH', 'HOME', 'TMPDIR', 'TMP', 'TEMP', 'SystemRoot',
    'HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'NO_PROXY',
    'http_proxy', 'https_proxy', 'all_proxy', 'no_proxy',
    'NODE_EXTRA_CA_CERTS', 'SSL_CERT_FILE', 'SSL_CERT_DIR', 'npm_config_cafile']) {
    if (source[key]) env[key] = source[key];
  }
  return Object.assign(env, {
    CI: '1', NO_COLOR: '1',
    npm_config_cache: join(state, 'npm'), npm_config_userconfig: '/dev/null',
    npm_config_globalconfig: join(state, 'empty-global-npmrc'),
    XDG_CONFIG_HOME: join(state, 'config'), XDG_CACHE_HOME: join(state, 'cache'),
    WRANGLER_LOG_PATH: join(state, 'logs'), WRANGLER_SEND_METRICS: 'false',
    CLOUDFLARE_LOAD_DEV_VARS_FROM_DOT_ENV: 'false',
    EXPO_NO_DOTENV: '1', EXPO_NO_TELEMETRY: '1',
    GOTOOLCHAIN: 'go1.26.2', GOPATH: join(state, 'go'),
    GOMODCACHE: join(state, 'go', 'pkg', 'mod'), GOCACHE: join(state, 'go-build'),
  });
}

function main() {
  if (process.argv.length > 4) throw new Error('Too many arguments');
  const [action, profile = 'agent'] = process.argv.slice(2);
  const commands = plan(action, profile);
  const [major, minor] = process.versions.node.split('.').map(Number);
  if (major < 22 || (major === 22 && minor < 13)) throw new Error('Node.js >=22.13 required');
  const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
  const directories = commands.map(command => command.cwd);
  // Wrangler may also inspect ancestors of its project directory.
  assertCleanConfig(root, [...directories, 'deployments', 'deployments/cloudflare']);
  if (process.env.HOME && existsSync(join(process.env.HOME, '.wrangler'))) {
    throw new Error('Legacy ~/.wrangler exists; use a clean development environment without Cloudflare login state.');
  }
  const env = developmentEnv(root);
  for (const key of ['npm_config_cache', 'XDG_CONFIG_HOME', 'XDG_CACHE_HOME', 'WRANGLER_LOG_PATH', 'GOPATH', 'GOMODCACHE', 'GOCACHE']) mkdirSync(env[key], { recursive: true });
  for (const { cwd, args } of commands) {
    console.log(`\n[${profile}] ${cwd}: ${args.join(' ')}`);
    const result = spawnSync(args[0], args.slice(1), { cwd: join(root, cwd), env, stdio: 'inherit' });
    if (result.error) throw result.error;
    if (result.status !== 0) throw new Error(`${args[0]} failed (${result.signal ?? result.status})`);
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { main(); } catch (error) { console.error(error.message); process.exitCode = 1; }
}
