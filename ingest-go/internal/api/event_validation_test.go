package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateIdempotencyKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"simple", "abc123", false},
		{"all allowed punctuation", "usage-2026-10-05T14:00_acme.v1", false},
		{"single character", "a", false},
		{"max length", strings.Repeat("a", maxIdentifierLen), false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", maxIdentifierLen+1), true},
		{"space", "has space", true},
		{"slash", "a/b", true},
		{"unicode", "café", true},
		{"trailing newline", "abc\n", true},
		{"null byte", "\x00", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateIdempotencyKey(tt.key)

			if !tt.wantErr {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, CodeInvalidIdempotencyKey, got.Code)
			assert.Contains(t, got.Message, "idempotency key")
			assert.NotEmpty(t, got.Message)
		})
	}
}

func TestValidateMeter(t *testing.T) {
	tests := []struct {
		name    string
		meter   string
		wantErr bool
	}{
		{"simple", "abc123", false},
		{"all allowed punctuation", "usage-2026-10-05T14:00_acme.v1", false},
		{"single character", "a", false},
		{"max length", strings.Repeat("a", maxIdentifierLen), false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", maxIdentifierLen+1), true},
		{"space", "has space", true},
		{"slash", "a/b", true},
		{"unicode", "café", true},
		{"trailing newline", "abc\n", true},
		{"null byte", "\x00", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateMeter(tt.meter)

			if !tt.wantErr {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, CodeUnknownMeter, got.Code)
			assert.Contains(t, got.Message, "meter")
			assert.NotEmpty(t, got.Message)
		})
	}
}

func TestParseQuantity(t *testing.T) {
	const (
		msgRequired = "quantity is required"
		msgInteger  = "quantity must be an integer"
		msgRange    = "quantity must be between 1 and 1000000000"
	)

	tests := []struct {
		name         string
		raw          json.RawMessage
		wantQuantity int64
		wantMsg      string // "" means the input is valid
	}{
		// valid
		{"min", json.RawMessage(`1`), 1, ""},
		{"typical", json.RawMessage(`1000`), 1000, ""},
		{"max", json.RawMessage(`1000000000`), 1000000000, ""},

		// missing
		{"missing", json.RawMessage(``), 0, msgRequired},

		// not an integer
		{"null", json.RawMessage(`null`), 0, msgRange}, // decodes to 0, so the range check rejects it
		{"decimal", json.RawMessage(`1000.5`), 0, msgInteger},
		{"whole-number decimal", json.RawMessage(`1.0`), 0, msgInteger},
		{"exponent", json.RawMessage(`1e3`), 0, msgInteger},
		{"numeric string", json.RawMessage(`"5"`), 0, msgInteger},
		{"non-numeric string", json.RawMessage(`"not an integer"`), 0, msgInteger},
		{"bool", json.RawMessage(`true`), 0, msgInteger},
		{"array", json.RawMessage(`[1, 2, 3]`), 0, msgInteger},
		{"object", json.RawMessage(`{"quantity": 123}`), 0, msgInteger},
		{"overflows int64", json.RawMessage(`99999999999999999999`), 0, msgInteger},

		// out of range
		{"zero", json.RawMessage(`0`), 0, msgRange},
		{"negative zero", json.RawMessage(`-0`), 0, msgRange},
		{"negative", json.RawMessage(`-5`), 0, msgRange},
		{"above max", json.RawMessage(`1000000001`), 0, msgRange},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseQuantity(tt.raw)

			if tt.wantMsg == "" {
				assert.Nil(t, err)
				assert.Equal(t, tt.wantQuantity, got)
				return
			}
			require.NotNil(t, err)
			assert.Equal(t, CodeInvalidQuantity, err.Code)
			assert.Equal(t, tt.wantMsg, err.Message)
			assert.Zero(t, got)
		})
	}
}

func TestParseTimestamp(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339) }
	const (
		msgRequired = "timestamp is required"
		msgFormat   = "timestamp must be RFC 3339 in UTC with a Z suffix, e.g. 2026-10-05T14:00:00Z"
		msgFuture   = "timestamp must not be more than 5 minutes in the future"
		msgPast     = "timestamp must not be older than 35 days"
	)

	tests := []struct {
		name          string
		s             string
		wantTimestamp time.Time
		wantMsg       string
	}{
		{"now", at(0), now, ""},
		{"max future, exact", at(maxFutureSkew), now.Add(maxFutureSkew), ""},
		{"max age, exact", at(-maxEventAge), now.Add(-maxEventAge), ""},
		{
			"fractional seconds", "2026-10-20T11:59:59.123Z",
			time.Date(2026, 10, 20, 11, 59, 59, 123_000_000, time.UTC), "",
		},
		{"empty", "", time.Time{}, msgRequired},
		{"no timezone", "2026-10-05T14:00:00", time.Time{}, msgFormat},
		{"offset +00:00", "2026-10-05T14:00:00+00:00", time.Time{}, msgFormat},
		{"offset +05:00", "2026-10-20T17:00:00+05:00", time.Time{}, msgFormat},
		{"lowercase z", "2026-10-20T12:00:00z", time.Time{}, msgFormat},
		{"date only", "2026-10-05", time.Time{}, msgFormat},
		{"garbage", "not a time", time.Time{}, msgFormat},
		{"garbage ending in Z", "notatimeZ", time.Time{}, msgFormat}, // passes the suffix check, fails time.Parse
		{"impossible values", "2026-13-45T25:61:00Z", time.Time{}, msgFormat},
		{"future by 1s", at(maxFutureSkew + time.Second), time.Time{}, msgFuture},
		{"too old by 1s", at(-maxEventAge - time.Second), time.Time{}, msgPast},
		{"zero date", "0001-01-01T00:00:00Z", time.Time{}, msgPast},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTimestamp(tt.s, now)

			if tt.wantMsg == "" {
				assert.Nil(t, err)
				assert.True(t, tt.wantTimestamp.Equal(got), "want %s, got %s", tt.wantTimestamp, got)
				return
			}
			require.NotNil(t, err)
			assert.Equal(t, CodeInvalidTimestamp, err.Code)
			assert.Equal(t, tt.wantMsg, err.Message)
			assert.True(t, got.IsZero())
		})
	}
}
