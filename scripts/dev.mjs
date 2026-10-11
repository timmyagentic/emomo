#!/usr/bin/env node
// Secret-free checkout bootstrap/check entry point. No user config is sourced.
import { spawnSync } from 'node:child_process';
import { mkdirSync, readdirSync } from 'node:fs';
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
      add('.', 'node', '--test', 'scripts/test/goenv.test.mjs');
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
      if (name === '.npmrc' || (name === '.env' || name.startsWith('.env.') || name === '.dev.vars' || name.startsWith('.dev.vars.')) && !name.endsWith('.example')) {
        throw new Error(`Refusing local environment file: ${directory}/${name}; use a clean checkout (contents were not read).`);
      }
    }
  }
}

export function developmentEnv(root, source = process.env) {
  const state = join(root, '.dev-cache');
  // Isolate HOME as well: Go netrc, Git and tool fallbacks otherwise read user state.
  const env = {};
  for (const key of ['PATH', 'TMPDIR', 'TMP', 'TEMP', 'SystemRoot',
    'HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'NO_PROXY',
    'http_proxy', 'https_proxy', 'all_proxy', 'no_proxy',
    'NODE_EXTRA_CA_CERTS', 'SSL_CERT_FILE', 'SSL_CERT_DIR', 'npm_config_cafile']) {
    if (source[key]) env[key] = source[key];
  }
  return Object.assign(env, {
    CI: '1', NO_COLOR: '1', HOME: join(state, 'home'), USERPROFILE: join(state, 'home'),
    GIT_CONFIG_NOSYSTEM: '1', GIT_CONFIG_GLOBAL: join(state, 'empty-gitconfig'),
    GIT_CONFIG_COUNT: '2', GIT_CONFIG_KEY_0: 'core.hooksPath', GIT_CONFIG_VALUE_0: join(state, 'empty-hooks'),
    GIT_CONFIG_KEY_1: 'commit.gpgSign', GIT_CONFIG_VALUE_1: 'false',
    GIT_TERMINAL_PROMPT: '0', GOAUTH: 'off',
    npm_config_cache: join(state, 'npm'), npm_config_userconfig: join(state, 'empty-npmrc'),
    npm_config_globalconfig: join(state, 'empty-global-npmrc'),
    XDG_CONFIG_HOME: join(state, 'config'), XDG_CACHE_HOME: join(state, 'cache'),
    WRANGLER_LOG_PATH: join(state, 'logs'), WRANGLER_SEND_METRICS: 'false',
    CLOUDFLARE_LOAD_DEV_VARS_FROM_DOT_ENV: 'false',
    EXPO_NO_DOTENV: '1', EXPO_NO_TELEMETRY: '1',
    GOENV: 'off', GOTOOLCHAIN: 'go1.26.2', GOPATH: join(state, 'go'),
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
  const env = developmentEnv(root);
  for (const key of ['HOME', 'GIT_CONFIG_VALUE_0', 'npm_config_cache', 'XDG_CONFIG_HOME', 'XDG_CACHE_HOME', 'WRANGLER_LOG_PATH', 'GOPATH', 'GOMODCACHE', 'GOCACHE']) mkdirSync(env[key], { recursive: true });
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
