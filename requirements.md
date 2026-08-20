# StillMind — Requirements

## R1. Functional

- **R1.1** The agent must chat in a non-clinical, calm tone and never diagnose or treat.
- **R1.2** The agent must refuse to provide medical advice and surface a crisis hotline if crisis language is detected.
- **R1.3** Each user message must produce a reply under 1.5 s median latency on local docker-compose.
- **R1.4** The agent must persist profile facts, semantic notes, and sessions to CockroachDB.
- **R1.5** Each turn must retrieve at least one relevant memory item when present.
- **R1.6** The agent must generate a 7-day plan with morning/evening slots on demand.
- **R1.7** Plans must be exportable as CSV.
- **R1.8** Plans must be synchronizable to a Sheets-like target (mock in demo).
- **R1.9** The agent must compact long sessions when token pressure ≥ 0.85 of budget.
- **R1.10** Compaction must validate fact-preservation; abort or retry if score < 0.9.
- **R1.11** The agent must checkpoint after every turn for resume.
- **R1.12** A mid-run kill must not lose the latest committed state.
- **R1.13** Multi-agent claims must be prevented by a lease lock on plan creation.
- **R1.14** Every tool call must write an `audit_events` row.
- **R1.15** Time-travel queries via `AS OF SYSTEM TIME` must work against `audit_events`.

## R2. Non-functional

- **R2.1** All SQL must be parameterized; no string concatenation.
- **R2.2** Scope isolation must be enforced at the index layer (`user_id` prefix column).
- **R2.3** The system must tolerate a single CRDB node failure without app-level failover.
- **R2.4** Compaction must keep context within the configured token budget.
- **R2.5** Anthropic Bedrock model must be pinned to `claude-3-haiku` (cost tier).
- **R2.6** Embedding model must be pinned to `amazon.titan-embed-text-v2` (cost tier).
- **R2.7** Frontend must be a Next.js SPA with the `/api` rewrite proxying to the backend.
- **R2.8** Backend must be deployable both as a container and as a Lambda function.
- **R2.9** All runtime configuration must come from env vars (no hardcoded secrets).
- **R2.10** The system must run end-to-end with `STILLMIND_STUB=1` for offline demo.

## R3. Hackathon

- **R3.1** ≥ 2 CRDB tools demonstrated: Managed MCP Server + Distributed Vector Indexing (+ ccloud + Agent Skills).
- **R3.2** ≥ 1 AWS service demonstrated: Bedrock + Lambda + S3.
- **R3.3** Public repo with MIT license.
- **R3.4** Public demo URL.
- **R3.5** ≤ 3-minute demo video uploaded publicly.
- **R3.6** README identifies which CRDB tools and AWS services are used.

## R4. Security

- **R4.1** Crisis language must short-circuit to a static message (no model output).
- **R4.2** No cross-user scope leakage, verified by negative tests.
- **R4.3** No PII collected in the demo.
- **R4.4** Audit events are append-only for the runtime role.

## R5. Operational

- **R5.1** `docker compose up` brings up the full stack.
- **R5.2** `ccloud` commands are documented for cluster provisioning.
- **R5.3** Compaction can be disabled by env var and re-enabled without code changes.
- **R5.4** Logs are structured JSON.
- **R5.5** Health check endpoint exists at `/health`.
