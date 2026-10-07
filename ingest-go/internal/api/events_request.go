package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	maxBatchSize = 500
	maxBodyBytes = 1 << 20 // 1 MB
)

type RequestError struct {
	Status  int
	Code    string
	Message string
}

func (e *RequestError) Error() string { return e.Message }

func parseEventsRequest(c *gin.Context) ([]json.RawMessage, *RequestError) {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return nil, &RequestError{http.StatusUnsupportedMediaType, CodeUnsupportedMediaType,
			"Content-Type must be application/json"}
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	dec := json.NewDecoder(c.Request.Body)

	var events []json.RawMessage
	if err := dec.Decode(&events); err != nil {
		var tooBig *http.MaxBytesError
		var typeErr *json.UnmarshalTypeError
		switch {
		case errors.As(err, &tooBig):
			return nil, &RequestError{http.StatusRequestEntityTooLarge, CodeBodyTooLarge,
				fmt.Sprintf("request body exceeds %d bytes", maxBodyBytes)}
		case errors.As(err, &typeErr):
			return nil, &RequestError{http.StatusBadRequest, CodeNotAnArray,
				"request body must be a JSON array of events"}
		case errors.Is(err, io.EOF):
			return nil, &RequestError{http.StatusBadRequest, CodeInvalidJSON, "request body is empty"}
		default:
			return nil, &RequestError{http.StatusBadRequest, CodeInvalidJSON, "request body is not valid JSON"}
		}
	}

	// Anything after the closing bracket is invalid.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, &RequestError{http.StatusBadRequest, CodeInvalidJSON, "unexpected data after JSON array"}
	}

	switch {
	case len(events) == 0:
		return nil, &RequestError{http.StatusBadRequest, CodeEmptyBatch, "batch must contain at least one event"}
	case len(events) > maxBatchSize:
		return nil, &RequestError{http.StatusRequestEntityTooLarge, CodeBatchTooLarge,
			fmt.Sprintf("batch exceeds %d events", maxBatchSize)}
	}

	return events, nil
}
