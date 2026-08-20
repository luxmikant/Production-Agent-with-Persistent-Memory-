# StillMind — Agentic Memory for a Meditation Agent

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](https://go.dev/)
[![CI](https://github.com/luxmikant/Production-Agent-with-Persistent-Memory-/actions/workflows/ci.yml/badge.svg)](https://github.com/luxmikant/Production-Agent-with-Persistent-Memory-/actions/workflows/ci.yml)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](./CONTRIBUTING.md)

StillMind is a **non-clinical lifestyle meditation coach** built on
**CockroachDB** as the persistent memory layer and **AWS** as the runtime. It
is the single-tenant demo submission for the **CockroachDB × AWS — Build with
Agentic Memory** hackathon.

> **Not medical advice.** StillMind suggests lifestyle methods, gentle
> routines, notes, and schedules. It does not diagnose, treat, or replace
> professional care.

## Why this matters

People want to improve their mental wellbeing, but guidance is generic and
per-session in most apps. StillMind builds a **persistent, evolving memory** of
the user — preferences, notes, sessions, plans — and uses it to personalize
guided practice and weekly plans that survive across days, devices, and
crashes.

**The memory is the product.** CockroachDB holds vectors, facts, checkpoints,
audit, and locks in one system — no external vector store.

## How it works (ACM primitives)

StillMind realizes the agentic memory (ACM) lifecycle as concrete
CockroachDB + AWS components:

| ACM primitive | Realization |
|---|---|
| Architecting | One-shot `_memory_architecture` config row governs every primitive |
| Ingesting | API → SQS → ingest chain (`qualify → extract → resolve → persist`) |
| Scoping | `user_id` prefix column on the CRDB vector index; every query scoped |
| Anticipating | `prefetch_cache` rows with a TTL |
| Compacting | pressure-gated, judge-validated `compactions` (+ scheduled cron pass) |
| Checkpointing | `agent_checkpoints` upserted every turn → resumable runs |
| Locks | `resource_locks` lease via SERIALIZABLE txn (plan generation) |
| Audit | `audit_events` append-only + CloudWatch mirror |

## Tech highlights

- **Distributed vector indexing** — `VECTOR(1024)` + C-SPANN with `user_id` as
  a **prefix column** for tenant isolation, so a query without a constrained
  user cannot use the index.
- **Row-level TTL** on raw utterances and notes with a `@daily` sweep job.
- **Model-agnostic LLM layer** — Amazon Bedrock, or any OpenAI-compatible
  endpoint (`OPENAI_BASE_URL`, e.g. self-hosted Ollama with fully open-source
  models), or a deterministic offline stub.
- **Serverless-ready** — the Go binaries run on Lambda via the AWS Lambda Web
  Adapter; compaction runs on an EventBridge → SQS → Lambda cron.

## Layout

```
.
├── README.md
├── LICENSE                            MIT
├── CONTRIBUTING.md                    contribution guide
├── CODE_OF_CONDUCT.md
├── SECURITY.md                        vulnerability reporting
├── CHANGELOG.md                       release notes
├── SUPPORT.md
├── CITATION.cff                       cite this project
├── docs/
│   ├── architecture.md                ACM lifecycle → CRDB+AWS design
│   ├── acm-lifecycle.md               how each primitive maps to CRDB
│   ├── crdb-features.md               why CRDB for long-horizon memory
│   ├── aws-services.md                why each AWS service is chosen
│   ├── benchmarks.md                  long-horizon eval suite
│   ├── security.md                    safety, privacy, medical disclaimer
│   ├── ops.md                         deploy, scale, runbook
│   ├── sheet-ingestion.md             plan export + Sheets mock backend
│   └── demo-script.md                 <3 min video script
├── backend/                           Go service
│   ├── cmd/stillmind/                 API + agent loop + migrations
│   ├── cmd/compact-http/              compaction endpoint (cron/Lambda)
│   ├── cmd/recall-harness/            benchmark driver
│   ├── internal/agent/                ACM agent loop
│   ├── internal/memory/               CRDB client (vector + lexical)
│   ├── internal/bedrock/              model layer (stub / OpenAI / Bedrock)
│   ├── internal/compaction/           validated compaction
│   ├── internal/plan/                 plan generator + CSV/Sheets export
│   ├── internal/audit/                append-only audit trail
│   ├── internal/api/                  HTTP handlers
│   ├── internal/config/               env config
│   ├── migrations/                    SQL migrations (statement-split runner)
│   └── scripts/                       dev helpers
├── frontend/                          Next.js SPA (static export)
│   ├── app/                           App Router
│   └── lib/                           client SDK
├── infra/
│   ├── docker-compose.yml             local CRDB + backend
│   ├── Dockerfile.backend             container for Lambda/Fargate
│   ├── Dockerfile.frontend
│   ├── cdk-sketch.ts                  (sketch) AWS CDK app
│   └── env.example
├── benchmarks/                        harnesses + sample corpus
└── .github/workflows/                 CI (Go + frontend)
```

## Quick start (local demo)

```bash
# 1. Database (CockroachDB in Docker)
docker compose -f infra/docker-compose.yml up -d cockroachdb

# 2. Migrations (idempotent, statement-split runner)
cd backend
export CRDB_URL="postgresql://root@localhost:26257/stillmind?sslmode=disable"
go run ./cmd/stillmind migrate -dir migrations

# 3. Backend (stub mode = no LLM required; set OPENAI_BASE_URL for real models)
STILLMIND_STUB=1 go run ./cmd/stillmind

# 4. Frontend
cd ../frontend && npm install && npm run dev
```

A single-tenant demo user is provisioned automatically on startup.

## Model configuration

| Mode | Env |
|---|---|
| Offline stub | `STILLMIND_STUB=1` |
| OpenAI-compatible (Ollama / hosted) | `OPENAI_BASE_URL`, `OPENAI_CHAT_MODEL`, `OPENAI_EMBED_MODEL` |
| Amazon Bedrock | `BEDROCK_MODEL`, `BEDROCK_EMBED_MODEL` |
| Shared | `EMBED_DIM=1024`, `LLM_MAX_TOKENS` |

## Running checks

```bash
cd backend && go build ./... && go vet ./...
cd frontend && npm run build
```

## Production deployment

A full walkthrough from an empty AWS account to a live deployment —
CockroachDB Cloud, model serving, Secrets Manager, IAM, S3, SQS, Lambda,
API Gateway, Cognito, edge hosting, observability, and cost guardrails — is in
[`shipping_prod.md`](./shipping_prod.md). Targeted runbooks live under `docs/`.

## License

MIT — see [`LICENSE`](./LICENSE). See [`SUPPORT.md`](./SUPPORT.md) for where to
get help.
