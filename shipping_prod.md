# StillMind — Production Shipping Guide (End-to-End)

This is the full depth walkthrough to take StillMind from an empty AWS account to a
running production system. It follows the production design exactly:

```
Layer            | Service                              | Role
-----------------|--------------------------------------|--------------------------------
LLM & embeddings | Bedrock (Claude Haiku + Titan Embed) | Reasoning, semantic vectors
Agent runtime    | Lambda (short) / ECS Fargate (long)  | Orchestrator, planner, compactors
API              | API Gateway (HTTP API)               | Webhook + export endpoints
Async pipelines  | SQS + EventBridge                    | Ingest & compact queues
Artifacts        | S3                                   | Audio scripts, plan CSVs, exports
Secrets          | Secrets Manager                      | DB creds, Bedrock access
Observability    | CloudWatch + X-Ray                   | Latency, token cost, retries
Auth             | Cognito                              | User isolation (production)
Schedules        | EventBridge cron                     | Compaction, TTL sweep
Edge UI          | S3 + CloudFront (or Amplify)         | Demo frontend
Memory DB        | CockroachDB Cloud                    | All durable state
```

The ACM primitives are realized as AWS + CRDB components:

| ACM primitive | Production realization |
|---|---|
| Architecting | One-shot **architect Lambda** writes `_memory_architecture` |
| Ingesting | **API → SQS → ingest Lambda chain** (qualify → extract → resolve → persist) |
| Scoping | `user_id` prefix column on the CRDB vector index; every query scoped |
| Anticipating | **prefetch Lambda** writes `prefetch_cache` rows (TTL'd) |
| Compacting | **compact Lambda** (EventBridge cron) + **judge Lambda** validates |
| Checkpointing | Agent loop persists `agent_checkpoints` every step |
| Locks | `resource_locks` TTL lease via SERIALIZABLE txn |
| Audit | `audit_events` append-only + CloudWatch mirror |

Estimated time: **2–4 hours** for a first deploy. Estimated monthly cost at demo scale:
**< $25** (CRDB free tier + Bedrock Haiku/Titan + Lambda free tier + small S3/CloudFront).

---

# Phase 0 — Local prerequisites

Install and verify every tool before touching the cloud.

## 0.1 Tools

| Tool | Version | Verify |
|---|---|---|
| Go | 1.22+ | `go version` |
| Node.js | 20+ | `node --version` |
| AWS CLI | v2 | `aws --version` |
| ccloud | latest | `ccloud version` |
| psql | 14+ | `psql --version` |
| jq | latest | `jq --version` |
| zip | any | `zip --version` |

## 0.2 AWS CLI identity

```powershell
aws configure
# AWS Access Key ID:     <from IAM user with AdministratorAccess for setup>
# AWS Secret Access Key: <...>
# Default region:        us-east-1
# Default output:        json

aws sts get-caller-identity
# Record the AccountId — you will use it as $ACCOUNT_ID everywhere below.
```

Set shell variables used throughout this guide:

```powershell
$ACCOUNT_ID = (aws sts get-caller-identity --query Account --output text)
$REGION     = "us-east-1"
$APP        = "stillmind"
```

> **Region note.** Use `us-east-1` unless you have a reason otherwise. It has the
> broadest Bedrock model availability (Claude 3 Haiku + Titan Embeddings v2) and
> every service in the design (Cognito, X-Ray, EventBridge Scheduler, Lambda,
> API Gateway v2, SQS, S3, Secrets Manager) is GA there.

---

# Phase 1 — CockroachDB Cloud (memory system of record)

Everything durable lives here: profiles, semantic notes (with vectors), sessions,
checkpoints, tool history, audit, plans, locks, prefetch cache, compactions, and
the `_memory_architecture` config that governs the ACM lifecycle.

## 1.1 Authenticate ccloud

```powershell
ccloud auth login
```

This opens the browser; sign in with the same CockroachDB Cloud account.

## 1.2 Create the cluster

**Demo (free tier):**

```powershell
ccloud cluster create --name stillmind-demo --cloud aws --region us-east-1 --servers 1
```

**Production (multi-AZ, ready to grow to multi-region):**

```powershell
ccloud cluster create --name stillmind-prod --cloud aws --region us-east-1 `
  --servers 3 --replication-factor 3
```

> For global users later: add `--regions us-east-1,us-west-2,eu-west-1`.
> CRDB's Raft replication is what makes memory survive AZ/region loss with zero
> app-level failover code — this is the resilience story for the judges.

Capture the cluster ID:

```powershell
$CLUSTER_ID = (ccloud cluster list --format json | jq -r '.clusters[] | select(.name=="stillmind-demo") | .id')
ccloud cluster get --cluster $CLUSTER_ID
```

## 1.3 Create the SQL user

```powershell
$DB_PASSWORD = -join ((48..57)+(65..90)+(97..122) | Get-Random -Count 24 | ForEach-Object {[char]$_})
ccloud cluster user create --cluster $CLUSTER_ID --user stillmind --password $DB_PASSWORD
```

Store it temporarily — it goes into Secrets Manager in Phase 3.

## 1.4 Get the connection string

```powershell
$CRDB_URL = ccloud cluster connection-string --cluster $CLUSTER_ID --user stillmind
$CRDB_URL = $CRDB_URL -replace '\{password\}', $DB_PASSWORD
echo $CRDB_URL
# postgresql://stillmind:XXXX@stillmind-demo.abc.aws-us-east-1.crdb.io:26257/defaultdb?sslmode=verify-full
```

## 1.5 Apply migrations

Apply migrations with the built-in runner (executes each statement
individually, so it works regardless of client-side batching behavior):

```powershell
cd backend
$env:CRDB_URL = $CRDB_URL
go run ./cmd/stillmind migrate -dir migrations
```

> The `migrate` subcommand applies `0001_init.sql` then `0002_seed.sql` in
> order and prints `ok stmt N` per statement. It is idempotent — safe to
> re-run (all DDL uses `IF NOT EXISTS`, the seed uses `ON CONFLICT DO NOTHING`).
>
> If you prefer psql: `psql "$CRDB_URL" -f backend/migrations/0001_init.sql -f backend/migrations/0002_seed.sql`
> works too — but if you see `XXUUU: expecting 1 statement, found N`, use the
> Go runner instead (the CockroachDB Cloud Console SQL editor and some drivers
> send multi-statement batches).

`0001_init.sql` does three important things:

1. `SET CLUSTER SETTING feature.vector_index.enabled = true;`
2. Creates all memory tables, including `notes` with a
   **distributed vector index with `user_id` as a prefix column**:
   ```sql
   CREATE VECTOR INDEX notes_user_embed_idx ON notes (user_id, embedding);
   ```
   The prefix column is what enforces tenant isolation **at the index layer** —
   the index is only used when `user_id` is constrained to a specific value.
3. Enables row-level TTL on `raw_utterances` and `notes` via
   `ttl_expiration_expression = 'ttl_until'` with a `@daily` sweep job —
   compaction sets `ttl_until`, CockroachDB deletes the expired rows.

## 1.6 Verify the vector index actually engages

```powershell
psql $CRDB_URL -c "SHOW TABLES FROM stillmind"
psql $CRDB_URL -c "EXPLAIN SELECT id FROM stillmind.notes WHERE user_id='demo-tenant:demo-user' ORDER BY embedding <-> '[0,0,0]'::vector LIMIT 3"
```

You must see:

```
• vector search
  table: notes@notes_user_embed_idx
  prefix spans: [/demo-tenant:demo-user - /demo-tenant:demo-user]
```

If you see a full scan instead, check:
- `feature.vector_index.enabled = true` was set on **this** cluster
- the WHERE clause constrains `user_id` to an equality (not a range)

## 1.7 Enable the Managed MCP Server (demo inspection)

1. CockroachDB Cloud Console → your cluster → **MCP Server** tab.
2. Copy the config snippet.
3. Paste into Claude Code / Cursor / VS Code settings:

```json
{
  "mcpServers": {
    "cockroachdb": { "type": "http", "url": "https://cockroachlabs.cloud/mcp" }
  }
}
```

4. During the demo, ask your coding agent: *"List tables in stillmind, then EXPLAIN
   the vector recall query."* This is the live memory-inspection beat.

## 1.8 Audit logs + backups

```powershell
ccloud cluster audit-logs list --cluster $CLUSTER_ID --limit 10
ccloud cluster backups list --cluster $CLUSTER_ID
```

Both are automatic on Cloud. This is part of the "production readiness" story.

---

# Phase 2 — Amazon Bedrock (reasoning + embeddings)

Cost-efficient defaults are pinned by the design:

- **Chat / reasoning:** `anthropic.claude-3-haiku-20240307-v1:0`
- **Embeddings:** `amazon.titan-embed-text-v2:0` (1024-dim)

## 2.1 Enable model access

Console path: **AWS Console → Bedrock → Model access → Manage model access** →
check *Claude 3 Haiku* and *Titan Embeddings v2* → Save. Access is usually granted
in seconds for these models.

CLI verification:

```powershell
aws bedrock list-foundation-models --region $REGION `
  --query "modelSummaries[?contains(modelId,'haiku') || contains(modelId,'titan-embed')].modelId"
```

Expected: both model IDs listed.

## 2.2 Smoke test from CLI

```powershell
$body = @{
  inputText = "evening wind down"
} | ConvertTo-Json -Compress
$body | Out-File -Encoding ascii -NoNewline body.json

aws bedrock-runtime invoke-model --region $REGION `
  --model-id amazon.titan-embed-text-v2:0 `
  --content-type application/json `
  --body fileb://body.json `
  out.json
```

`out.json` should contain a 1024-length `embedding` array. This is the exact call
`backend/internal/bedrock/bedrock.go` makes.

---

# Phase 3 — Secrets Manager (DB creds, nothing in plaintext)

## 3.1 Store the CRDB URL

```powershell
aws secretsmanager create-secret `
  --name "$APP/crdb_url" `
  --secret-string "$CRDB_URL" `
  --region $REGION

$CRDB_SECRET_ARN = (aws secretsmanager describe-secret --secret-id "$APP/crdb_url" --query ARN --output text)
```

## 3.2 Store the DB password separately (optional but cleaner rotation)

```powershell
aws secretsmanager create-secret `
  --name "$APP/crdb_password" `
  --secret-string "$DB_PASSWORD"
```

## 3.3 Retrieval pattern

At Lambda cold start, the backend reads the secret:

```go
// (pattern — wire in internal/config if you want secret-first boot)
val, _ := secretsClient.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
    SecretId: aws.String("stillmind/crdb_url"),
})
os.Setenv("CRDB_URL", *val.SecretString)
```

For the demo you may instead pass `CRDB_URL` as a Lambda env var directly —
Lambda env vars are encrypted at rest with KMS. **Secrets Manager is the
production-correct answer and this guide uses it.**

---

# Phase 4 — IAM (least-privilege roles)

You need three roles: one for the API/agent Lambda, one for the compaction pair,
one for the ingest chain.

## 4.1 Trust policy (shared by all three)

```powershell
@'
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": { "Service": "lambda.amazonaws.com" },
    "Action": "sts:AssumeRole"
  }]
}
'@ | Out-File -Encoding ascii trust.json
```

## 4.2 API/agent role

```powershell
aws iam create-role --role-name stillmind-api-role --assume-role-policy-document fileb://trust.json
```

Inline policy:

```powershell
@'
{
  "Version": "2012-10-17",
  "Statement": [
    { "Effect": "Allow",
      "Action": ["bedrock:InvokeModel"],
      "Resource": [
        "arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-3-haiku-20240307-v1:0",
        "arn:aws:bedrock:us-east-1::foundation-model/amazon.titan-embed-text-v2:0"
      ]},
    { "Effect": "Allow",
      "Action": ["logs:CreateLogGroup","logs:CreateLogStream","logs:PutLogEvents"],
      "Resource": "*" },
    { "Effect": "Allow",
      "Action": ["secretsmanager:GetSecretValue"],
      "Resource": "arn:aws:secretsmanager:us-east-1:ACCOUNT_ID:secret:stillmind/*" },
    { "Effect": "Allow",
      "Action": ["sqs:SendMessage"],
      "Resource": "arn:aws:sqs:us-east-1:ACCOUNT_ID:stillmind-ingest" },
    { "Effect": "Allow",
      "Action": ["s3:PutObject","s3:GetObject"],
      "Resource": "arn:aws:s3:::stillmind-artifacts/*" },
    { "Effect": "Allow",
      "Action": ["xray:PutTraceSegments","xray:PutTelemetryRecords"],
      "Resource": "*" }
  ]
}
'@ -replace 'ACCOUNT_ID', $ACCOUNT_ID | Out-File -Encoding ascii api-policy.json

aws iam put-role-policy --role-name stillmind-api-role `
  --policy-name stillmind-api --policy-document fileb://api-policy.json
```

## 4.3 Ingest role (needs SQS receive + CRDB write via secret + Bedrock)

```powershell
aws iam create-role --role-name stillmind-ingest-role --assume-role-policy-document fileb://trust.json
```

Same policy as 4.2 but swap the SQS statement for:

```json
{ "Effect": "Allow",
  "Action": ["sqs:ReceiveMessage","sqs:DeleteMessage","sqs:GetQueueAttributes"],
  "Resource": "arn:aws:sqs:us-east-1:ACCOUNT_ID:stillmind-ingest" }
```

## 4.4 Compact role (Bedrock ×2 for summarize+validate, CRDB, logs)

```powershell
aws iam create-role --role-name stillmind-compact-role --assume-role-policy-document fileb://trust.json
```

Policy: Bedrock invoke + secrets + logs + xray (no SQS, no S3).

---

# Phase 5 — S3 (artifacts: CSVs, audio scripts, exports)

```powershell
aws s3api create-bucket --bucket "$APP-artifacts-$ACCOUNT_ID" --region $REGION
aws s3api put-bucket-encryption --bucket "$APP-artifacts-$ACCOUNT_ID" `
  --server-side-encryption-configuration '{\"Rules\":[{\"ApplyServerSideEncryptionByDefault\":{\"SSEAlgorithm\":\"AES256\"}}]}'
aws s3api put-public-access-block --bucket "$APP-artifacts-$ACCOUNT_ID" `
  --public-access-block-configuration BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true
```

Layout the bucket will use:

```
s3://stillmind-artifacts-<account>/
├── plans/<user_id>/<plan_id>.csv      # plan exports
├── scripts/<session_id>.txt           # guided audio script text
├── exports/<user_id>/<date>.json      # full memory export (GDPR-style)
└── compactions/<user_id>/<id>.json    # archived raw packets (optional)
```

In the demo the CSV is generated in-memory by the Go handler; in production,
`plan.ToCSV` output is `PutObject`'d here and the API returns a presigned URL.

---

# Phase 6 — SQS (async ingest pipeline)

The ACM **Ingesting** primitive must never block the user's turn.

## 6.1 Create the queue

```powershell
aws sqs create-queue --queue-name stillmind-ingest --region $REGION `
  --attributes '{\"VisibilityTimeout\":\"60\",\"MessageRetentionPeriod\":\"86400\"}'

$INGEST_QUEUE_URL = (aws sqs get-queue-url --queue-name stillmind-ingest --query QueueUrl --output text)
```

## 6.2 Dead-letter queue

```powershell
aws sqs create-queue --queue-name stillmind-ingest-dlq
$DLQ_ARN = (aws sqs get-queue-attributes --queue-url (aws sqs get-queue-url --queue-name stillmind-ingest-dlq --query QueueUrl --output text) --attribute-names QueueArn --query Attributes.QueueArn --output text)

aws sqs set-queue-attributes --queue-url $INGEST_QUEUE_URL `
  --attributes ("{\"RedrivePolicy\":\"{\\\"deadLetterTargetArn\\\":\\\"$DLQ_ARN\\\",\\\"maxReceiveCount\\\":3}\"}")
```

A turn that fails ingestion 3 times lands in the DLQ for inspection — this is the
"operations" part of production readiness.

---

# Phase 7 — Build the Go binaries

The backend is one module with three entrypoints:

| Binary | Role | Trigger |
|---|---|---|
| `cmd/stillmind` | API + agent loop + three-call pattern | API Gateway |
| `cmd/compact` | Pressure/cron compaction + validation | EventBridge cron |
| `cmd/recall-harness` | Benchmark | manual/CI |

## 7.1 Compile for Lambda (Linux)

```powershell
cd backend
$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"
go build -ldflags="-s -w" -o bin/bootstrap-api    ./cmd/stillmind
go build -ldflags="-s -w" -o bin/bootstrap-compact ./cmd/compact
```

> For Lambda's `provided.al2023` runtime the binary must literally be named
> `bootstrap`. We produce one per zip.

## 7.2 Package

```powershell
New-Item -ItemType Directory -Force dist | Out-Null

Copy-Item bin/bootstrap-api dist/bootstrap
Compress-Archive -Path dist/bootstrap -DestinationPath dist/api.zip -Force

Remove-Item dist/bootstrap
Copy-Item bin/bootstrap-compact dist/bootstrap
Compress-Archive -Path dist/bootstrap -DestinationPath dist/compact.zip -Force
Remove-Item dist/bootstrap
```

---

# Phase 8 — Lambda: the ACM primitive runtimes

## 8.1 Architecting (one-shot config writer)

The architect step already ran when you applied `0001_init.sql` — the
`_memory_architecture` row was inserted. In a fuller system this is a Lambda that
takes the agent description and writes that row. For production, keep it as a
**deployment-time job**:

```powershell
psql $CRDB_URL -c "SELECT agent_id, compact_policy FROM stillmind._memory_architecture"
```

The compact Lambda and the agent loop both read this row — **one config governs
every primitive**, exactly as the ACM paper prescribes.

## 8.2 Deploy the API/agent Lambda

```powershell
aws lambda create-function `
  --function-name stillmind-api `
  --runtime provided.al2023 `
  --handler bootstrap `
  --role "arn:aws:iam::$ACCOUNT_ID:role/stillmind-api-role" `
  --zip-file fileb://dist/api.zip `
  --memory-size 512 --timeout 30 `
  --tracing-config Mode=Active `
  --environment "Variables={
    CRDB_SECRET_ID=$APP/crdb_url,
    AWS_REGION=$REGION,
    BEDROCK_MODEL=anthropic.claude-3-haiku-20240307-v1:0,
    BEDROCK_EMBED_MODEL=amazon.titan-embed-text-v2:0,
    EMBED_DIM=1024,
    INGEST_QUEUE_URL=$INGEST_QUEUE_URL,
    ARTIFACTS_BUCKET=$APP-artifacts-$ACCOUNT_ID,
    COMPACT_BUDGET_TOKENS=4000,
    COMPACT_PRESSURE_THRESHOLD=0.85
  }"
