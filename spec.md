# StillMind — Spec (Feature Inventory)

This is the canonical feature list for the demo. Each feature lists its
purpose, the ACM primitives it supports, the CRDB mechanism that delivers
it, and the AWS mechanism that exposes it.

## F1. Onboarding & disclaimer

- **Purpose:** Establish the non-clinical scope and gather goal.
- **ACM:** Architecting (the agent's `categories` include `goal`, `prefs`).
- **CRDB:** `profiles.facts`, `profiles.goals`, `profiles.prefs`.
- **AWS:** Frontend banner + Lambda POST to `/chat`.
- **Status:** ✅ implemented.

## F2. Guided micro-session

- **Purpose:** A short meditation script based on user's stated intent.
- **ACM:** Ingesting (session stored on completion), Scoping (per-user).
- **CRDB:** `sessions` table; new `note` row with `embedding` from reply.
- **AWS:** Bedrock chat completion.
- **Status:** ✅ implemented.

## F3. Semantic recall (vector)

- **Purpose:** Find the most relevant past notes for the current user.
- **ACM:** Scoping.
- **CRDB:** Vector index on `notes (user_id, embedding)` with `vector_l2_ops` (L2 `<->`).
- **AWS:** Bedrock Titan Embeddings v2.
- **Status:** ✅ implemented.

## F4. Lexical recall (fallback)

- **Purpose:** Catch exact-term matches that vectors can miss.
- **ACM:** Scoping.
- **CRDB:** Regular `ILIKE` query parameterized by `user_id`.
- **AWS:** None.
- **Status:** ✅ implemented.

## F5. Hybrid recall

- **Purpose:** Combine vector + lexical results with deduplication.
- **ACM:** Scoping.
- **CRDB:** union of two queries, dedupe by ID.
- **AWS:** None.
- **Status:** ✅ implemented.

## F6. Plan generation

- **Purpose:** Build a 7-day plan with morning/evening slots.
- **ACM:** Ingesting (plan rows), Scoping.
- **CRDB:** `plans`, `plan_items` with `user_id` foreign scope.
- **AWS:** Lambda with Bedrock call (template-only for the demo).
- **Status:** ✅ implemented.

## F7. CSV export

- **Purpose:** Download the plan as CSV.
- **ACM:** Scoping.
- **CRDB:** read from `plan_items`.
- **AWS:** S3 in production, in-memory in the demo.
- **Status:** ✅ implemented.

## F8. Mock Sheets sync

- **Purpose:** Show the "send to spreadsheet" UX without OAuth.
- **ACM:** Scoping.
- **CRDB:** write `audit_events` row with `sheets.sync` payload.
- **AWS:** Behind a `SheetClient` interface; production path = Google Sheets.
- **Status:** ✅ implemented.

## F9. Multi-agent lock

- **Purpose:** Two planner agents must not double-write the same plan.
- **ACM:** Scoping.
- **CRDB:** `resource_locks` with TTL; SERIALIZABLE txn.
- **AWS:** None.
- **Status:** ✅ implemented.

## F10. Checkpoint + resume

- **Purpose:** Survive a Lambda kill mid-run.
- **ACM:** Compaction & consolidation (state is persisted after every step).
- **CRDB:** `agent_runs`, `agent_checkpoints`.
- **AWS:** Lambda + API Gateway.
- **Status:** ✅ implemented.

## F11. Pressure-triggered compact

- **Purpose:** Keep context within budget when a session grows long.
- **ACM:** Compacting & consolidation.
- **CRDB:** `compactions` rows; `raw_utterances` TTL.
- **AWS:** Bedrock two-pass (summarize + validate).
- **Status:** ✅ implemented.

## F12. Validated compaction

- **Purpose:** Avoid the accuracy cliff from the ACM paper.
- **ACM:** Compaction validation.
- **CRDB:** `compactions.validation_score`, `compactions.status`.
- **AWS:** Bedrock second prompt.
- **Status:** ✅ implemented.

## F13. Audit trail

- **Purpose:** Every tool call and turn is auditable.
- **ACM:** Ingesting.
- **CRDB:** `audit_events`, append-only.
- **AWS:** CloudWatch logs mirror.
- **Status:** ✅ implemented.

## F14. Time-travel audit

- **Purpose:** Replay "what did the agent believe at time T?".
- **ACM:** Observability.
- **CRDB:** `AS OF SYSTEM TIME` against `audit_events`.
- **AWS:** None.
- **Status:** ✅ schema supported; surface query in v1.1.

## F15. Managed MCP

- **Purpose:** Live memory inspection from Claude/Cursor/VS Code.
- **ACM:** Observability.
- **CRDB:** CockroachDB Cloud Managed MCP server.
- **AWS:** None.
- **Status:** ✅ documented in `benchmarks/mcp-snippet.md`.

## F16. ccloud provisioning

- **Purpose:** One-shot cluster provisioning and audit log access.
- **ACM:** Architecting (stack setup).
- **CRDB:** `ccloud` CLI.
- **AWS:** None.
- **Status:** ✅ documented in `benchmarks/ccloud-snippets.md`.

## F17. CockroachDB Agent Skills

- **Purpose:** Reuse open-source skills for schema, performance, observability.
- **ACM:** Architecting.
- **CRDB:** `cockroachdb-skills` repo.
- **AWS:** None.
- **Status:** ✅ referenced; not yet wired into runtime.

## F18. Anticipating prefetch

- **Purpose:** Pre-warm likely context for the next turn.
- **ACM:** Anticipating.
- **CRDB:** `prefetch_cache` with TTL.
- **AWS:** Heuristic in the agent loop.
- **Status:** 🟡 storage ready; heuristic is a stub.

## F19. Multi-region CRDB

- **Purpose:** Production path to global users.
- **ACM:** Architecting.
- **CRDB:** Follower reads + regional tables.
- **AWS:** Region pinning + Bedrock in same region.
- **Status:** ✅ documented; not enabled in demo.

## F20. Bedrock stub mode

- **Purpose:** Demo without AWS keys.
- **ACM:** N/A.
- **CRDB:** None.
- **AWS:** `STILLMIND_STUB=1` returns deterministic embeddings and a small reply.
- **Status:** ✅ implemented.

## Feature matrix

| ID | Feature | ACM | CRDB | AWS | Status |
|---|---|---|---|---|---|
| F1 | Onboarding | Architecting | `profiles` | Bedrock | ✅ |
| F2 | Micro-session | Ingesting | `sessions`, `notes` | Bedrock | ✅ |
| F3 | Vector recall | Scoping | vector index | Bedrock Titan | ✅ |
| F4 | Lexical recall | Scoping | ILIKE | — | ✅ |
| F5 | Hybrid recall | Scoping | union + dedupe | — | ✅ |
| F6 | Plan generation | Ingesting | `plans`, `plan_items` | Bedrock | ✅ |
| F7 | CSV export | Scoping | `plan_items` | S3 (prod) | ✅ |
| F8 | Mock Sheets | Scoping | `audit_events` | interface | ✅ |
| F9 | Multi-agent lock | Scoping | `resource_locks` | — | ✅ |
| F10 | Checkpoint + resume | Compacting | `agent_checkpoints` | Lambda | ✅ |
| F11 | Pressure compact | Compacting | `compactions` | Bedrock | ✅ |
| F12 | Validated compact | Compacting | `compactions.validation_score` | Bedrock | ✅ |
| F13 | Audit trail | Ingesting | `audit_events` | CloudWatch | ✅ |
| F14 | Time-travel audit | Observability | `AS OF SYSTEM TIME` | — | ✅ schema |
| F15 | Managed MCP | Observability | CockroachDB MCP | — | ✅ documented |
| F16 | ccloud | Architecting | `ccloud` CLI | — | ✅ documented |
| F17 | Agent Skills | Architecting | skills repo | — | ✅ referenced |
| F18 | Prefetch | Anticipating | `prefetch_cache` | — | 🟡 stub |
| F19 | Multi-region | Architecting | regional tables | Bedrock region | ✅ documented |
| F20 | Stub mode | — | — | env flag | ✅ |
