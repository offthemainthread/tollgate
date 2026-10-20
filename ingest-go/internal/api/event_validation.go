package api

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const maxIdentifierLen = 255
const minQuantity = 1
const maxQuantity = 1_000_000_000
const maxFutureSkew = 5 * time.Minute
const maxEventAge = 35 * 24 * time.Hour

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)

func validateIdentifier(value, field, code string) *ErrorDetail {
	if value == "" {
		return &ErrorDetail{Code: code, Message: field + " is required"}
	}
	if len(value) > maxIdentifierLen {
		return &ErrorDetail{Code: code, Message: fmt.Sprintf("%s is too long (max %d characters)", field, maxIdentifierLen)}
	}
	if !identifierPattern.MatchString(value) {
		return &ErrorDetail{Code: code, Message: field + " may only contain letters, digits, and _ . : -"}
	}
	return nil
}

func validateIdempotencyKey(key string) *ErrorDetail {
	return validateIdentifier(key, "idempotency key", CodeInvalidIdempotencyKey)
}

func validateMeter(meter string) *ErrorDetail {
	return validateIdentifier(meter, "meter", CodeUnknownMeter)
}

// parseQuantity decodes the raw "quantity" value from one event.
// It returns the parsed value, or an *ErrorDetail with CodeInvalidQuantity.
func parseQuantity(raw json.RawMessage) (int64, *ErrorDetail) {
	if len(raw) == 0 {
		return 0, &ErrorDetail{
			Code:    CodeInvalidQuantity,
			Message: "quantity is required",
		}
	}
	quantity := int64(0)
	err := json.Unmarshal(raw, &quantity)
	if err != nil {
		return 0, &ErrorDetail{
			Code:    CodeInvalidQuantity,
			Message: "quantity must be an integer",
		}
	}
	if quantity < minQuantity || quantity > maxQuantity {
		return 0, &ErrorDetail{
			Code:    CodeInvalidQuantity,
			Message: fmt.Sprintf("quantity must be between %d and %d", minQuantity, maxQuantity),
		}
	}
	return quantity, nil
}

func parseTimestamp(s string, now time.Time) (time.Time, *ErrorDetail) {
	if s == "" {
		return time.Time{}, &ErrorDetail{
			Code:    CodeInvalidTimestamp,
			Message: "timestamp is required",
		}
	}
	const msgTimestampFormat = "timestamp must be RFC 3339 in UTC with a Z suffix, e.g. 2026-10-05T14:00:00Z"
	if !strings.HasSuffix(s, "Z") {
		return time.Time{}, &ErrorDetail{
			Code:    CodeInvalidTimestamp,
			Message: msgTimestampFormat,
		}
	}
	parsedTime, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, &ErrorDetail{
			Code:    CodeInvalidTimestamp,
			Message: msgTimestampFormat,
		}
	}
	if parsedTime.After(now.Add(maxFutureSkew)) {
		return time.Time{}, &ErrorDetail{
			Code:    CodeInvalidTimestamp,
			Message: fmt.Sprintf("timestamp must not be more than %d minutes in the future", int(maxFutureSkew.Minutes())),
		}
	}
	if parsedTime.Before(now.Add(-maxEventAge)) {
		return time.Time{}, &ErrorDetail{
			Code:    CodeInvalidTimestamp,
			Message: fmt.Sprintf("timestamp must not be older than %d days", int(maxEventAge.Hours()/24)),
		}
	}
	return parsedTime.UTC(), nil
}
