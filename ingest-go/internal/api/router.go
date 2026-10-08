package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func NewRouter(handler *Handler) *gin.Engine {
	r := gin.Default()

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "OK"})
	})

	r.POST("/v1/events", handler.PostEvents)

	return r
}
