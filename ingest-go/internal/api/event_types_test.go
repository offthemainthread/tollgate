package api

import (
	"encoding/json"
	"testing"
)

func TestTenantEventDecodesLoosely(t *testing.T) {
	in := `{"idempotency_key":"k1","meter":"api_calls","quantity":1000.5,"timestamp":"not-a-time"}`

	var e IncomingEvent
	if err := json.Unmarshal([]byte(in), &e); err != nil {
		t.Fatalf("decode should succeed even with bad values, got: %v", err)
	}
	if e.IdempotencyKey != "k1" { // proves the json tags work
		t.Fatalf("idempotency_key not decoded, got %q", e.IdempotencyKey)
	}
	if string(e.Quantity) != "1000.5" {
		t.Fatalf("expected raw quantity 1000.5, got %s", e.Quantity)
	}
}
