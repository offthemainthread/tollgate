package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCtx(body, contentType string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body))
	if contentType != "" {
		c.Request.Header.Set("Content-Type", contentType)
	}
	return c
}

func TestParseEventsRequest_Errors(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		contentType string
		wantStatus  int
		wantCode    string
	}{
		{"no content type", "[{}]", "", http.StatusUnsupportedMediaType, CodeUnsupportedMediaType},
		{"xml content type", "[{}]", "application/xml", http.StatusUnsupportedMediaType, CodeUnsupportedMediaType},
		{"text/plain content type", "[{}]", "text/plain", http.StatusUnsupportedMediaType, CodeUnsupportedMediaType},
		{"empty body", "", "application/json", http.StatusBadRequest, CodeInvalidJSON},
		{"truncated array", "[", "application/json", http.StatusBadRequest, CodeInvalidJSON},
		{"garbage", "not json", "application/json", http.StatusBadRequest, CodeInvalidJSON},
		{"trailing data", "[{}] junk", "application/json", http.StatusBadRequest, CodeInvalidJSON},
		{"second array after first", "[{}][{}]", "application/json", http.StatusBadRequest, CodeInvalidJSON},
		{"object instead of array", `{"a":1}`, "application/json", http.StatusBadRequest, CodeNotAnArray},
		{"string instead of array", `"hello"`, "application/json", http.StatusBadRequest, CodeNotAnArray},
		{"number instead of array", `42`, "application/json", http.StatusBadRequest, CodeNotAnArray},
		{"empty array", "[]", "application/json", http.StatusBadRequest, CodeEmptyBatch},
		{"null", "null", "application/json", http.StatusBadRequest, CodeEmptyBatch},
		{
			"too many events",
			"[" + strings.Repeat("{},", maxBatchSize) + "{}]",
			"application/json",
			http.StatusRequestEntityTooLarge, CodeBatchTooLarge,
		},
		{
			"body too large",
			`["` + strings.Repeat("a", maxBodyBytes) + `"]`,
			"application/json",
			http.StatusRequestEntityTooLarge, CodeBodyTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := parseEventsRequest(newCtx(tt.body, tt.contentType))

			require.NotNil(t, err)
			assert.Nil(t, events)
			assert.Equal(t, tt.wantStatus, err.Status)
			assert.Equal(t, tt.wantCode, err.Code)
			assert.NotEmpty(t, err.Message)
		})
	}
}

func TestParseEventsRequest_ContentTypeMessage(t *testing.T) {
	_, err := parseEventsRequest(newCtx("[{}]", "application/xml"))

	expected := &RequestError{
		Status:  http.StatusUnsupportedMediaType,
		Code:    CodeUnsupportedMediaType,
		Message: "Content-Type must be application/json",
	}
	assert.Equal(t, expected, err)
}

func TestParseEventsRequest_Valid(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		contentType string
		wantLen     int
	}{
		{"one event", `[{"idempotency_key":"a"}]`, "application/json", 1},
		{"two events", `[{"idempotency_key":"a"},{"idempotency_key":"b"}]`, "application/json", 2},
		{"charset parameter", `[{}]`, "application/json; charset=utf-8", 1},
		{"mixed-case media type", `[{}]`, "Application/JSON", 1},
		{"max batch size", "[" + strings.Repeat("{},", maxBatchSize-1) + "{}]", "application/json", maxBatchSize},
		{"surrounding whitespace", " \n[{}]\n ", "application/json", 1},
		{"non-object elements still pass", `[1, "x", null]`, "application/json", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := parseEventsRequest(newCtx(tt.body, tt.contentType))

			require.Nil(t, err)
			assert.Len(t, events, tt.wantLen)
		})
	}
}
