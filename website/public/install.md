# Install Emomo for this Agent

The user asked you to install Emomo. Execute this workflow with your terminal/file tools, rather than merely repeating commands to the user. The current stable version is **0.1.3**.

Scope: install the public CLI and the Emomo Skill for the Agent environment you are currently running in, then verify them. Do not create accounts, request model/infrastructure credentials, deploy services, change the user's selected library or send images to anyone. Follow your host's permission rules. If you lack terminal access, explain that installation needs a terminal-capable Agent; do not claim completion.

## 1. Check the environment

Run `node --version`, `npm --version` and `emomo --version` when available. Node.js >=22.13 is required. If it is absent, use the user's existing Node version manager when available; otherwise report the missing prerequisite without pretending installation succeeded. Do not use sudo or modify unrelated system settings.

## 2. Download and verify the exact release

- Release: https://github.com/timmyagentic/emomo/releases/tag/v0.1.3
- Archive: https://github.com/timmyagentic/emomo/releases/download/v0.1.3/timmyagentic-emomo-cli-0.1.3.tgz
- Checksums: https://github.com/timmyagentic/emomo/releases/download/v0.1.3/SHA256SUMS.txt

Download the archive and checksum file into a new temporary directory using HTTPS. Compare the archive's SHA-256 to the matching filename in SHA256SUMS.txt (`shasum -a 256`, `sha256sum`, Node crypto or PowerShell Get-FileHash). Stop if they differ. Do not blindly execute downloaded shell scripts.

Install the verified local archive:

```sh
npm install --global --ignore-scripts /absolute/path/to/timmyagentic-emomo-cli-0.1.3.tgz
emomo --version
```

Expected version: `0.1.3`. This package has no runtime npm dependencies or install lifecycle scripts. It is distributed through GitHub Release, not the npm registry. If the global prefix is not writable, install to a user-owned prefix and use its explicit bin path; report that path, without silently rewriting the user's shell configuration.

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

## 4. Configure access and verify the public flow

The hosted API and image downloads require an Emomo access key. If no key is available, direct the user to https://emomo.timmyagentic.si/account to sign in with their email and obtain one, saved in a private file. Do not ask them to paste the key into chat. It is separate from model or Cloudflare credentials. If no issued key is available, complete the CLI/Skill install and report public verification as blocked; do not claim success or seek infrastructure secrets. Never print a key or put it in a URL, prompt or shell argument.

If the user provides a private key-file path, configure it:

```sh
emomo auth configure --key-file /absolute/path/to/private-key-file
emomo auth status
```

On macOS/Linux the file must have mode 0600. Configuration binds the key to the official API and does not change the selected catalog. The CLI sends it to the official API and canonical official image downloads only. For the clean verification below, run auth configure with the same temporary EMOMO_CONFIG_DIR first.


Run these commands in a child process with EMOMO_API_TOKEN, EMOMO_API_TOKEN_URL, EMOMO_API_URL, EMOMO_CATALOG and EMOMO_IMAGE_HOSTS removed, and EMOMO_CONFIG_DIR pointing to a fresh temporary directory. Do not modify the parent shell or saved configuration. On POSIX use `env -u NAME` for each variable; on Windows construct a clean child environment. Use the explicit API URL so a saved local catalog does not accidentally pass the public-service test:

```sh
emomo doctor --api-url https://api.timmyagentic.si/agent/v1
emomo search "开心" --limit 3 --api-url https://api.timmyagentic.si/agent/v1
emomo download <an-id-returned-by-search> --dir <new-test-directory> --api-url https://api.timmyagentic.si/agent/v1
```

Check exit codes and JSON `ok`. Verify that the downloaded file exists, its SHA-256 equals the returned value, and download integrity is `verified` (the canonical public SHA-256 matched). Inspect the image if your tools support it. Respect `RATE_LIMITED` / `retryAfterSeconds`; do not loop. The shared service is keyword search, not semantic reasoning; choose imagery based on the actual content and tone.

## 5. Finish with a concrete result

Report the installed version, actual CLI/Skill paths and search/download verification result. Explain whether the host can load the Skill now or requires a new session. If something failed, state the exact unfinished step. Do not substitute an unrelated package or call partial setup complete.

The public static gallery has 7,833 primary assets (7,803 reactions and 30 object stickers); the dataset can change independently of the CLI version. First-frame previews are explicitly marked in their descriptions. Local galleries are optional and remain unchanged. Emomo needs no model keys. Code is MIT; image rights belong to their respective owners.
