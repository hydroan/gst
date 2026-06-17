package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/middleware"
)

func init() {
	middleware.Register(Middleware1, Middleware2, Middleware3)
}

func Middleware1(c *gin.Context) {
	c.Set("name", "middleware1")
}

func Middleware2(c *gin.Context) {
	c.Set("name", "middleware2")
}

func Middleware3(c *gin.Context) {
	c.Set("name", "middleware3")
}
