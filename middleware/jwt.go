package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/authn/jwt"
	"github.com/hydroan/gst/response"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

// abortUnauthenticatedJWT refuses the request with one fixed message and keeps
// the reason in the log.
//
// The reasons this layer rejects for are graded — malformed, expired, signed by
// something else — and answering each of them in its own words hands the bearer
// of a stolen token a probe it can read the server's checks off of. The holder
// of a valid token is told nothing by the distinction either, so only the log
// keeps it.
func abortUnauthenticatedJWT(c *gin.Context, err error) {
	zap.S().Warnw(
		"jwt authentication rejected",
		"error", err.Error(),
		"path", c.Request.URL.Path,
		"method", c.Request.Method,
	)
	response.Abort(c, http.StatusUnauthorized, "invalid token")
}

// JwtAuth authenticates a request from the bearer token in its Authorization
// header.
//
// The token answers for itself: it is verified from its signature and claims,
// with nothing read from storage. Revoking one before it expires therefore is
// not something this middleware can do, which is the trade a stateless token
// makes and the reason IAM's own sessions are not built on it.
func JwtAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		accessToken, claims, err := jwt.ParseTokenFromHeader(c.Request.Header)
		if err != nil {
			abortUnauthenticatedJWT(c, err)
			return
		}
		if err := jwt.Verify(claims, accessToken); err != nil {
			abortUnauthenticatedJWT(c, err)
			return
		}

		// Store the username of the current request in the request-scoped *gin.Context,
		// so that later handlers can read the current user through c.Get("username").
		c.Set(consts.CTX_USER_ID, claims.UserID)
		c.Set(consts.CTX_USERNAME, claims.Username)
		c.Set(consts.CTX_SESSION_ID, c.GetHeader("X-Session-Id"))
		c.Next()
	}
}
