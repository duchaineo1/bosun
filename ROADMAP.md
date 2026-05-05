# Bosun Roadmap

Bosun is a Kubernetes-native Ansible automation platform. Each section tracks feature status.

Legend: ✅ Done · 🚧 In Progress · 🔲 Planned · 💡 Stretch

---

## Core Platform

| Feature | Status | Notes |
|---------|--------|-------|
| K8s Job execution (one pod per job) | ✅ | `batch/v1` Jobs, TTL cleanup |
| Log streaming (SSE) | ✅ | DB-backed, `event: done` on terminal state |
| Configurable runner image (EE-style) | ✅ | Per-template image override, falls back to `DEFAULT_RUNNER_IMAGE` |
| Multi-arch images (amd64 + arm64) | ✅ | QEMU + Buildx in release workflow |
| CI / GitHub Actions | ✅ | Build + vet on PR; build+push on merge |
| HA controller (leader election) | 🔲 | Currently single-replica SPOF |
| Runner resource limits | 🔲 | CPU/memory requests/limits on spawned pods |
| Job concurrency limits | 🔲 | Max parallel jobs per template or globally |
| Workflow templates (DAG) | 🔲 | Chain templates with success/failure branching |
| Scheduled jobs (cron) | 🔲 | Trigger templates on a cron schedule |

---

## Authentication

| Feature | Status | Notes |
|---------|--------|-------|
| Local users (bcrypt, JWT) | ✅ | Single admin via `/setup`; JWT 24h TTL |
| Multi-user local auth | 🔲 | Admin-managed user accounts |
| LDAP | 🔲 | Bind DN + group mapping |
| OAuth 2.0 (GitHub, Google) | 🔲 | Social login |
| OIDC (generic) | 🔲 | Keycloak, Azure AD, Okta, etc. |
| SAML 2.0 | 💡 | Enterprise SSO |
| MFA / TOTP | 💡 | TOTP second factor for local users |

---

## Authorization & RBAC

| Feature | Status | Notes |
|---------|--------|-------|
| Single global admin | ✅ | All-or-nothing today |
| Organizations | 🔲 | Top-level tenant boundary |
| Teams (within org) | 🔲 | Group users for permission assignment |
| Role definitions (R/W/X) | 🔲 | Read, Write, Execute per resource type |
| Resource-level permissions | 🔲 | Grant team X execute-only on template Y |
| Audit log | 🔲 | Immutable record of who did what and when |

---

## Credentials

| Feature | Status | Notes |
|---------|--------|-------|
| Credential vault | 🔲 | Encrypted store for secrets injected into runner pods |
| SSH key type | 🔲 | Private key + passphrase |
| Username/password type | 🔲 | Generic machine credentials |
| Cloud provider type | 🔲 | AWS, GCP, Azure IAM |
| HashiCorp Vault integration | 💡 | Dynamic secret fetch at job runtime |

---

## Inventories

| Feature | Status | Notes |
|---------|--------|-------|
| Static inventory | 🔲 | Host lists managed in UI/API |
| Dynamic inventory | 🔲 | Cloud provider plugins (AWS, GCP, etc.) |
| Inventory variables | 🔲 | Host/group vars stored alongside inventory |
| Inventory sync from source | 🔲 | Pull and refresh from an external source |

---

## Git Integration (Project Sources)

| Feature | Status | Notes |
|---------|--------|-------|
| Manual playbook (in runner image) | ✅ | Playbook path passed as env var |
| Git repo as playbook source (public) | ✅ | Clone at job launch, playbook relative to repo root |
| PAT auth (GitHub / GitLab / Bitbucket) | ✅ | Token stored in credentials table, injected via git credential helper |
| GitHub App auth | 🔲 | App installation token, no user PAT needed |
| SSH key auth | 🔲 | Deploy key per repo |
| Webhook-triggered sync | 🔲 | Push → auto-sync project → optional auto-launch |
| Branch / tag / SHA pinning | 🔲 | Per-template ref override |

---

## Notifications

| Feature | Status | Notes |
|---------|--------|-------|
| Webhook (generic HTTP) | 🔲 | POST job result to arbitrary URL |
| Slack | 🔲 | Incoming webhook or Bot token |
| Email (SMTP) | 🔲 | On success / failure / always |
| PagerDuty / OpsGenie | 💡 | Alert routing on failure |

---

## UI

| Feature | Status | Notes |
|---------|--------|-------|
| Login + setup | ✅ | Single-page vanilla JS |
| Template CRUD + launch | ✅ | Minimal |
| Job list + status badges | ✅ | Manual refresh |
| Log viewer (SSE streaming) | ✅ | Modal, live tail |
| Real-time job list updates | 🔲 | Auto-refresh or WebSocket |
| Org / team / user management | 🔲 | Depends on RBAC milestone |
| Credential management | 🔲 | Depends on Credentials milestone |
| Inventory management | 🔲 | Depends on Inventories milestone |
| Full UI redesign | 🔲 | Framework TBD (React / HTMX / etc.) |

---

## Suggested Milestone Order

1. **M1 — Multi-user auth + basic RBAC** — local users, org/team model, R/W/X roles
2. **M2 — Git projects** — PAT + SSH key auth, clone at launch, branch pinning
3. **M3 — Credentials vault** — SSH keys, username/password, injected into runner pods
4. **M4 — Inventories** — static first, dynamic later
5. **M5 — LDAP / OIDC** — enterprise auth on top of the RBAC model
6. **M6 — Notifications + audit log** — webhooks, Slack, immutable audit trail
7. **M7 — Reliability** — HA controller, resource limits, concurrency controls
8. **M8 — Workflows + schedules** — DAG templates, cron triggers
9. **M9 — UI redesign** — after the API surface stabilises
