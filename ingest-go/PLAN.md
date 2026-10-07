# ingest-go: Setup and Build Plan

The tenant-facing entry point. It authenticates, rate limits, validates, dedupes, and publishes usage events to Kafka. See [`README.md`](README.md) for what the service does.

Contracts this service implements:

- [`../docs/incoming-event-schema.md`](../docs/incoming-event-schema.md): the HTTP API (request, response, errors, limits, implementation notes)
- [`../docs/kafka-event-schema.md`](../docs/kafka-event-schema.md): the message published to `usage.events.v1`

## Progress

- [x] Module created: `github.com/offthemainthread/tollgate/ingest-go`
- [x] Dependencies fetched: gin, franz-go, go-redis, miniredis
- [ ] Skeleton and `/healthz`
- [ ] Request/response types and request-level checks
- [ ] Validation
- [ ] Auth
- [ ] Publisher (Kafka)
- [ ] Dedupe (Redis)
- [ ] Handler wiring
- [ ] Rate limiting
- [ ] Meter registry backed by `billing-spring`

`go.mod` currently marks every dependency `// indirect` because no code imports them yet. Run `go mod tidy` after writing the first imports and the markers will correct themselves.

## Dependencies

| Package                                | Purpose                                   |
| -------------------------------------- | ----------------------------------------- |
| `github.com/gin-gonic/gin`             | HTTP server and routing                   |
| `github.com/twmb/franz-go/pkg/kgo`     | Kafka producer (idempotent by default)    |
| `github.com/redis/go-redis/v9`         | Dedupe and rate limiting                  |
| `github.com/alicebob/miniredis/v2`     | In-memory fake Redis for tests            |

Everything else uses the standard library: `log/slog` for logging, `os.Getenv` for config, `net/http` for server lifecycle.

## Layout

```
ingest-go/
  cmd/ingest/main.go     # wiring, config, graceful shutdown
  internal/
    config/              # env-based config
    api/                 # Gin router, handlers, request/response types
    validate/            # per-event validation rules (pure functions)
    auth/                # Authenticator interface + static-key implementation
    meters/              # MeterRegistry interface + static implementation
    dedupe/              # Deduper interface + Redis implementation
    publisher/           # Publisher interface + Kafka implementation
    ratelimit/           # later
```

Define each dependency as an interface where it is consumed, so the handler can be tested with fakes.

## Build order

1. **Skeleton.** `main.go` with Gin, `GET /healthz`, and graceful shutdown (`signal.NotifyContext` plus `http.Server.Shutdown`).
2. **Request and response types** matching the incoming schema doc. `POST /v1/events` returns `400` for a bad body (not JSON, not an array, empty), `413` over 500 events or 1 MB, `415` for the wrong content type.
3. **Validation.** Pure functions with table-driven tests. The rules are fully specified in the incoming schema doc.
4. **Auth.** Static API-key map from config, behind an `Authenticator` interface. Returns the tenant ID.
5. **Publisher.** franz-go producer, `ProduceSync`, all-replica acks, topic `usage.events.v1`, message key `tenant_id`, one message per event.
6. **Dedupe.** Redis `SET NX` on `idem:{tenant}:{key}` with a TTL, value is a hash of (`meter`, `quantity`, `timestamp`). Delete the key if the Kafka publish fails.
7. **Wire it together** in this order: authenticate, rate limit, request-level checks, per-event validation, dedupe, publish.
8. **Rate limiting** per tenant (token bucket), returning `429` with `Retry-After`.
9. **Real meter registry**: cached tenant profile from `billing-spring` with refresh-on-miss. Until then use the static `MeterRegistry`.

Test each step before moving on. After step 7, `curl` the service and watch the topic.

## Run and verify locally

Start the stack and create the topic once (matches the 3 partitions in the Kafka schema doc):

```
make up
docker compose exec kafka /opt/kafka/bin/kafka-topics.sh --create \
  --topic usage.events.v1 --partitions 3 --replication-factor 1 \
  --bootstrap-server localhost:9092
```

Watch the topic:

```
docker compose exec kafka /opt/kafka/bin/kafka-console-consumer.sh \
  --topic usage.events.v1 --from-beginning --bootstrap-server localhost:9092
```

Send an event:

```
curl -i -X POST http://localhost:8080/v1/events \
  -H "Authorization: Bearer <API_KEY>" \
  -H "Content-Type: application/json" \
  -d '[{"idempotency_key":"test-1","meter":"api_calls","quantity":5,"timestamp":"2026-10-05T14:00:00Z"}]'
```

Send it twice. The second response should show `duplicate`, and the topic should still hold one message.

## Configuration

Environment variables (commit a `.env.example`):

| Variable        | Purpose                                          |
| --------------- | ------------------------------------------------ |
| `PORT`          | HTTP port (default `8080`)                       |
| `KAFKA_BROKERS` | Comma-separated broker list                      |
| `KAFKA_TOPIC`   | Defaults to `usage.events.v1`                    |
| `REDIS_ADDR`    | Redis address                                    |
| static API keys | Key to tenant (and meters) mapping, until real auth exists |

## Gotchas

- **Per-event errors need loose decoding.** Decoding straight into `[]Event` with `Quantity int64` makes one event with `"quantity": 1000.5` fail the whole request. Decode the array as `[]json.RawMessage` and parse each event separately, so a bad event becomes a per-event `rejected` result instead of a request-level `400`.
- **Enforce the 1 MB limit** with `http.MaxBytesReader` before decoding.
- **Check the `Z` suffix explicitly.** `time.Parse(time.RFC3339, ...)` also accepts other offsets, so verify the timestamp is UTC.
- **Delete the Redis key if publishing fails**, or the client's retry is wrongly treated as a duplicate and the event is lost.
- **If publishing fails partway through a batch**, fail the whole request with `503`. Retrying is safe because already-published events come back as `duplicate`.
- **Use `ProduceSync`** and return `202` only after Kafka acknowledges the write.

## Repo housekeeping

- [ ] **Makefile:** line 3 reads `.PHONY up down restart logs`. It needs a colon: `.PHONY: up down restart logs`. Without it, `make` stops with "missing separator".
- [ ] **Makefile:** add a `topics` target for the topic-creation command above.
- [ ] **CI:** `.github/workflows/go-test.yml` runs `aggregator-go` too, but that folder has no `go.mod` yet. Remove it from the matrix until the module exists.
- [ ] Add `.env.example`.
