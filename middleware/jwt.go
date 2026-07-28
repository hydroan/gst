package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/authn/jwt"
	"github.com/hydroan/gst/types/consts"
)

// JwtAuth behaves as follows:
//  1. Logging in again refreshes the accessToken and refreshToken, which invalidates the old accessToken.
//  2. Switching browser or operating system requires a new login, and that login evicts the
//     sessions on other devices and browsers.
func JwtAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		accessToken, claims, err := jwt.ParseTokenFromHeader(c.Request.Header)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":          -1,
				"msg":           err.Error(),
				"data":          nil,
				consts.TRACE_ID: c.GetString(consts.TRACE_ID),
			})
			return
		}
		if err := jwt.Verify(claims, accessToken, c.Request.UserAgent()); err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":          -1,
				"msg":           err.Error(),
				"data":          nil,
				consts.TRACE_ID: c.GetString(consts.TRACE_ID),
			})
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
