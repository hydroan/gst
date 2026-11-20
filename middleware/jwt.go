package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/apidoc"
	"github.com/hydroan/gst/authn/jwt"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
)

// JwtAuth authenticates a request from the bearer token in its Authorization
// header, and the X-Session-Id header as its session (see
// jwt.HeaderSessionID). A request it refuses, its token missing, malformed,
// expired or signed by something else, is answered 401 with one fixed
// message, the reason kept to the log (see jwt.LogRefusal).
//
// The token answers for itself: it is verified from its signature and claims,
// with nothing read from storage. Revoking one before it expires therefore is
// not something this middleware can do, which is the trade a stateless token
// makes and the reason IAM's own sessions are not built on it. Building the
// middleware declares the token to the OpenAPI document as the bearerAuth
// scheme.
func JwtAuth() gin.HandlerFunc {
	apidoc.RegisterSecurityScheme("bearerAuth", apidoc.SecurityScheme{Type: "http", Scheme: "bearer"})
	return func(c *gin.Context) {
		_, claims, err := jwt.ParseTokenFromHeader(c.Request.Header)
		if err == nil {
			err = jwt.Verify(claims)
		}
		if err != nil {
			response.Abort(c, types.NewError(http.StatusUnauthorized, jwt.LogRefusal(err, c.Request.URL.Path, c.Request.Method)))
			return
		}

		// Store the username of the current request in the request-scoped *gin.Context,
		// so that later handlers can read the current user through c.Get("username").
		c.Set(requestctx.KeyUserID, claims.UserID)
		c.Set(requestctx.KeyUsername, claims.Username)
		c.Set(requestctx.KeySessionID, c.GetHeader(jwt.HeaderSessionID))
	}
}
