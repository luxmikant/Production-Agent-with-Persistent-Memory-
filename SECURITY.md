# Security Policy

## Supported Versions

This project is a hackathon prototype under active development. Only the latest
commit on `main` receives security fixes.

| Version | Supported          |
| ------- | ------------------ |
| main    | :white_check_mark: |

## Reporting a Vulnerability

Please do **not** open a public issue for security vulnerabilities. Instead,
email `luxmikant@outlook.com`.

You should receive an acknowledgment within 48 hours. If you do not, follow up
by opening a private issue or contacting one of the maintainers directly.

In your report please include, where possible:

- A description of the vulnerability and the potential impact
- Steps to reproduce (proof of concept)
- Affected versions / endpoints
- Any suggested remediation

We will use the Common Vulnerability Scoring System (CVSS) to assess severity
and will disclose coordinated fixes once they are ready.

## Security Considerations for This Project

StillMind is a demo of agentic memory on CockroachDB + AWS. Before running it
against real user data, address at least:

- **Model endpoint exposure** — the demo opens the inference port (Ollama
  `11434`) to the world with no auth. Restrict it to a VPN/known IPs before
  anything real.
- **Database credentials** — use AWS Secrets Manager (the code reads
  `CRDB_URL` from env; wire secret-first boot for production).
- **AuthN/AuthZ** — the included Cognito pool + JWT authorizer is provisioned
  but not wired to routes; enable it before multi-tenant use.
- **Medical disclaimer** — StillMind is non-clinical lifestyle guidance only;
  never present it as medical advice.
