# AWS services — why each one

The demo uses a **cost-efficient** set of AWS services. Bedrock defaults target the **free / lowest-cost** tier models.

## Amazon Bedrock

- **Reasoning:** `anthropic.claude-3-haiku-20240307-v1:0` — cheapest Claude on Bedrock, sufficient for guided meditation scripts and plan generation.
- **Embeddings:** `amazon.titan-embed-text-v2:0` — 1024-dim, cheap, native to Bedrock.
- **Why:** no model hosting, no GPU ops, IAM-native, scales per request.

## AWS Lambda

- Frontend API + agent orchestrator functions.
- Compaction trigger (memory pressure or scheduled).
- Short-lived, well-priced for the demo cadence.

## Amazon S3

- Generated CSV plans.
- Audio script text (TTS in a future iteration).
- Compacted summaries (optional archive).

## API Gateway

- Public HTTPS front for the Lambda backend.
- Optional usage plans / API keys.

## CloudWatch

- p50/p95 latency per route.
- Tokens per turn, cost per session.
- Compaction success rate, validation score.

## EventBridge + SQS

- Async ingest pipeline.
- Compaction scheduler.

## Secrets Manager

- CockroachDB connection string.
- AWS credentials (if not using an instance role).

## Amazon Cognito (optional, production path)

- Multi-user auth. The demo is single-tenant and ships without Cognito.

## Bedrock free-tier note

Bedrock offers free inference windows for select models in select regions for new accounts. Use the cheapest models (`Claude 3 Haiku`, `Titan Embeddings v2`) — they consume less of any free credits and are also cheap once credits are exhausted. The repo pins those defaults.
