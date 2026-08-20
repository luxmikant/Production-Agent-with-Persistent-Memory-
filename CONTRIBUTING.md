# Contributing to StillMind

Thanks for your interest in StillMind! Contributions — issues, docs, PRs,
and demo feedback — are welcome.

Please read the [Code of Conduct](./CODE_OF_CONDUCT.md) first. This project is
a hackathon demo of **agentic memory on CockroachDB + AWS**; keep changes
focused and demonstrable.

## What we try to do

1. **Memory is the product** — everything durable lives in CockroachDB: notes
   (with vectors), sessions, checkpoints, audit, plans, locks, compactions.
2. **Tenant isolation at the index layer** — every memory query is scoped by
   `user_id`, and the vector index uses `user_id` as a prefix column.
3. **Non-clinical only** — this is lifestyle-grade guidance, never medical
   advice. Crisis language is gated to a static message.
4. **Run cheap** — free/basic CRDB tier, small models, serverless triggers.

## Project layout

```
backend/    Go service (API, compaction, benchmarks, migrations)
frontend/   Next.js demo SPA
infra/      docker-compose, containers, CDK sketch, env example
docs/       architecture, lifecycle, ops, security, benchmarks
benchmarks/ recall harness + sample corpus
```

## Setting up locally

```bash
docker compose -f infra/docker-compose.yml up -d cockroachdb
cd backend
CRDB_URL="postgresql://root@localhost:26257/stillmind?sslmode=disable" \
  go run ./cmd/stillmind migrate -dir migrations
go run ./cmd/stillmind
cd ../frontend && npm install && npm run dev
```

> The fixtures live in the `stillmind` database; point `CRDB_URL` at it after
> migrating (migrations create it and `USE` it internally).

## Model providers

The model layer is provider-agnostic (`internal/bedrock`). Precedence:

1. `STILLMIND_STUB=1` — deterministic offline stub embeddings/chat (no network,
   great for tests/CI).
2. `OPENAI_BASE_URL` — any OpenAI-compatible endpoint (self-hosted Ollama,
   Hugging Face Inference, etc.). Configure with `OPENAI_CHAT_MODEL` and
   `OPENAI_EMBED_MODEL`; the embedding model must match `EMBED_DIM` (1024).
3. Amazon Bedrock — `BEDROCK_MODEL` / `BEDROCK_EMBED_MODEL` (used when
   `OPENAI_BASE_URL` is unset).

## House rules

- **SQL** goes through pgx parameterized queries. Never interpolate user input
  into SQL strings.
- **Embedding dimensions** must match the schema (`VECTOR(1024)`) or a
  migration is required.
- **Migrations** are forward-only and ordered (`0001_init.sql`, `0002_seed.sql`,
  …). Use `IF NOT EXISTS` and `ON CONFLICT DO NOTHING` so the runner is
  idempotent.
- **No secrets in the repo** — use env vars / Secrets Manager; never commit
  `.env`, `.pem`, or connection strings.
- **Commit style** — small conventional-commit messages
  (`feat`, `fix`, `docs`, `chore`, `ci`, `infra`); reference issues when
  present.

## Running checks

```bash
cd backend && go build ./... && go vet ./...
cd frontend && npm run build
```

CI runs these on every push/PR (see `.github/workflows/ci.yml`).

## Opening a PR

1. Fork and branch off `main`.
2. Make a focused change with tests/verification where feasible.
3. Verify the checks above pass locally.
4. Open a PR using the template; keep the description clear about what and why.

Small, clearly-scoped PRs review fastest. If in doubt, open an issue first to
discuss direction.
