package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz(t *testing.T) {
	r := NewRouter(&Handler{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status OK; got %d", w.Code)
	}
}
