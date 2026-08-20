# CockroachDB — Why it fits long-horizon agent memory

Fault tolerance is delivered by CRDB itself. The agent doesn't write FT code.

## Resilience model (handled by CRDB)

- **Raft consensus** with ≥ 3 replicas by default per range.
- **Multi-active availability** — any node accepts reads/writes on the data it owns.
- **Automatic rebalancing** — ranges split and re-replicate when nodes fail or join.
- **Multi-region deployment** with regional tables or follower reads.
- **Self-healing** — replicas reconcile, leases transfer, no manual failover.

## Features we use

| Feature | Use in StillMind |
|---|---|
| `VECTOR` + C-SPANN distributed index | Note recall with prefix isolation |
| `vector_l2_ops` (default) | L2 Euclidean distance for semantic memory |
| SERIALIZABLE isolation | Safe multi-agent writes, no lost updates |
| JSONB | Flexible memory categories, plan structure, tool args |
| Row-level TTL | Auto-expire raw turns after compact |
| `AS OF SYSTEM TIME` | Replay what the agent believed at time T |
| Materialized views | Optional — per-user memory dashboards |
| Changefeeds | Future: stream memory events to analytics |
| `_crdb_internal` + MCP | Live memory inspection for the demo |
| ccloud CLI | One-shot provisioning, audit log access |
| Agent Skills | Schema/performance/observability patterns |

## Vector specifics

- Index: `CREATE VECTOR INDEX ON notes (user_id, embedding);`
- `user_id` is a prefix column — required to be constrained in the WHERE clause for the index to be used.
- `vector_l2_ops` is the default opclass — gives us `<->` L2 Euclidean distance.
- Partition and beam sizes tuned for ~10k–100k vectors per user (the realistic ceiling for a personal coach).

## Index acceleration is automatic

When `user_id` is fixed in the query, the planner selects the vector index. `EXPLAIN` shows `vector search` with `prefix spans: [/user123 - /user123]`.

## What we don't need to do

- No app-level failover.
- No retry-on-write-failure in steady state (CRDB handles Raft transient retries internally).
- No separate driver for vectors — same `pgx` connection.
- No ETL between vector store and operational DB.

## Limits we acknowledge

- Vector index backfill blocks writes on the table. We create it on empty tables and add new rows incrementally.
- `IMPORT INTO` is not supported on vector-indexed tables. We insert via parameterized SQL.
- L1 / Hamming / Jaccard distance aren't implemented — we use L2 only.
