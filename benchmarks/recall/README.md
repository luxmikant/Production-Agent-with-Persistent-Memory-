# Synthetic recall harness

Goal: measure hybrid vs vector-only vs lexical-only recall on a small meditation-note corpus.

## Corpus

`benchmarks/recall/corpus.jsonl` — 200 short notes, each tagged with `expected_terms`.

## Run

```bash
cd backend
STILLMIND_STUB=1 go run ./cmd/recall-harness
```

Expected output (illustrative):

```json
{
  "vector_only":  { "recall_at_5": 0.78 },
  "lexical_only": { "recall_at_5": 0.62 },
  "hybrid":       { "recall_at_5": 0.88 }
}
```

## Acceptance

Hybrid ≥ 0.85 → ACM hybrid thesis holds for the demo corpus.