```

What this Lambda does per turn (the three-call loop):

```
1. FETCH   → memory.RetrieveByVector + RetrieveByLexical (scoped by user_id)
2. COMPACT → compaction.Worker.MaybeCompact (pressure-gated, judge-validated)
3. MODEL   → bedrock.Chat(retrieved + compacted + recent turns)
4. INGEST  → sqs.SendMessage(turn)  → returns ingestion_id immediately
5. CHECKPOINT → agent_checkpoints upsert (resume story)
6. AUDIT   → audit_events insert
```

## 8.3 Deploy the ingest Lambda

The ingest worker is the **qualify → extract → resolve → persist** chain. In this
repo it lives in the same binary family; build it as a third entrypoint or run it
as a lightweight Lambda triggered by SQS:

```powershell
aws lambda create-function `
  --function-name stillmind-ingest `
  --runtime provided.al2023 --handler bootstrap `
  --role "arn:aws:iam::$ACCOUNT_ID:role/stillmind-ingest-role" `
  --zip-file fileb://dist/api.zip `   # same binary; mode from env
  --environment "Variables={MODE=ingest,CRDB_SECRET_ID=$APP/crdb_url,AWS_REGION=$REGION,BEDROCK_MODEL=anthropic.claude-3-haiku-20240307-v1:0,BEDROCK_EMBED_MODEL=amazon.titan-embed-text-v2:0}" `
  --memory-size 512 --timeout 120

aws lambda create-event-source-mapping `
  --function-name stillmind-ingest `
  --event-source-arn "arn:aws:sqs:$REGION:${ACCOUNT_ID}:stillmind-ingest" `
  --batch-size 10
