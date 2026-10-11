# Secret-free development

Use a clean Linux or macOS checkout with Node.js >=22.13 and npm on PATH. Node 22 LTS is the CI baseline; Node 24 is also supported by the CLI. `full` additionally needs a Go launcher capable of downloading Go 1.26.2, Git and Bash. Installation needs access to the npm registry and (for full) the Go module proxy/toolchain download service. No Cloudflare login, account token, database URL, model key, private catalog or image library is needed.

From the repository root:

```sh
node scripts/dev.mjs bootstrap agent
node scripts/dev.mjs check agent
```

These are suitable setup and verification commands for a disposable cloud environment. Setup installs the locked search dependencies with `npm ci`; it does not launch a server. The dependency-free CLI needs no global installation. Repeating setup reinstalls node_modules from the lockfile. Native workerd/esbuild installation scripts are required; do not use `--ignore-scripts` here.

| Profile | Setup | Verification |
| --- | --- | --- |
| `agent` (default) | Search Worker dependencies | Bootstrap contract tests; CLI syntax and fixture tests including package installation; generated search DTO sync; Worker types; real local workerd/D1, CLI integration and outbound-blocking tests; Wrangler deployment **dry-run** |
| `full` | Agent plus legacy gateway, frontend, mobile and Go modules | Agent plus legacy gateway tests/types, Go tests/build, frontend lint/build, mobile generation and clean generated diff, tests/types/lint, Expo doctor and mobile CI wiring |

```sh
node scripts/dev.mjs bootstrap full
node scripts/dev.mjs check full
```

The full check mirrors `.github/workflows/required-ci.yml`; that workflow remains the merge gate. Expo doctor is the existing online diagnostic, fetched by `npx`; its latest version and registry/service availability can affect a full run. Bootstrap installs dependencies only; run check explicitly after code changes. For a cached environment, rerun bootstrap when lockfiles change.

The scripts set tool-specific caches/config/log locations under ignored `.dev-cache/`, including npm, Wrangler and Go. Go's persistent user configuration is disabled with `GOENV=off` (including macOS, where XDG does not relocate it). The `full` check and backend CI exercise this with a temporary nonsecret configuration and the real Go executable, without reading or writing the user's Go configuration. They isolate HOME/USERPROFILE, disable default Go authentication with `GOAUTH=off`, disable global/system Git config and Git hooks/signing, and pass only basic process paths and session proxy/CA settings to subprocesses, suppress telemetry, and do not inherit service credentials or caller Node options. Local `.env*` / `.dev.vars*` files (except `.example`) in participating project directories and Worker ancestors are rejected by filename without reading their contents. Project `.npmrc` files are rejected as well; npm user/global config is redirected to empty paths. Legacy Wrangler configuration is unreachable through the isolated HOME. Run from a clean checkout; do not copy private environment files, snapshots, images, import SQL or a local Emomo catalog into the environment. This entry point is not a general sandbox for arbitrary dependency code.

No setup/check command provisions resources, imports production data, deploys a Worker, changes user configuration, publishes a package or starts the legacy model services. The website is static and needs no dependency install or build; browser layout, copy/install links and image behavior require separate manual verification. Mobile checks do not build/sign an iOS or Android application. Frontend checks do not run browser E2E. Production search availability, release assets, native UI and real private-library coverage are outside this test contract.

For a cloud acceptance record, capture the checkout SHA, OS, `node --version`, `npm --version`, selected profile, setup exit status and check exit status. Record real Linux cloud execution separately from local macOS and GitHub Actions. A local pass or an environment saved in the UI alone does not establish cloud cold-start success.
