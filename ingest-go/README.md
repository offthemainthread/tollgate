# Intro

This is the entry point to the system for tenants. They will send events and this service will:

- Authenticate the events using API key
- Rate limit
- Validate the request
- Deduplication on incoming API request using idempotency key provided
- Serialize the request data into the internal model for events
- Create a kafka message and publish it on the right topic/partition
