package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
)

// BaseAuth guards the documentation endpoints with HTTP Basic authentication.
//
// Its refusal is gin's own: a bare 401 carrying WWW-Authenticate and no body,
// not the API envelope every other refusal answers in. That is the point — the
// header is what makes a browser open its credentials dialog, and these
// endpoints are opened by a person in a browser rather than by a client parsing
// responses.
func BaseAuth() gin.HandlerFunc {
	return gin.BasicAuth(gin.Accounts{
		config.App.Auth.BaseAuthUsername: config.App.Auth.BaseAuthPassword,
	})
}
