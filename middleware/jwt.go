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
// The reasons this layer rejects for are graded — malformed, expired, issued
// for another browser, for another OS — and answering each of them in its own
// words hands a bearer of a stolen token a probe: it can vary one component of
// the request at a time and read back which one the server objected to, which
// is the token's binding described to the one caller who should not learn it.
// The holder of a valid token is told nothing by the distinction either, so
// only the log keeps it.
func abortUnauthenticatedJWT(c *gin.Context, err error) {
	zap.S().Warnw(
		"jwt authentication rejected",
		"error", err.Error(),
		"path", c.Request.URL.Path,
		"method", c.Request.Method,
	)
	response.Abort(c, http.StatusUnauthorized, "invalid token")
}

// JwtAuth behaves as follows:
//  1. Logging in again refreshes the accessToken and refreshToken, which invalidates the old accessToken.
//  2. Switching browser or operating system requires a new login, and that login evicts the
//     sessions on other devices and browsers.
func JwtAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		accessToken, claims, err := jwt.ParseTokenFromHeader(c.Request.Header)
		if err != nil {
			abortUnauthenticatedJWT(c, err)
			return
		}
		if err := jwt.Verify(claims, accessToken, c.Request.UserAgent()); err != nil {
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
