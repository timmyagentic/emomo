// Temporary CVE-2026-85393 backport until node-forge publishes its fix.
// Equivalent guard: https://github.com/digitalbazaar/forge/pull/1152
const { createHash } = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');

const root = path.resolve(__dirname, '..');
const originalHash = 'fd4740238145ec26470eb3f06a627c72039538ce1307dbdce40521f94dfd0a50';
const patchedHash = 'c9b1e3799e230528b6d6815c1f6cd3c6058b9d45975264b55d995abb976589af';
const before = '            obj.value.length !== 2) {';
const after = "            obj.value.length !== 2 ||\n" +
  "            obj.value[0].value.length !==\n" +
  "              (('parameters' in capture) ? 2 : 1)) {";
const hash = (bytes) => createHash('sha256').update(bytes).digest('hex');

function verifyOrPatch(checkOnly = false) {
  const lock = JSON.parse(fs.readFileSync(path.join(root, 'package-lock.json'), 'utf8'));
  const copies = Object.entries(lock.packages).filter(([name]) =>
    name === 'node_modules/node-forge' || name.endsWith('/node_modules/node-forge'));
  if (!copies.length) throw new Error('Expected node-forge dependency; review removal of the backport');
  for (const [name, metadata] of copies) {
    if (metadata.version !== '1.4.0') throw new Error('Review the node-forge backport before changing its version');
    const file = path.join(root, name, 'lib/rsa.js');
    const content = fs.readFileSync(file);
    const digest = hash(content);
    if (digest === patchedHash) continue;
    if (checkOnly || digest !== originalHash) throw new Error(`Unverified node-forge source: ${name}`);
    const patched = content.toString('utf8').replace(before, after);
    if (hash(patched) !== patchedHash) throw new Error('Unexpected node-forge patch result');
    fs.writeFileSync(file, patched);
  }
  return copies.length;
}

module.exports = { verifyOrPatch };
if (require.main === module) {
  console.log(`Verified CVE-2026-85393 backport in ${verifyOrPatch(process.argv.includes('--check'))} node-forge copy/copies`);
}
