# Changelog

All notable changes to this project are documented here following
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Initial open-source release as a CockroachDB × AWS
"Build with Agentic Memory" hackathon entry.

## [0.1.0] - 2026-08-18

### Added

- **Agentic memory runtime** (`backend/`)
  - CockroachDB-backed persistent memory: profiles, semantic notes (vectors),
    sessions, checkpoints, tool history, audit, plans, locks, prefetch cache,
    compactions, and the `_memory_architecture` config that governs the
    agentic memory (ACM) lifecycle.
  - Distributed vector index on `notes (user_id, embedding)` with `user_id`
    as a prefix column for tenant isolation.
  - Row-level TTL on `raw_utterances` and `notes` with a `@daily` sweep job.
  - Statement-splitting migration runner (`go run ./cmd/stillmind migrate`).
- **Agent loop** (`internal/agent`) implementing ACM primitives
  - Retrieve (vector + lexical, scoped by `user_id`), compact (pressure-gated,
    judge-validated), generate, checkpoint every turn, and append-only audit.
- **Model layer** (`internal/bedrock`)
  - Provider-agnostic `Client` with a deterministic offline stub
    (`STILLMIND_STUB=1`), an OpenAI-compatible HTTP path (`OPENAI_BASE_URL`,
    e.g. self-hosted Ollama or hosted inference), and Amazon Bedrock.
- **Planning & export** — 7-day wind-down plan builder, CSV export, and a mock
  Google Sheets sync endpoint; `resource_locks` lease around plan creation.
- **HTTP API** (`internal/api`) — `/health`, `/chat`, `/plan`, `/plan/csv`,
  `/sheets/sync`, `/memory/recall`, `/memory/notes`, `/memory/audit`,
  `/memory/explain`, with CORS for static hosting.
- **Serverless entrypoints** — `cmd/stillmind` (API), `cmd/compact-http`
  (EventBridge/SQS-triggered compaction), `cmd/recall-harness` (benchmark);
  both binaries run under the AWS Lambda Web Adapter.
- **Demo SPA** (`frontend/`) — Next.js client with notes, recall, audit,
  plan, CSV export, and simulated kill/resume.
- **Local dev** (`infra/`) — `docker-compose.yml` (CRDB + backend), container
  build for Lambda/Fargate, env example.
- **Docs** — architecture, ACM lifecycle mapping, CRDB feature deep-dive, AWS
  service rationale, security, ops runbook, benchmarks, and demo script.
- **Open-source tooling** — MIT license, code of conduct, security policy,
  contributing guide, issue/PR templates, and a CI pipeline (Go build/vet,
  Next.js build).

### Changed

- N/A (initial release).

### Fixed

- N/A (initial release).