```

> If you keep ingest inline (demo mode), `MODE=ingest` is unused and the API
> Lambda writes notes directly. The SQS path is the production-correct one.

## 8.4 Deploy the compaction + judge Lambda

```powershell
aws lambda create-function `
  --function-name stillmind-compact `
  --runtime provided.al2023 --handler bootstrap `
  --role "arn:aws:iam::$ACCOUNT_ID:role/stillmind-compact-role" `
  --zip-file fileb://dist/compact.zip `
  --memory-size 512 --timeout 300 `
  --tracing-config Mode=Active `
  --environment "Variables={CRDB_SECRET_ID=$APP/crdb_url,AWS_REGION=$REGION,BEDROCK_MODEL=anthropic.claude-3-haiku-20240307-v1:0,BEDROCK_EMBED_MODEL=amazon.titan-embed-text-v2:0,COMPACT_BUDGET_TOKENS=4000,COMPACT_PRESSURE_THRESHOLD=0.85,COMPACT_MIN_INTERVAL_S=60}"
```

The judge pass lives inside this binary (`Worker.validate`): a second Bedrock
prompt scores whether key facts survived. Below threshold → retry with a less
aggressive prompt (up to 3 attempts) → record `compactions.validation_score`.

## 8.5 Schedule compaction with EventBridge cron

```powershell
aws events put-rule --name stillmind-compact-5m `
  --schedule-expression "rate(5 minutes)" --region $REGION

aws lambda add-permission --function-name stillmind-compact `
  --statement-id evb-compact --action lambda:InvokeFunction `
  --principal events.amazonaws.com `
  --source-arn "arn:aws:events:$REGION:${ACCOUNT_ID}:rule/stillmind-compact-5m"

