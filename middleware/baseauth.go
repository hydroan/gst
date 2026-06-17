package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
)

func BaseAuth() gin.HandlerFunc {
	return gin.BasicAuth(gin.Accounts{
		config.App.Auth.BaseAuthUsername: config.App.Auth.BaseAuthPassword,
	})
}
