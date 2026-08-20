# Security & safety

StillMind is a lifestyle coach. It is **not** medical advice.

## Legal & product boundaries

- Top-level disclaimer shown on every chat first reply.
- No diagnosis, no treatment language, no claims of cure.
- Crisis language → static message with hotlines; never model output.
- No export of medical identifiers. Names, emails, and dates are stripped from notes.

## Technical controls

- All SQL is parameterized via prepared statements.
- Per-user `user_id` is enforced at every query; the vector index has `user_id` as a prefix column.
- No cross-tenant scans. Integration tests assert that user A's retrieval never returns user B's notes.
- Tool calls are bounded to a fixed allowlist (`export_plan`, `mark_session_done`).
- Compact and ingest pipelines run in separate IAM roles from the public-facing API.

## Compliance posture

- Demo: no PII, no PHI, no GDPR data subject.
- Production path: GDPR data subject export (dump `notes`, `profiles`, `audit_events` by `user_id`) and delete (TTL + hard delete) — both already supported by the schema.

## Threat model

- Single-tenant demo runs on a private DB user with table-level grants.
- Public-facing Lambda has read/write only on `notes`, `sessions`, `tool_history`, `audit_events`, `plans`, `agent_checkpoints`, `prefetch_cache`.
- No DDL or admin permissions for the runtime.

## Audit

- Every tool call writes an `audit_events` row.
- `AS OF SYSTEM TIME` lets reviewers replay "what did the agent believe at time T".
- Audit table is append-only; no UPDATE/DELETE permissions for the runtime role.
