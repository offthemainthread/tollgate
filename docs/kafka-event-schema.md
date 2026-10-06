# Kafka Event Schema

Internal Kafka event schema that passes through ingestion service -> aggregator service.

## Topic

| Setting       | Value                                                                         |
| ------------- | ----------------------------------------------------------------------------- |
| Topic         | `usage.events.v1`                                                             |
| Message key   | `tenant_id` (UTF-8 string). Keeps each tenant's events ordered per partition. |
| Message value | JSON, UTF-8                                                                   |
| Granularity   | One message per event. The ingestion service splits the incoming array.       |

## Message

```json
{
  "schema_version": 1,
  "tenant_id": "123",
  "received_time": "2026-10-05T14:00:02Z",
  "idempotency_key": "usage-2026-10-05-14:00-acme",
  "meter": "api_calls",
  "quantity": 1000,
  "timestamp": "2026-10-05T14:00:00Z"
}
```

| Field             | Type    | Source                                                                             |
| ----------------- | ------- | ---------------------------------------------------------------------------------- |
| `schema_version`  | integer | Set by ingestion. Currently `1`.                                                   |
| `tenant_id`       | string  | Taken from the API key in the `Authorization` header. Never from the request body. |
| `received_time`   | string  | Set by ingestion when the request was received. RFC 3339, UTC.                     |
| `idempotency_key` | string  | From the request                                                                   |
| `meter`           | string  | From the request. The name of one of the tenant's meters.                          |
| `quantity`        | integer | From the request. Whole number, 1 to 1,000,000,000.                                |
| `timestamp`       | string  | From the request. Event time (when the usage happened), RFC 3339, UTC.             |

`timestamp` is event time and `received_time` is arrival time. The aggregator uses `timestamp` to choose the aggregation window.

## Delivery semantics

- Only events that passed validation and were not known duplicates are published.
- Kafka delivery is at-least-once, and a duplicate can still reach the topic (for example if the Redis dedupe key expired or was lost). The aggregator must treat processing as idempotent, using the unique constraint on (`tenant_id`, `idempotency_key`) in Postgres as the authoritative check.
- If the aggregator hits that unique constraint, it compares contents. Identical (`meter`, `quantity`, `timestamp`): ignore the message silently. Different: the same key was reused with new values, so send the message to `usage.events.dlq` with reason `idempotency_conflict` instead of dropping it.

## Topic configuration

| Setting     | Local dev | Notes                                                                                                                                          |
| ----------- | --------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| Partitions  | 3         | Choose the production count before first deploy. Changing it later changes which partition each tenant maps to and breaks per-tenant ordering. |
| Replication | 1         | Use 3 in production.                                                                                                                           |
| Retention   | 14 days   | Long enough to replay a billing period. Revisit alongside the raw-event archive plan.                                                          |

## Dead-letter topic

`usage.events.dlq`, keyed by `tenant_id`. Used for messages the aggregator cannot process.

```json
{
  "failed_at": "2026-10-05T14:00:05Z",
  "reason": "string",
  "original": "raw message value as a string"
}
```

`original` holds the raw message bytes as a string, so messages that were not valid JSON can still be stored.

## Schema evolution

- Adding an optional field is a compatible change. No version bump is needed.
- Removing or renaming a field, or changing its meaning or type, is a breaking change. Publish to a new topic (`usage.events.v2`) and bump `schema_version`.
