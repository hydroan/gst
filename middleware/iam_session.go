package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types/consts"
	"github.com/mssola/useragent"
	"go.uber.org/zap"
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
	default:
		return false
	}
}

func IAMSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		// fmt.Println("----- identifySession middleware", c.Request.RequestURI)
		sessionID, err := c.Cookie(serviceiamsession.SessionCookieName)
		sessionID = strings.TrimSpace(sessionID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no session"})
			return
		}
		if sessionID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no session"})
			return
		}

		ctx := c.Request.Context()
		session, e := serviceiamsession.SessionManager.Load(ctx, sessionID)
		if e != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": e.Error()})
			return
		}
		if err = serviceiamsession.SessionManager.Validate(sessionID, session); err != nil {
			_, _ = serviceiamsession.SessionManager.Delete(ctx, sessionID)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
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

		if session, err = serviceiamsession.ValidateSessionUserState(ctx, session); err != nil {
			_, _ = serviceiamsession.SessionManager.Delete(ctx, sessionID)
			status := http.StatusForbidden
			var serviceErr *service.Error
			if errors.As(err, &serviceErr) {
				status = serviceErr.Status()
			}
			c.AbortWithStatusJSON(status, gin.H{"error": err.Error()})
			return
		}

		if sessionRequiresPasswordChange(session) && !mustChangePasswordExempt(c.Request.Method, c.Request.URL.Path) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "password change required before using this resource",
			})
			return
		}

		if err = serviceiamsession.TouchSession(ctx, sessionID, session, time.Now()); err != nil {
			zap.S().Warnw("failed to touch iam session", "session_id", sessionID, "error", err)
		}

		c.Request = c.Request.WithContext(serviceiamsession.WithCurrentSession(ctx, sessionID, session))
		c.Set(consts.CTX_USER_ID, session.UserID)
		c.Set(consts.CTX_USERNAME, session.Username)
		c.Set(consts.CTX_SESSION_ID, sessionID)
		c.Set(consts.CTX_TENANT_ID, session.TenantID)
		c.Next()
	}
}
