import { execFileSync } from 'node:child_process';
import { appendFileSync } from 'node:fs';

const scopes = { backend: false, frontend: false, mobile: false, gateway: false };
const all = () => { for (const key of Object.keys(scopes)) scopes[key] = true; };
const base = process.env.BASE_SHA ?? '', head = process.env.HEAD_SHA ?? '';
if (process.env.EVENT_NAME === 'workflow_dispatch' || !/^[a-f0-9]{40}$/.test(base) || /^0+$/.test(base) || !/^[a-f0-9]{40}$/.test(head)) all();
else {
  try {
    const paths = execFileSync('git', ['diff', '--name-only', '--no-renames', '-z', base, head], { encoding: 'utf8' }).split('\0').filter(Boolean);
    for (const path of paths) {
      if (/^(?:\.github\/|scripts\/|protocol\/|backend\/proto\/|backend\/buf)/.test(path)) all();
      if (path.startsWith('backend/')) scopes.backend = true;
      if (path.startsWith('frontend/')) scopes.frontend = true;
      if (path.startsWith('mobile/')) scopes.mobile = true;
      if (path.startsWith('deployments/cloudflare/api-gateway/')) scopes.gateway = true;
    }
  } catch { all(); } // Unknown diffs never silently bypass retained-product checks.
}
const output = Object.entries(scopes).map(([key, value]) => `${key}=${value}`).join('\n') + '\n';
if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, output);
process.stdout.write(output);
