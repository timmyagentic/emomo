# Mobile dependency security maintenance

The app stays on Expo SDK 54 and React Native 0.81.5. The lockfile refresh includes compatible SDK/tooling patches and shell-quote 1.12.0. `image-size` is pinned to 2.0.3; the Metro buffer-based PNG path is covered by a compatibility test and Android JS export.

## Temporary node-forge backport

As checked on 2026-10-23, node-forge 1.4.0 is the latest npm release and remains affected by [CVE-2026-85393 / GHSA-86w9-cpqp-85rv](https://github.com/advisories/GHSA-86w9-cpqp-85rv). npm's force fix would downgrade Expo to 44.x. The retained SDK 54 tooling needs node-forge, so the dependency stays at its published version with a small source backport equivalent to [upstream proposal #1152](https://github.com/digitalbazaar/forge/pull/1152). That proposal is not yet merged or released.

`npm ci` runs `scripts/patch-node-forge.cjs`. It verifies the exact original RSA module SHA-256, adds the missing nested DigestAlgorithm element-count check, and verifies the resulting SHA-256. Every node-forge copy in the lockfile must be 1.4.0; unknown versions/content fail closed. Repeated installation/checking is idempotent. Skipping lifecycle scripts leaves this backport unapplied and is caught by the security checks.

Run:

```sh
npm run test:security
npm run audit:security
```

The RSA regression proves ordinary SHA-256 signatures with and without NULL parameters still verify, while an otherwise signed DigestInfo containing extra nested garbage is rejected. The same malformed input was accepted before the patch.

**Raw npm audit remains visible:** four High package entries trace to this one unpublished node-forge fix (node-forge, @expo/code-signing-certificates, @expo/cli and expo). They are not claimed to be a clean upstream npm audit. `audit:security` verifies the patched source before allowing only this specific advisory; every other High/Critical advisory fails the check. CI runs both checks after installation.

When an upstream fixed version is published, review its advisory and implementation, update the dependency, remove the temporary postinstall backport and its audit exception, then rerun the RSA, Metro, application, Expo and export checks. No runtime credentials, application API behavior or product release version is changed here.
