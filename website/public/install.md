# Install Emomo for this Agent

The user asked you to install Emomo. Execute this workflow with your terminal/file tools, rather than merely repeating commands to the user. The current version is **1.0.0-beta1**, a prerelease; the previous v1.0.0 release was withdrawn.

Scope: install the public CLI and the Emomo Skill for the Agent environment you are currently running in, then verify them. Do not create accounts, request model/API credentials, deploy services, change the user's selected library or send images to anyone. Follow your host's permission rules. If you lack terminal access, explain that installation needs a terminal-capable Agent; do not claim completion.

## 1. Check the environment

Run `node --version`, `npm --version` and `emomo --version` when available. Node.js >=22.13 is required. If it is absent, use the user's existing Node version manager when available; otherwise report the missing prerequisite without pretending installation succeeded. Do not use sudo or modify unrelated system settings.

## 2. Download and verify the exact release

- Release: https://github.com/timmyagentic/emomo/releases/tag/v1.0.0-beta1
- Archive: https://github.com/timmyagentic/emomo/releases/download/v1.0.0-beta1/timmyagentic-emomo-cli-1.0.0-beta1.tgz
- Checksums: https://github.com/timmyagentic/emomo/releases/download/v1.0.0-beta1/SHA256SUMS.txt

Download the archive and checksum file into a new temporary directory using HTTPS. Compare the archive's SHA-256 to the matching filename in SHA256SUMS.txt (`shasum -a 256`, `sha256sum`, Node crypto or PowerShell Get-FileHash). Stop if they differ. Do not blindly execute downloaded shell scripts.

Install the verified local archive:

```sh
npm install --global --ignore-scripts /absolute/path/to/timmyagentic-emomo-cli-1.0.0-beta1.tgz
emomo --version
```

Expected version: `1.0.0-beta1`. This package has no runtime npm dependencies or install lifecycle scripts. It is distributed through GitHub Release, not the npm registry. If the global prefix is not writable, install to a user-owned prefix and use its explicit bin path; report that path, without silently rewriting the user's shell configuration.

## 3. Install the Skill for this Agent

Choose the current Agent from session context; do not ask the user to choose when it is already clear.

```sh
# Codex
emomo skill install --agent codex
# Claude Code (choose this instead for Claude Code)
emomo skill install --agent claude
# Other Agents supporting ~/.agents/skills
emomo skill install --agent agents
```

Only run the applicable command. A host-specific skill root can be supplied with `--dir /absolute/path/to/skills`. The CLI returns the installed path. It is idempotent when the same Skill is present.

If a different existing Skill is present, the CLI refuses to overwrite it. Preserve it. Compare the existing instructions with the installed package's `skills/emomo/` files; install the new Skill to a separate directory with `--dir` and report the actual path and any host loading step required. Do not label a conflicting/unloaded Skill as active or overwrite user customizations. Never switch or re-import an existing local catalog during installation.

## 4. Verify the actual public flow

Use the explicit API URL so a saved local catalog does not accidentally pass the public-service test:

```sh
emomo doctor --api-url https://api.timmyagentic.si/agent/v1
emomo search "开心" --limit 3 --api-url https://api.timmyagentic.si/agent/v1
emomo download <an-id-returned-by-search> --dir <new-test-directory> --api-url https://api.timmyagentic.si/agent/v1
```

Check exit codes and JSON `ok`. Verify that the downloaded file exists and that its SHA-256 equals the returned value. Inspect the image if your tools support it. Respect `RATE_LIMITED` / `retryAfterSeconds`; do not loop. The shared service is keyword search, not semantic reasoning; choose imagery based on the actual content and tone.

## 5. Finish with a concrete result

Report the installed version, actual CLI/Skill paths and search/download verification result. Explain whether the host can load the Skill now or requires a new session. If something failed, state the exact unfinished step. Do not substitute an unrelated package or call partial setup complete.

The public static gallery has 7,302 primary images. Local galleries are optional and remain unchanged. Emomo needs no model keys. Code is MIT; image rights belong to their respective owners.
