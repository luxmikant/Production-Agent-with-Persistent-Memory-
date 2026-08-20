# Architecture — StillMind

StillMind is a single-tenant, production-shaped meditation agent that runs on AWS and uses CockroachDB as its persistent, evolving memory layer. The architecture directly maps the **Agentic Context Management (ACM)** lifecycle — Architecting, Ingesting, Scoping, Anticipating, Compacting — onto CRDB primitives and AWS services.

## High-level

```
                  ┌─────────────────────────────────────────────┐
                  │              Next.js SPA (browser)         │
                  │  Chat · Plan · Notes · Memory Inspector     │
                  └──────────────┬──────────────────────────────┘
                                 │ HTTPS / JSON
                                 ▼
                       ┌────────────────────┐
                       │  API Gateway       │
                       └──────────┬─────────┘
                                  │
                                  ▼
                 ┌────────────────────────────────────┐
                 │  Backend (Go) on Lambda / ECS      │
                 │  ┌──────────────┐ ┌──────────────┐ │
                 │  │ Agent Loop   │ │ Plan Builder │ │
                 │  └──────┬───────┘ └──────┬───────┘ │
                 │         │     ing/retrieve│tool call │
                 │  ┌──────▼───────┐ ┌──────▼───────┐ │
                 │  │ Memory Client│ │Sheet Exporter│ │
                 │  └──────┬───────┘ └──────────────┘ │
                 │         │                          │
                 │  ┌──────▼───────┐  ┌──────────────┐ │
                 │  │ Compaction   │  │ Audit Logger │ │
                 │  │  Worker      │  └──────────────┘ │
                 │  └──────┬───────┘                   │
                 └─────────┼────────────────────────────┘
                           │
                ┌──────────┴───────────┐
                │                      │
                ▼                      ▼
   ┌────────────────────────┐  ┌────────────────────────┐
   │  CockroachDB Cloud     │  │  Amazon Bedrock        │
   │  (system of record)    │  │  - Claude 3 Haiku      │
   │  - profiles, vectors   │  │  - Titan Embeddings v2 │
   │  - checkpoints, audit  │  └────────────────────────┘
   │  - compactions, locks  │
   │  + MCP + ccloud        │  ┌────────────────────────┐
   └────────────────────────┘  │  Amazon S3             │
                               │  - plans/, audio/      │
                               └────────────────────────┘
```

## Memory lifecycle → CRDB

| ACM primitive | CRDB implementation |
|---|---|
| **Architecting** | `_memory_architecture` row defines per-agent categories, retention, opclass, scopes, compact policy. One-shot design Lambda (or seeded for the demo). |
| **Ingesting** | SERIALIZABLE txns write `notes`, `sessions`, `tool_history`, `audit_events`. Async ingest via SQS → Lambda to keep the user-facing turn fast. |
| **Scoping** | Every query is `WHERE user_id = $1`; vector index has `user_id` as a prefix column → tenant isolation enforced at the index layer. |
| **Anticipating** | Heuristic pre-fetch writes likely context into a `prefetch_cache` table with short TTL; on miss, fall back to synchronous retrieve. |
| **Compacting** | Pressure-triggered (token budget usage ≥ 0.85) with a min interval guard. Category-aware; validated by a judge prompt that scores fact recovery. Raw turns rolled under TTL. |

## The three-call loop

```
retrieved  = memory.Retrieve(userMsg, scope)
compacted  = memory.CompactWindow(scope)         // if pressure ≥ threshold
reply      = bedrock.Chat(assemble(retrieved, compacted, recent))
memory.IngestAsync(turn, scope)                  // returns ingestion_id immediately
```

## Why single tenant for the demo

- Removes auth, multi-user isolation, and quota concerns from the demo.
- Still keeps the schema multi-tenant-ready (`tenant_id`, `user_id` columns, prefix on vector index). Production cut-over is a row-level change, not a redesign.
- All hackathon judging criteria (memory design, technical use, impact, production readiness, creativity) are still demonstrated.

## Compaction strategy

Two guarded triggers:

1. **Memory pressure** — computed as `tokens_in_context / COMPACT_BUDGET_TOKENS`. When ≥ 0.85, fire.
2. **Minimum interval guard** — never compact more often than `COMPACT_MIN_INTERVAL_S`.

Each pass:

- Picks category to compact (e.g. raw session turns).
- Summarizes with category-aware preservation prompt.
- Writes a `compactions` row with `summary_text`, `summary_embedding`, `compression_ratio`, `validation_score`.
- A second prompt judges recovery of key facts. If score < threshold, retry with a less aggressive prompt (fallback to per-turn bullets).
- Applies TTL on raw rows.

This is the validated-compaction regime from the ACM paper: **linear cost, preserved fidelity**.

## Resilience story

- **Agent stack** is persisted via a CRDB checkpoint table on every step. A crashed Lambda is resumed by another Lambda reading the latest checkpoint.
- **Memory** is replicated across CRDB nodes (3× by default). Application-level failover is unnecessary.
- **Audit** uses `AS OF SYSTEM TIME` for "what did the agent believe at time T?" replay.
- **Locks** use a lease column with `expires_at` for multi-agent claims (e.g., prevent two planners from writing the same plan).

## Failure modes & what CRDB solves

| Failure | Mitigation |
|---|---|
| Lambda killed mid-run | Resume from CRDB checkpoint |
| Node crash | CRDB Raft + replicas — no app work |
| AZ loss | Replicas survive; leaseholder moves |
| Long context > budget | Validated compaction |
| Two agents writing same plan | `resource_locks` lease + SERIALIZABLE txn |
| Lost retried write | SERIALIZABLE + client retry on contention error |

## Threat model (short)

- Single-tenant demo, no PII inputs.
- All queries pass through prepared statements parameterized by `user_id`.
- Tool calls are sandboxed; only safe providers (CSV export, plan store) are wired.
- No medical advice; every response is gated by a wellness disclaimer.
- Crisis language triggers a static resource message (not model output).
