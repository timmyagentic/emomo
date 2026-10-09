# Deterministic Agent search

This independent Worker serves the canonical Emomo protobuf REST DTOs from a D1 text index. It must never call a model, proxy the legacy backend, use an AI binding, or make an outbound fetch. Do not add automatic OCR, captioning, embeddings, query expansion, reranking or a model fallback. User Agents do that work in their own sessions. Existing images remain in R2 and are downloaded directly by the CLI.

`gen/` is synchronized generated output from the canonical backend proto schemas (via `frontend/gen/`); use `npm run gen:sync` after canonical regeneration. No competing HTTP DTOs. Internal index input and D1 table shapes are not HTTP contracts. These migrations belong only to the new D1 read model; do not change the legacy PostgreSQL schema/data or its GORM migration ownership.

Run `npm run check`. The integration tests use real local workerd and D1, block and count outbound HTTP, and invoke the CLI. Test data is a fixture, not proof of actual library coverage. Keep metadata dumps and generated import SQL outside Git. No automatic remote provisioning, import, deployment, domain cutover or package publishing. Production is explicitly enabled at api.timmyagentic.si. Treat its D1 and R2 data as production; use local defaults for validation. Future production changes require user authorization.
