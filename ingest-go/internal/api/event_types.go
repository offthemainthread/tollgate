package api

import "encoding/json"

type IncomingEvent struct {
	IdempotencyKey string          `json:"idempotency_key"`
	Meter          string          `json:"meter"`
	Quantity       json.RawMessage `json:"quantity"`
	Timestamp      string          `json:"timestamp"`
}

type EventStatus string

const (
	StatusAccepted  EventStatus = "accepted"
	StatusDuplicate EventStatus = "duplicate"
	StatusRejected  EventStatus = "rejected"
)

const (
	CodeInvalidIdempotencyKey = "invalid_idempotency_key"
	CodeUnknownMeter          = "unknown_meter"
	CodeInvalidQuantity       = "invalid_quantity"
	CodeInvalidTimestamp      = "invalid_timestamp"
	CodeIdempotencyConflict   = "idempotency_conflict"
)

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type EventResult struct {
	Index          int          `json:"index"`
	IdempotencyKey string       `json:"idempotency_key"`
	Status         EventStatus  `json:"status"`
	Error          *ErrorDetail `json:"error,omitempty"`
}

type EventsResponse struct {
	Results []EventResult `json:"results"`
}
