# Demo script — < 3 minutes

## 0:00 — Title card

> StillMind — built on CockroachDB and AWS. A meditation agent that remembers.

## 0:10 — Onboarding

- Open the demo URL.
- Disclaimer visible.
- Quick "What's your goal?" chip selector: sleep, focus, calm.

## 0:25 — Guided micro-session

- Agent responds with a 3-minute box-breathing script.
- Visible: "Saving session, tool history, audit event."

## 0:55 — Memory recall

- "What did I notice last time about evenings?"
- Side panel shows the **vector recall** (L2 Euclidean) and the **lexical hits**.
- "This is one CockroachDB index with a `user_id` prefix column."

## 1:20 — Compact

- "Long sessions use a linear-cost compact."
- Show raw turns → rolled summary → TTL on raw.

## 1:45 — Resilience

- Lambda icon flashes "killed mid-run."
- "Resumed from the last checkpoint in CRDB."
- Output continues the plan.

## 2:15 — Plan export

- "Generate a 7-day plan."
- "Export to CSV" → real download.
- "Sync to Sheets" → mock sheet panel renders the same rows.

## 2:40 — Live memory inspection

- Open the MCP server (read-only) in the README.
- `dbtool list tables` → return StillMind memory tables.
- `dbtool explain SELECT ... ORDER BY embedding <-> $1` → show vector index pick.

## 2:55 — Closing

> The agent and the database aren't two systems. The database *is* the agent's memory.
