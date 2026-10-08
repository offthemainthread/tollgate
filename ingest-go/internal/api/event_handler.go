package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct{}

func (h *Handler) PostEvents(c *gin.Context) {
	_, reqErr := parseEventsRequest(c)
	if reqErr != nil {
		c.JSON(reqErr.Status, ErrorResponse{Error: ErrorDetail{Code: reqErr.Code, Message: reqErr.Message}})
		return
	}
	c.JSON(http.StatusAccepted, EventsResponse{Results: []EventResult{}})
}
