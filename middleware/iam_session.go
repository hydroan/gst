package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/types/consts"
	"github.com/mssola/useragent"
)

// sessionRequiresPasswordChange reads the flag stored on the session snapshot.
func sessionRequiresPasswordChange(session modeliamsession.Session) bool {
	return session.MustChangePassword
}

// mustChangePasswordExemptRoutes are allowed while MustChangePassword is true on the session.
func mustChangePasswordExempt(method, path string) bool {
	switch {
	case method == http.MethodPost && path == "/api/iam/change-password":
		return true
	case method == http.MethodPost && path == "/api/logout":
		return true
	case method == http.MethodGet && path == "/api/iam/session/current":
		return true
	case method == http.MethodDelete && path == "/api/iam/session/current":
		return true
	case method == http.MethodPost && path == "/api/iam/session/heartbeat":
		return true
	default:
		return false
	}
}

func IAMSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		// fmt.Println("----- identifySession middleware", c.Request.RequestURI)
		sessionID, err := c.Cookie("session_id")
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no session"})
			return
		}
		session, e := redis.Cache[modeliamsession.Session]().WithContext(c.Request.Context()).Get(modeliamsession.SessionIDKey(sessionID))
		if e != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": e.Error()})
			return
		}

		// 校验浏览器/OS
		ua := useragent.New(c.Request.UserAgent())
		engineName, _ := ua.Engine()
		browserName, _ := ua.Browser()
		if session.OS != ua.OS() {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "os mismatch"})
			return
		}
		if session.Platform != ua.Platform() {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "platform mismatch"})
			return
		}
		if engineName != session.EngineName {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "engine mismatch"})
			return
		}
		if browserName != session.BrowserName {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "browser mismatch"})
			return
		}

		if sessionRequiresPasswordChange(session) && !mustChangePasswordExempt(c.Request.Method, c.Request.URL.Path) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "password change required before using this resource",
			})
			return
		}

		c.Set(consts.CTX_USER_ID, session.UserID)
		c.Set(consts.CTX_USERNAME, session.Username)
		c.Next()
	}
}
