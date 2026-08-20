# Benchmarks — evaluating long-horizon agent memory

Long-horizon agents don't have a single perfect benchmark. We use a **layered suite** that covers what the ACM paper calls out — accuracy, latency, token cost, context-rot resistance — and is reproducible.

## Targets

| Benchmark | Target | Why |
|---|---|---|
| LongMemEval (subset) | ≥ 80% | Standard long-memory eval across 6 categories |
| LoCoMo (cat 1–4) | ≥ 85% | Multi-session dialogue |
| Hybrid recall (vector + lexical) | Recall@5 ≥ 0.85 | Validates ACM hybrid thesis |
| Compaction fidelity | ≥ 0.9 | Prevents the 66.7% → 57.1% cliff |
| p95 turn latency | < 2.5s | Production readiness |
| Tokens per turn | bounded by W | Linear cost regime |
| Scope isolation | 100% negative rate | Safety |
| Resume success rate | ≥ 95% | Resilience |
| `AS OF SYSTEM TIME` | every audit row reproducible | Audit/observability |

## Harness

`benchmarks/` contains:

- `longmemeval/` — reduced set, run via `go run ./cmd/longmemeval`.
- `locomo/` — reduced set, run via `go run ./cmd/locomo`.
- `recall/` — invoice a small synthetic meditation corpus, measure Recall@5 for vector-only, lexical-only, and hybrid.
- `compaction/` — generate fictional long sessions, measure fact-preservation score.
- `latency/` — p50/p95 with a synthetic 50-turn session.

## Reporting

Each run writes a JSON report to `benchmarks/_reports/`. The README links the latest numbers.

## What we don't measure

- Cost against proprietary models (we standardize on Bedrock Haiku).
- Multi-hop reasoning beyond the "join facts across sessions" category.
- Stress tests beyond 500 simulated turns.

Those are documented in the paper as open work.
