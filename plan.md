# StillMind — Project Plan

## 1. Goal

Ship a single-tenant demo of **StillMind**, a non-clinical lifestyle meditation coach with **persistent, evolving agentic memory** powered by **CockroachDB** and **AWS**, for the **CockroachDB × AWS — Build with Agentic Memory** hackathon.

The submission must demonstrate that the database is the agent's memory — not a bolt-on — and that the entire system runs on AWS.

## 2. Scope

**In scope (demo)**

- Meditation coach chat (lifestyle only, no medical advice)
- Persistent memory: profile, semantic notes, sessions, plans, audit, checkpoints, locks
- L2 Euclidean vector recall on a per-user prefix index
- Lexical + vector hybrid retrieval
- Pressure-triggered, validated compaction of long sessions
- Plan generation + CSV export + mock Sheets sync
- Resilience: mid-run checkpoint + resume
- Live memory inspection via CockroachDB Managed MCP

**In scope (production path, documented)**

- Multi-tenant extension (column + prefix already in place)
- Real Sheets ingestion behind the same `SheetClient` interface
- Multi-region CRDB cluster via `ccloud`

**Out of scope**

- Medical triage, diagnosis, treatment
- Real OAuth, payments, multi-user production data
- Wearables, calendar sync

## 3. Stakeholders

| Role | Person / System | Concern |
|---|---|---|
| Hackathon judge | CockroachDB × AWS | Memory design, technical use, impact, production readiness, creativity |
| Demo user | Browser user | Helpful, calm, safe coaching |
| GitHub Actions | CI | Build, vet, lint |
| CockroachDB Cloud | Memory system of record | HA, vectors, txn semantics |
| AWS Bedrock | Reasoning + embeddings | Cost-efficient, low-ops |

## 4. Milestones

| # | Milestone | Deliverable |
|---|---|---|
| M0 | Spec sign-off | `plan.md`, `requirements.md`, `spec.md` |
| M1 | Schema + migrations | `0001_init.sql`, `0002_seed.sql` |
| M2 | Backend skeleton | Go agents, memory, bedrock, compaction, plan, api |
| M3 | Frontend SPA | Next.js chat, plan, recall, audit |
| M4 | Infra | docker-compose, env example, CDK sketch |
| M5 | Benchmarks | recall harness, MCP snippet, ccloud snippets |
| M6 | Production shipping | `shipping_prod.md` with runbooks |
| M7 | Submission | repo, video, demo URL, hackathon checklist |

## 5. Architecture summary

```
Next.js SPA ─► API Gateway ─► Lambda (Go) ─► CockroachDB (system of record)
                                              ▲
                                              │
                                         Bedrock (Haiku + Titan v2)
                                              │
                                         S3 (artifacts)
```

## 6. Risks

| Risk | Mitigation |
|---|---|
| Bedrock quota or cost | `STILLMIND_STUB=1` for offline demo; cheapest models pinned |
| Vector index backfill blocks writes | Create index on empty table; insert incrementally |
| Long sessions blow context | Pressure-triggered, validated compaction |
| Misuse as medical advice | Disclaimers + crisis string gating |
| AWS region limits Bedrock | Choose `us-east-1` (broadest Bedrock support) |

## 7. Acceptance criteria

- `curl localhost:8080/health` → `{"ok":true,"service":"stillmind"}`
- All migrations apply cleanly against CRDB v25.2
- `go build ./...` and `npm run build` succeed
- `/chat` returns a tail of 4-7-8 breath cue for a "calm me" message
- `/plan` returns a 7-day plan and `/plan/csv` returns a downloadable CSV
- `/memory/recall` returns at least one note for a phrase already inserted
- `/memory/explain` shows vector index usage in the EXPLAIN plan
- Demo URL renders chat, plan, recall, audit panels
- `README.md` documents all of the above
