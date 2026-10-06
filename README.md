# Tollgate

A multi-tenant usage metering and billing platform. Tenants send usage events ("tenant X made 1,000 API calls"), and Tollgate aggregates them, enforces quotas in real time, and produces accurate, auditable invoices.

The goal is to get the hard parts of billing right: duplicate events, late and out-of-order events, mid-cycle plan changes, replay after bugs, and strict tenant isolation.

## Architecture

```
                         ┌──────────────┐
  Tenant systems ──────▶ │  ingest-go   │──┐  validate, auth, dedupe (Redis), 202 Accepted
  (API key)              └──────────────┘  │
                                           ▼
                                  ┌─────────────────┐
                                  │      Kafka      │  usage.events.v1 (key = tenant_id)
                                  └─────────────────┘
                                     │            │
                       ┌─────────────┘            └─────────────┐
                       ▼                                        ▼
               ┌────────────────┐                       ┌────────────────┐
               │ aggregator-go  │── live counters ────▶ │     Redis      │ quota checks
               │ (idempotent)   │                       └────────────────┘
               └────────────────┘
                       │ window totals (usage.aggregates.v1)
                       ▼
               ┌────────────────┐        ┌────────────┐
               │ billing-spring │◀──────▶│ PostgreSQL │  plans, subscriptions, invoices, ledger
               └────────────────┘        └────────────┘
                  │          │
                  ▼          ▼
          ┌─────────────┐  ┌──────────────┐
          │  web-react  │  │  LLM service │  invoice explanations, anomaly summaries
          └─────────────┘  └──────────────┘
```

### Event flow

1. A tenant sends a usage event to `ingest-go` with an API key and an idempotency key.
2. `ingest-go` authenticates, validates, checks Redis for a recent duplicate, and publishes to `usage.events.v1`, keyed by tenant ID so each tenant's events stay ordered within a partition.
3. `aggregator-go` consumes events, writes each one exactly once (unique constraint on tenant and idempotency key), and rolls them into per-tenant, per-meter window totals.
4. Live counters in Redis power real-time quota checks ("tenant is at 100% of plan").
5. At period close, `billing-spring` reads the aggregates, applies the tenant's plan and pricing tiers, and generates an invoice.
6. The dashboard shows live usage and invoices, and the LLM layer explains invoices and flags unusual spikes.

## Services

| Folder                                    | Stack                 | Job                                                                                                 |
| ----------------------------------------- | --------------------- | --------------------------------------------------------------------------------------------------- |
| `ingest-go/`                              | Go (Gin)              | Accepts usage events, authenticates, validates, dedupes via idempotency key, publishes to Kafka     |
| `aggregator-go/`                          | Go                    | Kafka consumer, rolls events into per-tenant/per-meter window totals, maintains Redis live counters |
| `billing-spring/`                         | Java (Spring Boot)    | Plans, pricing tiers, subscriptions, proration, invoices, payment state machine (Postgres)          |
| `web-react/`                              | React + TypeScript    | Tenant usage and invoice views, admin dashboard                                                     |
| `ai-go/` or a module in `billing-spring/` | Go or Spring          | LLM invoice explanations and anomaly summaries                                                      |
| `loadtest/`                               | Go or k6              | Fires duplicate and out-of-order events and checks the resulting invoice                            |
| `deploy/`                                 | Kubernetes, Terraform | Manifests and infrastructure for GKE                                                                |

## Kafka topics

| Topic                 | Key         | Producer         | Consumers                | Notes                                       |
| --------------------- | ----------- | ---------------- | ------------------------ | ------------------------------------------- |
| `usage.events.v1`     | `tenant_id` | `ingest-go`      | `aggregator-go`          | Raw events, long retention to allow replay  |
| `usage.events.dlq`    | `tenant_id` | `aggregator-go`  | ops tooling              | Events that failed validation or processing |
| `usage.aggregates.v1` | `tenant_id` | `aggregator-go`  | `billing-spring`         | Window totals per tenant and meter          |
| `invoice.events.v1`   | `tenant_id` | `billing-spring` | notifications, analytics | Invoice and payment state changes           |

## API sketch

Ingestion (`ingest-go`):

```
POST /v1/events
Authorization: Bearer <api-key>
{
  "idempotency_key": "evt_8f3a...",
  "meter": "api_calls",
  "quantity": 1000,
  "timestamp": "2026-10-04T15:30:00Z"
}
→ 202 Accepted

POST /v1/events:batch      # up to N events per request, per-event results
```

Billing (`billing-spring`), all scoped to the authenticated tenant:

```
GET  /v1/usage?meter=api_calls&from=...&to=...
GET  /v1/invoices
GET  /v1/invoices/{id}
POST /v1/invoices/{id}/explain        # LLM explanation
POST /v1/admin/recompute              # replay a period after a pricing fix
```

## Data model (PostgreSQL)

