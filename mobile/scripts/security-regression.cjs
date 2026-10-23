const assert = require('node:assert/strict');
const { constants, createHash, generateKeyPairSync, privateEncrypt } = require('node:crypto');
const { test } = require('node:test');
const path = require('node:path');
const forge = require('node-forge');

const keys = generateKeyPairSync('rsa', {
  modulusLength: 1024,
  publicKeyEncoding: { type: 'spki', format: 'pem' },
  privateKeyEncoding: { type: 'pkcs8', format: 'pem' },
});
const publicKey = forge.pki.publicKeyFromPem(keys.publicKey);
const digest = createHash('sha256').update('Emomo certificate regression').digest('latin1');

function signature(extraElement, withNull = true) {
  const { asn1 } = forge;
  const node = (type, constructed, value) => asn1.create(asn1.Class.UNIVERSAL, type, constructed, value);
  const algorithm = [node(asn1.Type.OID, false, asn1.oidToDer(forge.pki.oids.sha256).getBytes())];
  if (withNull) algorithm.push(node(asn1.Type.NULL, false, ''));
  if (extraElement) algorithm.push(node(asn1.Type.OCTETSTRING, false, 'unconsumed garbage'));
  const info = node(asn1.Type.SEQUENCE, true, [
    node(asn1.Type.SEQUENCE, true, algorithm),
    node(asn1.Type.OCTETSTRING, false, digest),
  ]);
  return privateEncrypt({ key: keys.privateKey, padding: constants.RSA_PKCS1_PADDING },
    Buffer.from(asn1.toDer(info).getBytes(), 'latin1')).toString('latin1');
}

test('RSA verification accepts ordinary SHA-256 DigestInfo', () => {
  assert.equal(publicKey.verify(digest, signature(false)), true);
  assert.equal(publicKey.verify(digest, signature(false, false)), true);
});

test('RSA verification rejects extra nested DigestAlgorithm elements (CVE-2026-85393)', () => {
  assert.throws(() => publicKey.verify(digest, signature(true)), /DigestInfo/);
});

test('Metro can still read PNG dimensions with the image-size security update', () => {
  const metro = path.dirname(require.resolve('metro/package.json'));
  const { getAssetSize } = require(path.join(metro, 'src/Assets.js'));
  const image = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aX1cAAAAASUVORK5CYII=', 'base64');
  assert.deepEqual(getAssetSize('png', image, 'pixel.png'), { width: 1, height: 1 });
});
