# ccloud CLI — one-shot ops

```bash
# Login
ccloud auth login

# Create a free-tier demo cluster
ccloud cluster create --name stillmind-demo --cloud aws --region us-east-1 --servers 1

# Capture the connection string
ccloud cluster connection-string --cluster stillmind-demo --user stillmind

# Migrate (statement-split runner — immune to multi-statement batch errors)
cd backend
export CRDB_URL="$(ccloud cluster connection-string --cluster stillmind-demo --user stillmind)"
go run ./cmd/stillmind migrate -dir migrations

# Audit log access
ccloud cluster audit-logs list --cluster stillmind-demo --limit 50

# Backups
ccloud cluster backups list --cluster stillmind-demo
```

## Spin up a multi-region cluster for the production path

```bash
ccloud cluster create --name stillmind-prod --cloud aws --region us-east-1 \
  --servers 3 --replication-factor 3 --regions us-east-1,us-west-2,eu-west-1
```

## List CockroachDB Agent Skills

```bash
# Pull the open-source skills repo
git clone https://github.com/cockroachlabs/cockroachdb-skills
ls cockroachdb-skills/skills
```

Reference them in your agent runtime to encode onboarding, schema design, performance, security, and observability workflows as machine-executable skills.