aws events put-targets --rule stillmind-compact-5m `
  --targets "Id=1,Arn=arn:aws:lambda:$REGION:${ACCOUNT_ID}:function:stillmind-compact"
```

**Note:** compaction is *also* pressure-triggered inside the agent loop
(memory-based). The cron is the belt-and-suspenders scheduled pass — together
they implement "memory-based or optimized scheduled compact" from the design.

## 8.6 ECS Fargate path (long sessions)

For sessions that exceed Lambda's 15-minute limit (long guided journeys, batch
compaction across many users), run the **same container** on ECS Fargate:

```powershell
# build + push
aws ecr create-repository --repository-name stillmind
$ECR = "$ACCOUNT_ID.dkr.ecr.$REGION.amazonaws.com/stillmind"
aws ecr get-login-password | docker login --username AWS --password-stdin $ECR
docker build -f infra/Dockerfile.backend -t "$ECR:latest" backend
docker push "$ECR:latest"

# cluster + service (sketch — full task def in infra/cdk-sketch.ts)
aws ecs create-cluster --cluster-name stillmind
```

Rule of thumb: **Lambda for turns and triggers; Fargate for anything > 5 minutes
or continuously running.**

---

# Phase 9 — API Gateway (public HTTPS front)

## 9.1 Create the HTTP API