| Table                  | Purpose                                                                                                                      |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `tenants`              | Customers of the platform                                                                                                    |
| `api_keys`             | Hashed keys, tenant, scopes, revoked flag                                                                                    |
| `meters`               | What is measured (`api_calls`, `gb_stored`, ...) and how it aggregates                                                       |
| `plans`, `plan_prices` | Pricing model: flat fee, per-unit price, tiers (from/to quantity, unit price in cents)                                       |
| `subscriptions`        | Tenant, plan, start, end, billing period                                                                                     |
| `usage_events`         | Append-only record: tenant, idempotency key, meter, quantity, event time, received time. Unique on (tenant, idempotency key) |
| `usage_aggregates`     | Tenant, meter, window start, total quantity                                                                                  |
| `invoices`             | Tenant, period, status, totals in cents                                                                                      |
| `invoice_line_items`   | Charges, proration, and adjustments (credits and debits)                                                                     |
| `payments`             | Payment attempts and their state                                                                                             |
| `audit_log`            | Append-only record of every state change                                                                                     |

All tenant-owned tables carry `tenant_id`, with Postgres row-level security as a second line of defence behind application checks.

## Key design decisions

### Idempotency and duplicates

Clients retry, so the same event will arrive more than once. There are two layers: a Redis `SET NX` with a TTL as a cheap fast path at ingestion, and a unique constraint on (tenant, idempotency key) in Postgres as the source of truth. Redis can lose data; the database cannot.

### At-least-once delivery

Kafka delivers at least once, so every consumer is idempotent. The target is "effectively once": processing an event twice has the same result as processing it once.

### Late and out-of-order events

Aggregation windows use event time, not arrival time. A short grace period lets late events land in an open window. Once an invoice is finalized it is never edited: an event that arrives afterward becomes an adjustment line item on the next invoice.

### Replay and corrections

Raw events are retained, so a billing period can be recomputed after fixing a pricing bug. A recompute produces new adjustment entries rather than overwriting history.

### Proration

Mid-cycle upgrades and downgrades are prorated by the remaining time in the billing period, with all arithmetic in integer cents and a documented rounding rule.

### Invoice and payment state machines

Invoices move `DRAFT → FINALIZED → PAID | PAST_DUE | VOID`. Payments move `PENDING → SUCCEEDED | FAILED`. Invalid transitions are rejected, and every transition writes to the audit log.

### Quota enforcement

Redis counters keyed by tenant, meter, and period support soft limits (alert) and hard limits (throttle or reject). Counters can be rebuilt from the aggregates if Redis is lost.

### Tenant isolation

The tenant ID always comes from the authenticated credential, never from the request body. Rate limits are per tenant so one noisy tenant cannot starve the others.

### LLM usage

Anomaly detection is deterministic first (for example a z-score against the tenant's history); the LLM only explains what was flagged. Only aggregates and plan details are sent, with no personal data, and the LLM never decides what a customer is charged.

## Design rules

- Money is integer cents, never floats.
- Every usage event has a tenant ID and an idempotency key.
- Kafka is at-least-once, so consumers must be idempotent.
- Usage events are append-only; corrections are new events or credits.
- Finalized invoices are immutable; changes become adjustments.
- Every tenant query is scoped by tenant ID.
- Secrets live in a secret manager, never in the repo.

## Security

- API keys are stored hashed and can be rotated and revoked.
- Dashboard users authenticate with short-lived tokens and role-based access (tenant user, tenant admin, platform admin).
- OWASP API Top 10 review: broken object-level authorization, rate limiting, input validation, and excessive data exposure.
- Audit trail for billing-relevant actions.

## Local development

```
docker compose up -d      # Postgres, Redis, Kafka
```

| Service  | Address                                                    |
| -------- | ---------------------------------------------------------- |
| Postgres | `localhost:5432` (user, password, and database: `billing`) |
| Redis    | `localhost:6379`                                           |
| Kafka    | `localhost:9092`                                           |

Each service will document its own run and test commands in its folder.

## Testing and the load-test demo

- Unit tests for pricing, tiering, and proration math.
- Integration tests against the compose stack (Testcontainers or compose).
- The main demo: a load-test script fires thousands of duplicate and out-of-order events, then verifies that the invoice total matches an independently computed expected value.

## Deployment (GCP)

- GKE for the services, Cloud SQL for PostgreSQL, Memorystore for Redis, Artifact Registry for images, Secret Manager for secrets.
- Kafka: managed service, or self-hosted on GKE to save credits.
- GitHub Actions builds, tests, and deploys, authenticating to GCP with Workload Identity Federation.
- Cost control: develop locally, deploy only for demos and load tests, tear down afterward, and set a billing alert.

## Build order

1. Ingestion API + Kafka + aggregator, with a load test proving idempotency
2. Spring Boot plans and invoice generation (flat + per-unit pricing)
3. Tiered pricing, proration, late and out-of-order event handling
4. Redis quota enforcement
5. React dashboard
6. LLM features
7. Dockerize, deploy to GKE, add CI/CD

## Status

- [x] Project idea and architecture
- [x] Local infrastructure (`docker-compose.yml`)
- [ ] `ingest-go`
- [ ] `aggregator-go`
- [ ] `billing-spring`
- [ ] `web-react`
- [ ] LLM features
- [ ] Load test
- [ ] GKE deployment and CI/CD

## Out of scope (for now)

Taxes, multi-currency, dunning, real card processing (use a mock payment provider).

## Interview talking points

- Why "exactly-once" is really effectively-once, and how the idempotency layers work.
- What happens to an event that arrives after its invoice closed.
- How a pricing bug is corrected for past periods without rewriting history.
- Why Redis is a cache and optimization here, and Postgres is the source of truth.
- How tenant isolation is enforced at the API, query, and database layers.
