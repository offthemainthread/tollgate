package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func postEvents(t *testing.T, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	NewRouter(&Handler{}).ServeHTTP(w, req)
	return w
}

func TestPostEvents_Errors(t *testing.T) {
	tests := []struct {
		name, contentType, body string
		wantStatus              int
		wantCode                string
	}{
		{"wrong content type", "text/plain", `[{}]`, http.StatusUnsupportedMediaType, CodeUnsupportedMediaType},
		{"invalid json", "application/json", `[`, http.StatusBadRequest, CodeInvalidJSON},
		{"not an array", "application/json", `{}`, http.StatusBadRequest, CodeNotAnArray},
		{"empty batch", "application/json", `[]`, http.StatusBadRequest, CodeEmptyBatch},
		{
			"batch too large", "application/json",
			"[" + strings.Repeat("{},", maxBatchSize) + "{}]",
			http.StatusRequestEntityTooLarge, CodeBatchTooLarge,
		},
		{
			"body too large", "application/json",
			`["` + strings.Repeat("a", maxBodyBytes) + `"]`,
			http.StatusRequestEntityTooLarge, CodeBodyTooLarge,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := postEvents(t, tt.contentType, tt.body)

			assert.Equal(t, tt.wantStatus, w.Code)
			var resp ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, tt.wantCode, resp.Error.Code)
			assert.NotEmpty(t, resp.Error.Message)
		})
	}
}

func TestPostEvents_Accepted(t *testing.T) {
	w := postEvents(t, "application/json", `[{}]`)

	assert.Equal(t, http.StatusAccepted, w.Code)
	// "results" must be an empty array, never null, so clients can always iterate it.
	assert.JSONEq(t, `{"results":[]}`, w.Body.String())
}