```powershell
$API_ID = (aws apigatewayv2 create-api --name stillmind --protocol-type HTTP `
  --cors-configuration AllowOrigins='*',AllowMethods='GET,POST',AllowHeaders='content-type,authorization' `
  --query ApiId --output text)

$INTEG_ID = (aws apigatewayv2 create-integration --api-id $API_ID `
  --integration-type AWS_PROXY `
  --integration-uri "arn:aws:lambda:$REGION:${ACCOUNT_ID}:function/stillmind-api" `
  --payload-format-version 2.0 `
  --query IntegrationId --output text)

aws apigatewayv2 create-route --api-id $API_ID --route-key "ANY /{proxy+}" --target "integrations/$INTEG_ID"
aws apigatewayv2 create-stage --api-id $API_ID --stage-name prod --auto-deploy

aws lambda add-permission --function-name stillmind-api `
  --statement-id apigw --action lambda:InvokeFunction --principal apigateway.amazonaws.com `
  --source-arn "arn:aws:execute-api:$REGION:${ACCOUNT_ID}:$API_ID/*/*"

$API_BASE = "https://$API_ID.execute-api.$REGION.amazonaws.com/prod"
echo $API_BASE
```

## 9.2 Endpoints served

| Route | Method | Purpose |
|---|---|---|
| `/health` | GET | smoke |
| `/chat` | POST | agent turn (three-call loop) |
| `/plan` | POST | generate 7-day plan (with `resource_locks` lease) |
| `/plan/csv` | GET | CSV export (prod: presigned S3 URL) |
| `/sheets/sync` | POST | mock Sheets sync → production Sheets later |
| `/memory/recall` | GET | vector + lexical recall debug |
| `/memory/notes` | GET | recent notes |
| `/memory/audit` | GET | audit trail |
| `/memory/explain` | GET | EXPLAIN of the vector query |

