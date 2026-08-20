## What does this PR do?

<!-- Describe the change and why. Link the issue it resolves, if any. -->

Fixes #issue_number

## Type of change

- [ ] Bug fix
- [ ] New feature
- [ ] Documentation
- [ ] CI / tooling
- [ ] Refactor (no behavior change)

## How has this been tested?

- [ ] `cd backend && go build ./... && go vet ./...`
- [ ] `cd frontend && npm run build`
- [ ] Manual test on the running stack (`/health`, `/chat`, `/memory/recall`)

## Checklist

- [ ] No secrets or credentials committed
- [ ] SQL goes through parameterized pgx queries (no string interpolation)
- [ ] Migrations are forward-only and ordered (`000N_name.sql`)
- [ ] Lifestyle-only guidance; no medical claims added
- [ ] `CHANGELOG.md` updated for user-facing changes
