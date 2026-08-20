# Operations

## Deploy

### Local dev (docker-compose)

```bash
docker compose -f infra/docker-compose.yml up -d
```

Brings up CockroachDB, the Go backend, and the Next.js frontend. Demo data is seeded on first boot.

### AWS (recommended)

1. Provision a CRDB Cloud cluster (free tier ok). Capture the connection string.
2. Apply migrations with the Go runner (splits statements itself, immune to
   the `XXUUU: expecting 1 statement` multi-statement batch error):

```powershell
cd backend
$env:CRDB_URL = "<connection string>"
go run ./cmd/stillmind migrate -dir migrations
```
3. Build the backend with `cd backend && go build -o stillmind ./cmd/stillmind`.
4. Package as a container; deploy to **Lambda** behind **API Gateway** or to **ECS Fargate**.
5. Host the frontend on **S3 + CloudFront** or **Amplify**.
6. Wire **EventBridge** rules for compaction and TTL sweep.
7. Set **CloudWatch** alarms on latency, error rate, compaction validation score.

## Runbook

### "Latency p95 > 3s"

- Check `bedrock_invocation` latency in CloudWatch.
- Check `vector_search` p95 in CRDB DB Console.
- Inspect compaction counters — too-frequent compaction hurts throughput.

### "Compaction validation score < 0.9"

- Recent prompts may have over-summarized. The worker falls back to a verbose mode.
- Check `_memory_architecture.compact_policy` and adjust `budget_tokens` upward.

### "Resume failed"

- Check `agent_checkpoints` for the latest row. If missing, the previous turn's commit failed and the agent restarts from the prior committed checkpoint.

### "Negative test": user A sees user B's data

- Immediately block the runtime DB role.
- Inspect `audit_events` for the offending query.
- Run the isolation test in `benchmarks/recall/`.

## Backups

- CRDB Cloud handles automated backups. Use `ccloud` to verify:

```bash
ccloud cluster backups list --cluster <cluster-id>
```

## Cost guardrails

- Bedrock model is pinned to `claude-3-haiku` + `titan-embed-text-v2`.
- Compaction runs at most every `COMPACT_MIN_INTERVAL_S` and only when pressure ≥ 0.85.
- Lambda reserved concurrency is set to 5 in the demo.