---

# Phase 10 — Cognito (auth, production path)

The demo is single-tenant, but production multi-tenancy starts here.

```powershell
$POOL_ID = (aws cognito-idp create-user-pool --pool-name stillmind-users `
  --policies "PasswordPolicy={MinimumLength=12,RequireUppercase=true,RequireLowercase=true,RequireNumbers=true}" `
  --query UserPool.Id --output text)

aws cognito-idp create-user-pool-client --user-pool-id $POOL_ID `
  --client-name stillmind-web --explicit-auth-flows ALLOW_USER_SRP_AUTH,ALLOW_REFRESH_TOKEN_AUTH
```

Wire the API Gateway JWT authorizer:

```powershell
aws apigatewayv2 create-authorizer --api-id $API_ID --authorizer-type JWT `
  --name cognito --identity-source '$request.header.Authorization' `
  --jwt-configuration "Issuer=https://cognito-idp.$REGION.amazonaws.com/$POOL_ID,Audience=$APP_CLIENT_ID"
```

**How scope propagates:** the JWT `sub` claim becomes `user_id` in every CRDB
query. Because the vector index has `user_id` as a **prefix column**, a query
without a constrained user simply cannot use the index — isolation is structural,
not convention. This is the strongest part of the production story.

---

# Phase 11 — Frontend on S3 + CloudFront (or Amplify)

## 11.1 Build the SPA

```powershell
cd frontend
$env:NEXT_PUBLIC_API_URL = $API_BASE
npm install
npm run build
```

Next.js App Router builds a SPA here (`app/page.tsx` is a client component).
If you prefer fully static output, add `output: 'export'` to `next.config.mjs`
and use the `out/` directory.

## 11.2 Host on S3 + CloudFront

```powershell
aws s3api create-bucket --bucket "$APP-frontend-$ACCOUNT_ID" --region $REGION
aws s3 sync frontend/out "s3://$APP-frontend-$ACCOUNT_ID" --delete

