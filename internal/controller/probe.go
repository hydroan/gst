package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type probe struct{}

var Probe = new(probe)

func (*probe) Healthz(c *gin.Context) {
	// c.Writer.WriteHeader(http.StatusOK)
	c.Status(http.StatusOK)
}

func (*probe) Readyz(c *gin.Context) {
	// c.Writer.WriteHeader(http.StatusOK)
	c.Status(http.StatusOK)
}
