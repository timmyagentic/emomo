const { spawnSync } = require('node:child_process');
const { verifyOrPatch } = require('./patch-node-forge.cjs');

verifyOrPatch(true);
const result = spawnSync(process.platform === 'win32' ? 'npm.cmd' : 'npm',
  ['audit', '--json', '--ignore-scripts', '--registry=https://registry.npmjs.org'],
  { encoding: 'utf8', shell: process.platform === 'win32' });
if (result.error || ![0, 1].includes(result.status)) throw result.error || new Error(result.stderr);
const report = JSON.parse(result.stdout);
if (report.error || report.auditReportVersion !== 2 || !report.vulnerabilities || !report.metadata) {
  throw new Error('Unable to verify the npm audit report');
}
const backport = 'https://github.com/advisories/GHSA-86w9-cpqp-85rv';
const unresolved = new Map();
function leaves(name, seen = new Set()) {
  if (seen.has(name) || !report.vulnerabilities[name]) throw new Error(`Unexpected npm advisory graph: ${name}`);
  const next = new Set(seen).add(name);
  return report.vulnerabilities[name].via.flatMap((advisory) =>
    typeof advisory === 'string' ? leaves(advisory, next) : [advisory]);
}
for (const [name, vulnerability] of Object.entries(report.vulnerabilities)) {
  if (!['high', 'critical'].includes(vulnerability.severity)) continue;
  const advisories = leaves(name);
  if (!advisories.length) throw new Error(`Unexplained npm severity: ${name}`);
  for (const advisory of advisories) {
    if (advisory.url !== backport) {
      unresolved.set(advisory.url, advisory.title);
    }
  }
}
console.log('Raw npm audit counts:', report.metadata.vulnerabilities);
console.log('Verified source backport:', backport);
if (unresolved.size) {
  for (const [url, title] of unresolved) console.error(`${title}: ${url}`);
  process.exitCode = 1;
} else {
  console.log('Unresolved high/critical advisories: 0 (raw node-forge findings remain visible above)');
}
