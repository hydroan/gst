package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/response"
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
		sessionID, err := c.Cookie(serviceiamsession.SessionCookieName)
		sessionID = strings.TrimSpace(sessionID)
		if err != nil || sessionID == "" {
			response.Abort(c, http.StatusUnauthorized, "no session")
			return
		}

		ctx := c.Request.Context()
		// A load that fails is answered with one fixed message. The cause is a
		// storage detail — a snapshot that expired, a cache that has no such
		// entry, a backend that did not answer — and none of it is the
		// caller's: what the caller can act on is that this cookie no longer
		// names a live session.
		session, e := serviceiamsession.SessionManager.Load(ctx, sessionID)
		if e != nil {
			response.Abort(c, http.StatusUnauthorized, "session invalid")
			return
		}
		// Validate speaks in client-safe terms — expired, not active — so its
		// message passes through as it stands.
		if err = serviceiamsession.SessionManager.Validate(sessionID, session); err != nil {
			_, _ = serviceiamsession.SessionManager.Delete(ctx, sessionID)
			response.Abort(c, http.StatusUnauthorized, err.Error())
			return
		}

		// verify the browser and OS
		ua := useragent.New(c.Request.UserAgent())
		engineName, _ := ua.Engine()
		browserName, _ := ua.Browser()
		if session.OS != ua.OS() {
			response.Abort(c, http.StatusUnauthorized, "os mismatch")
			return
		}
		if session.Platform != ua.Platform() {
			response.Abort(c, http.StatusUnauthorized, "platform mismatch")
			return
		}
		if engineName != session.EngineName {
			response.Abort(c, http.StatusUnauthorized, "engine mismatch")
			return
		}
		if browserName != session.BrowserName {
			response.Abort(c, http.StatusUnauthorized, "browser mismatch")
			return
		}

		if session, err = serviceiamsession.ValidateSessionUserState(ctx, session); err != nil {
			_, _ = serviceiamsession.SessionManager.Delete(ctx, sessionID)
			// A service error carries a status and a message written for the
			// client. Anything else is an internal failure whose text belongs
			// in logs, not in the response.
			status, msg := http.StatusForbidden, "session invalid"
			var serviceErr *service.Error
			if errors.As(err, &serviceErr) {
				status, msg = serviceErr.Status(), serviceErr.Msg()
			}
			response.Abort(c, status, msg)
			return
		}

		if sessionRequiresPasswordChange(session) && !mustChangePasswordExempt(c.Request.Method, c.Request.URL.Path) {
			response.Abort(c, http.StatusForbidden, "password change required before using this resource")
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