aws s3api put-bucket-website --bucket "$APP-frontend-$ACCOUNT_ID" `
  --website-configuration '{\"IndexDocument\":{\"Suffix\":\"index.html\"}}'
```

CloudFront distribution with an Origin Access Control (OAC) so the bucket stays
private:

```powershell
# console path is easier: CloudFront → Create distribution →
#   Origin: s3://stillmind-frontend-<acct>  (OAC: on)
#   Default root object: index.html
#   Viewer protocol: Redirect HTTP to HTTPS
```

**Amplify alternative:** connect the GitHub repo, set build settings to the
`frontend/` directory, and Amplify handles build + CDN. Either satisfies
"Edge UI".

---

# Phase 12 — Observability (CloudWatch + X-Ray)

## 12.1 Logs

Each Lambda gets a log group automatically:
`/aws/lambda/stillmind-api`, `/aws/lambda/stillmind-ingest`, `/aws/lambda/stillmind-compact`.

Set retention:

```powershell
aws logs put-retention-policy --log-group-name /aws/lambda/stillmind-api --retention-in-days 14
aws logs put-retention-policy --log-group-name /aws/lambda/stillmind-compact --retention-in-days 14
```

## 12.2 X-Ray

Tracing was enabled with `--tracing-config Mode=Active` on both Lambdas.
In the console: **X-Ray → Service map** shows `API GW → Lambda → Bedrock → CRDB`
segments. This is where you prove retrieval is off the critical path when the
prefetch cache hits.

## 12.3 Metrics + alarms

```powershell
aws cloudwatch put-metric-alarm --alarm-name stillmind-api-errors `
  --namespace AWS/Lambda --metric-name Errors `
  --dimensions Name=FunctionName,Value=stillmind-api `
  --statistic Sum --period 300 --threshold 5 --evaluation-periods 1 `
  --comparison-operator GreaterThanThreshold

aws cloudwatch put-metric-alarm --alarm-name stillmind-api-p95 `
  --namespace AWS/Lambda --metric-name Duration `
  --dimensions Name=FunctionName,Value=stillmind-api `
  --extended-statistic p95 --period 300 --threshold 3000 --evaluation-periods 2 `
  --comparison-operator GreaterThanThreshold
```

## 12.4 Custom memory metrics (published by the app)

The agent loop should publish:

