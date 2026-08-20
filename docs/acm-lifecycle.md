# ACM Lifecycle — How each primitive maps to CockroachDB

StillMind implements the five ACM primitives purely on **CRDB + AWS**, no external vector store, no graph DB, no Redis.

## 1. Architecting

A `_memory_architecture` row defines, per agent type, what to remember and how:

```sql
CREATE TABLE _memory_architecture (
  agent_id        STRING PRIMARY KEY,
  categories      JSONB NOT NULL,    -- ["profile", "note", "session", "tool", "plan"]
  retention_days  JSONB NOT NULL,    -- {"note": 90, "session": 365}
  embed_dim       INT NOT NULL,
  compact_policy  JSONB NOT NULL,    -- {"budget_tokens": 4000, "pressure": 0.85}
  scopes          JSONB NOT NULL,    -- ["user_id", "tenant_id"]
  created_at      TIMESTAMPTZ DEFAULT now()
);
```

A one-shot "memory architect" Lambda or seeded config is run before the agent goes live. Every downstream primitive reads this row.

## 2. Ingesting

SERIALIZABLE transactions on writes. Each ingestion is a single txn:

```sql
BEGIN;
  INSERT INTO notes (user_id, kind, body, embedding, ts) VALUES (...);
  INSERT INTO audit_events (user_id, type, payload, ts) VALUES (...);
  UPDATE profiles SET facts = facts || $new WHERE user_id = $1;
COMMIT;
```

Raw utterance rows are kept under TTL; promoted facts live longer. The agent turn itself returns an `ingestion_id` immediately — the heavier work is queued via SQS.

## 3. Scoping

Every retrieval statement is parameterized:

```sql
SELECT id, body, embedding <-> $1 AS distance
FROM notes
WHERE user_id = $2
ORDER BY embedding <-> $1 ASC
LIMIT 5;
```

The vector index is declared with `user_id` as a prefix column:

```sql
CREATE VECTOR INDEX notes_user_embed_idx
  ON notes (user_id, embedding);
```

Per the CRDB docs, prefix columns are required to be constrained to specific values for the index to be used. This naturally enforces isolation at the index layer.

## 4. Anticipating

A small table holds pre-fetched context with short TTL:

```sql
CREATE TABLE prefetch_cache (
  user_id      STRING,
  intent_key   STRING,
  context      JSONB,
  embedding    VECTOR(1024),
  expires_at   TIMESTAMPTZ,
  PRIMARY KEY (user_id, intent_key)
);
```

A heuristic — derived from the agent's recent tool history and time of day — writes likely context. On the next turn, the cache is checked first; on miss, the synchronous retrieval path runs.

## 5. Compacting & consolidation

Pressure-triggered. Category-aware summarization prompt. Validation by a second prompt that probes for fact preservation. All in one txn:

```sql
BEGIN;
  INSERT INTO compactions (user_id, kind, summary_text, summary_embedding,
                           compression_ratio, validation_score, ts) VALUES (...);
  UPDATE raw_utterances SET ttl_until = now() WHERE id = ANY($1);
COMMIT;
```

Row-level TTL is configured on `raw_utterances` with
`ttl_expiration_expression = 'ttl_until'` — the daily `@daily` TTL job deletes
the expired raw rows, so compaction sets the expiry and CRDB does the cleanup.

If `validation_score < 0.9`, the worker retries with a less aggressive prompt. If retries exceed `max_attempts`, the compaction is partially preserved with a manual-flag row.

## Trade-offs vs. the paper's reference stack

| Paper | StillMind |
|---|---|
| Chroma + Neo4j + Postgres + Redis | **CRDB only** (vector + relational + SQS queues) |
| Locally hosted ST models | **Bedrock** (managed) |
| Proprietary anticipator | Heuristic pre-warm + short TTL |
| Proprietary validation judge | Two-prompt open judge |

The wins we keep: SERIALIZABLE across all state, prefix-isolated vectors, validated compaction, multi-region option, MCP/ccloud integration for the demo.

The thing we lose: explicit graph. We compensate with structured JSONB relations and "last-N facts" recall that is sufficient for a lifestyle coach.
