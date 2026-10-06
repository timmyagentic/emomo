# Emomo CLI

The CLI is an installable, dependency-free Node.js 22+ client of the existing REST API. Agent output is one JSON envelope on stdout, including failures; exit codes are 0/1. Keep `schemaVersion`, error codes, and candidate fields stable.

The canonical backend proto schemas own HTTP DTOs. This client sends their existing snake_case fields and projects their responses into a compact Agent view; do not introduce a competing schema or expose infrastructure secrets. The default endpoint is `/agent/v1`; the new independent text-search Worker never calls cloud models and keeps old web/mobile `/api/v1` unavailable. The zero-model guarantee does not apply to arbitrary custom API instances.

Run `npm run check` and `npm test`. Tests use local protocol fixtures, including actual npm pack/install and image bytes. They are not proof of production search availability. Validate substantive skill changes with the available skill frontmatter validator and review the actual CLI workflow.

Keep downloaded images and install tests outside the repository. Never overwrite a user's existing skill or image. Do not add install lifecycle scripts, automatic service startup, production deployments, or package publication to this client.
