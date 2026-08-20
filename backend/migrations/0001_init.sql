-- StillMind schema. CockroachDB 25.2+.
-- Run order: 0001_init.sql

SET CLUSTER SETTING feature.vector_index.enabled = true;

CREATE DATABASE IF NOT EXISTS stillmind;
USE stillmind;

-- ---- Architecting --------------------------------------------------

CREATE TABLE IF NOT EXISTS _memory_architecture (
  agent_id        STRING PRIMARY KEY,
  categories      JSONB NOT NULL,
  retention_days  JSONB NOT NULL,
  embed_dim       INT NOT NULL,
  compact_policy  JSONB NOT NULL,
  scopes          JSONB NOT NULL,
  created_at      TIMESTAMPTZ DEFAULT now()
);

INSERT INTO _memory_architecture (agent_id, categories, retention_days, embed_dim, compact_policy, scopes)
VALUES (
  'stillmind.coach',
  jsonb_build_object(
    'list', jsonb_build_array('profile','note','session','tool','plan','fact')
  ),
  jsonb_build_object(
    'note', 90, 'session', 365, 'tool', 30, 'plan', 90, 'fact', 730, 'raw_utterance', 7
  ),
  1024,
  jsonb_build_object(
    'budget_tokens', 4000,
    'pressure_threshold', 0.85,
    'min_interval_s', 60,
    'validation_threshold', 0.9,
    'max_attempts', 3
  ),
  jsonb_build_object('list', jsonb_build_array('tenant_id','user_id'))
)
ON CONFLICT (agent_id) DO NOTHING;

-- ---- Profiles ------------------------------------------------------

CREATE TABLE IF NOT EXISTS profiles (
  user_id        STRING PRIMARY KEY,
  display_name   STRING,
  facts          JSONB NOT NULL DEFAULT '{}'::jsonb,
  goals          JSONB NOT NULL DEFAULT '[]'::jsonb,
  prefs          JSONB NOT NULL DEFAULT '{}'::jsonb,
  updated_at     TIMESTAMPTZ DEFAULT now()
);

-- ---- Notes (semantic memory) --------------------------------------

CREATE TABLE IF NOT EXISTS notes (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       STRING NOT NULL,
  kind          STRING NOT NULL,            -- 'note' | 'fact' | 'reflection'
  body          STRING NOT NULL,
  embedding     VECTOR(1024),
  source        STRING,                    -- 'chat' | 'ingest' | 'compact'
  ttl_until     TIMESTAMPTZ,
  created_at    TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS notes_user_ts_idx ON notes (user_id, created_at DESC);

-- Distributed vector index with user_id as prefix column.
-- Per CRDB docs, prefix columns must be constrained to a specific value.
CREATE VECTOR INDEX IF NOT EXISTS notes_user_embed_idx
  ON notes (user_id, embedding);

-- ---- Sessions ------------------------------------------------------

CREATE TABLE IF NOT EXISTS sessions (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       STRING NOT NULL,
  method        STRING NOT NULL,             -- 'box_breath','body_scan','walk'
  duration_min  INT NOT NULL,
  mood_pre      STRING,
  mood_post     STRING,
  script_ref    STRING,
  started_at    TIMESTAMPTZ DEFAULT now(),
  ended_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS sessions_user_started_idx ON sessions (user_id, started_at DESC);

-- ---- Agent runs / checkpoints --------------------------------------

CREATE TABLE IF NOT EXISTS agent_runs (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       STRING NOT NULL,
  status        STRING NOT NULL DEFAULT 'active', -- 'active'|'done'|'failed'
  started_at    TIMESTAMPTZ DEFAULT now(),
  ended_at      TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS agent_checkpoints (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id        UUID NOT NULL REFERENCES agent_runs(id),
  step          INT NOT NULL,
  state         JSONB NOT NULL,
  tokens_used   INT NOT NULL,
  ts            TIMESTAMPTZ DEFAULT now(),
  UNIQUE (run_id, step)
);

CREATE INDEX IF NOT EXISTS agent_checkpoints_run_step_idx
  ON agent_checkpoints (run_id, step DESC);

-- ---- Tool history --------------------------------------------------

CREATE TABLE IF NOT EXISTS tool_history (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       STRING NOT NULL,
  run_id        UUID,
  tool          STRING NOT NULL,
  args          JSONB,
  result        JSONB,
  ts            TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS tool_history_user_ts_idx ON tool_history (user_id, ts DESC);

-- ---- Audit (append-only) ------------------------------------------

CREATE TABLE IF NOT EXISTS audit_events (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       STRING,
  type          STRING NOT NULL,
  payload       JSONB,
  ts            TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS audit_events_ts_idx ON audit_events (ts DESC);

-- ---- Plans ---------------------------------------------------------

CREATE TABLE IF NOT EXISTS plans (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       STRING NOT NULL,
  title         STRING NOT NULL,
  created_at    TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS plan_items (
  plan_id       UUID NOT NULL REFERENCES plans(id),
  day_offset    INT NOT NULL,
  slot          STRING NOT NULL,    -- 'morning'|'midday'|'evening'
  method        STRING NOT NULL,
  duration_min  INT NOT NULL,
  notes         STRING,
  PRIMARY KEY (plan_id, day_offset, slot)
);

-- ---- Locks (multi-agent claims) -----------------------------------

CREATE TABLE IF NOT EXISTS resource_locks (
  resource      STRING NOT NULL,
  user_id       STRING NOT NULL,
  owner         STRING NOT NULL,
  expires_at    TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (resource, user_id)
);

-- ---- Prefetch cache -----------------------------------------------

CREATE TABLE IF NOT EXISTS prefetch_cache (
  user_id       STRING NOT NULL,
  intent_key    STRING NOT NULL,
  context       JSONB NOT NULL,
  embedding     VECTOR(1024),
  expires_at    TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (user_id, intent_key)
);

-- ---- Compactions ---------------------------------------------------

CREATE TABLE IF NOT EXISTS compactions (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id           STRING NOT NULL,
  kind              STRING NOT NULL,
  input_tokens      INT NOT NULL,
  output_tokens     INT NOT NULL,
  compression_ratio FLOAT NOT NULL,
  validation_score  FLOAT NOT NULL,
  summary_text      STRING NOT NULL,
  summary_embedding VECTOR(1024),
  raw_ids           JSONB,
  attempt           INT NOT NULL DEFAULT 1,
  status            STRING NOT NULL DEFAULT 'ok', -- 'ok'|'retried'|'failed'
  ts                TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS compactions_user_ts_idx ON compactions (user_id, ts DESC);

-- ---- Raw utterances (TTL'd after compact) -------------------------

CREATE TABLE IF NOT EXISTS raw_utterances (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       STRING NOT NULL,
  role          STRING NOT NULL,           -- 'user'|'assistant'|'tool'
  body          STRING NOT NULL,
  run_id        UUID,
  ttl_until     TIMESTAMPTZ,
  ts            TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS raw_utterances_user_ts_idx
  ON raw_utterances (user_id, ts DESC);

-- ---- Row-level TTL: per-row expiry via ttl_until (NULL = keep) -----
ALTER TABLE raw_utterances SET (ttl_expiration_expression = 'ttl_until', ttl_job_cron = '@daily');
ALTER TABLE notes SET (ttl_expiration_expression = 'ttl_until', ttl_job_cron = '@daily');