- `stillmind.tokens_per_turn` — proves linear-cost regime (bounded W)
- `stillmind.compaction.score` — validation fidelity (target ≥ 0.9)
- `stillmind.prefetch.hit_rate` — target ≥ 0.6 (the paper's research bar)
- `stillmind.recall.latency_ms` — vector index health

Publish via `cloudwatch:PutMetricData` from the Lambda (add that action to the
API role if you enable it).

---

# Phase 13 — End-to-end smoke test

```powershell
# health
curl "$API_BASE/health"

# a coaching turn (writes note + audit + checkpoint; queues ingest)
curl -X POST "$API_BASE/chat" -H "content-type: application/json" `
  -d '{\"message\":\"I sleep badly on Sundays. Help me wind down.\"}'

# semantic recall (L2 <-> over the prefixed vector index)
curl "$API_BASE/memory/recall?q=sleep"

# generate + export the plan
$plan = (curl -X POST "$API_BASE/plan" | ConvertFrom-Json)
curl "$API_BASE/plan/csv?plan_id=$($plan.plan_id)" -OutFile plan.csv
curl -X POST "$API_BASE/sheets/sync?plan_id=$($plan.plan_id)"

# prove the vector index engages on the live cluster
curl "$API_BASE/memory/explain?q=sleep"

# force a compaction (make many turns, or lower the threshold env and redeploy)
# then check:
psql $CRDB_URL -c "SELECT kind, compression_ratio, validation_score, status FROM stillmind.compactions ORDER BY ts DESC LIMIT 5"

# time-travel audit
psql $CRDB_URL -c "SELECT type, payload FROM stillmind.audit_events AS OF SYSTEM TIME '-10m' LIMIT 10"
```

## The resilience beat (for the demo video)

1. Start a plan generation from the UI.
2. Kill the API Lambda mid-run (disable/re-enable the function, or just point at
   the checkpoint row):
   ```powershell
   psql $CRDB_URL -c "SELECT run_id, step, state::text FROM stillmind.agent_checkpoints ORDER BY ts DESC LIMIT 3"
   ```
3. Re-run the request — the agent loop reads the latest checkpoint and resumes.
   **No application failover code exists — CRDB's Raft replication keeps memory
   available the whole time.**

---

# Phase 14 — Cost guardrails

| Control | Setting |
|---|---|
| Bedrock chat model | Claude 3 Haiku (cheapest Claude tier) |
| Embeddings | Titan Embed v2 (1024-dim, cheap) |
| Compaction | pressure ≥ 0.85 AND min-interval 60s AND cron every 5m as backup |
| Lambda concurrency | `aws lambda put-function-concurrency --function-name stillmind-api --reserved-concurrent-executions 5` |
| S3 lifecycle | transition `exports/` to Glacier after 90 days |
| CRDB | free/basic tier for demo; scale spend limit in Cloud Console |

```powershell
aws lambda put-function-concurrency --function-name stillmind-api --reserved-concurrent-executions 5
```

---

# Phase 15 — Rollback

```powershell
# Roll back the API Lambda to the previous zip
aws lambda update-function-code --function-name stillmind-api --zip-file fileb://dist/api-prev.zip

# Pause compaction
aws events disable-rule --name stillmind-compact-5m

# DB restore (point-in-time)
ccloud cluster restore --cluster $CLUSTER_ID --backup <backup-id>

# Drain poisoned ingest messages
aws sqs purge-queue --queue-url $DLQ_URL   # after inspection
```

---

# Phase 16 — Hackathon submission checklist

- [ ] Public repo (MIT) — already structured
- [ ] Live demo URL (CloudFront/Amplify)
- [ ] < 3 min video: onboard → session → recall → compact → kill/resume → plan → CSV/Sheets → MCP inspection
- [ ] CRDB tools identified: **Managed MCP Server + Distributed Vector Indexing** (+ ccloud, Agent Skills)
- [ ] AWS services identified: **Bedrock, Lambda, API Gateway, SQS, EventBridge, S3, Secrets Manager, CloudWatch, X-Ray, Cognito, CloudFront**
- [ ] Architecture diagram: `docs/architecture.md`
- [ ] Memory story: store → fetch (L2) → compact (validated) → ingest (async) → resume — **resilience from CRDB Raft, zero app-level FT code**

---

# Appendix A — Environment variables (complete)

| Var | Where | Example |
|---|---|---|
| `CRDB_URL` | local / CI | `postgresql://stillmind:***@host:26257/defaultdb?sslmode=verify-full` |
| `CRDB_SECRET_ID` | Lambda | `stillmind/crdb_url` |
| `AWS_REGION` | all | `us-east-1` |
| `BEDROCK_MODEL` | all | `anthropic.claude-3-haiku-20240307-v1:0` |
| `BEDROCK_EMBED_MODEL` | all | `amazon.titan-embed-text-v2:0` |
| `EMBED_DIM` | all | `1024` |
| `INGEST_QUEUE_URL` | API Lambda | SQS URL |
| `ARTIFACTS_BUCKET` | API Lambda | `stillmind-artifacts-<acct>` |
| `COMPACT_BUDGET_TOKENS` | both | `4000` |
| `COMPACT_PRESSURE_THRESHOLD` | both | `0.85` |
| `COMPACT_MIN_INTERVAL_S` | compact | `60` |
| `STILLMIND_STUB` | demo-only | `1` (deterministic offline embeddings) |
| `NEXT_PUBLIC_API_URL` | frontend | API Gateway URL |

# Appendix B — Trouble-shooting quick table

| Symptom | Check |
|---|---|
| Vector query does a full scan | `user_id` equality in WHERE; `feature.vector_index.enabled` |
| Bedrock `AccessDeniedException` | Model access enabled in region; IAM `bedrock:InvokeModel` on exact ARNs |
| 502 from API Gateway | Lambda timeout (30s) vs Bedrock cold start; check `/aws/lambda/stillmind-api` |
| Compaction never fires | pressure < 0.85; cron rule enabled; check `compactions` rows |
| Ingest lag | DLQ depth; `stillmind-ingest` error metric |
| Resume broken | `agent_checkpoints` empty for run → previous commit failed |
