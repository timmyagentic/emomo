// Generate directly from the canonical schemas, without installing the old app.
import { execFileSync } from 'node:child_process';
import { mkdtemp, readFile, writeFile, mkdir, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, '..');
const targets = {
  worker: ['deployments/cloudflare/agent-search', ['api', 'meme', 'types']],
  website: ['website', ['account']],
};
const args = process.argv.slice(2);
const check = args.includes('--check');
const requested = args.filter(arg => arg !== '--check');
if (requested.length > 1 || requested.some(name => !Object.hasOwn(targets, name))) throw new Error('Usage: node protocol/generate.mjs [worker|website] [--check]');
const temporary = await mkdtemp(join(tmpdir(), 'emomo-proto-'));
try {
  const template = { version: 'v2', plugins: [{ local: [process.execPath, join(here, 'node_modules/@bufbuild/protoc-gen-es/bin/protoc-gen-es')], out: 'gen', opt: ['target=ts', 'import_extension=.js'] }] };
  execFileSync(process.execPath, [join(here, 'node_modules/@bufbuild/buf/bin/buf'), 'generate', join(root, 'backend/proto'), '--template', JSON.stringify(template), '--output', temporary], { cwd: here, stdio: 'pipe' });
  for (const name of requested.length ? requested : Object.keys(targets)) {
    const [directory, files] = targets[name];
    for (const file of files) {
      const target = join(root, directory, 'gen/emomo/v1', `${file}_pb.ts`);
      const generated = (await readFile(join(temporary, 'gen/emomo/v1', `${file}_pb.ts`), 'utf8')).trimEnd() + '\n';
      if (check) {
        if (await readFile(target, 'utf8') !== generated) throw new Error(`Generated ${name}/${file} differs from canonical proto. Run npm --prefix protocol run generate.`);
      } else { await mkdir(dirname(target), { recursive: true }); await writeFile(target, generated); }
    }
  }
} finally { await rm(temporary, { recursive: true, force: true }); }
