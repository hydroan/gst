package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/requestctx"
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

// abortInvalidSession refuses the request with one fixed message and keeps the
// reason in the log.
//
// The reasons this layer rejects for are graded — a snapshot storage no longer
// has, one that expired, one issued to another browser or another OS — and
// answering each of them in its own words hands the bearer of a stolen cookie a
// probe: it can vary one component of the request at a time and read back which
// one the server objected to, which is the session's binding described to the
// one caller who must not learn it. The holder of a live session is told
// nothing by the distinction either, since every one of these is answered by
// logging in again, so only the log keeps it.
func abortInvalidSession(c *gin.Context, reason string) {
	zap.S().Warnw(
		"iam session rejected",
		"reason", reason,
		"path", c.Request.URL.Path,
		"method", c.Request.Method,
	)
	response.Abort(c, http.StatusUnauthorized, "session invalid")
}

func IAMSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID, err := c.Cookie(serviceiamsession.SessionCookieName)
		sessionID = strings.TrimSpace(sessionID)
		if err != nil || sessionID == "" {
			response.Abort(c, http.StatusUnauthorized, "no session")
			return
		}

		// Every storage call below runs on this context, so it carries the
		// request metadata that lets their logs name a route. The identity
		// fields are empty by design: this middleware is what resolves who is
		// calling, and it publishes that onto the gin context only once every
		// check below has passed.
		ctx := requestctx.WithGinMetadata(c)
		session, e := serviceiamsession.Store.LoadSession(ctx, sessionID)
		if e != nil {
			abortInvalidSession(c, e.Error())
			return
		}
		if err = serviceiamsession.ValidateSession(sessionID, session); err != nil {
			_, _ = serviceiamsession.Store.DeleteSession(ctx, sessionID)
			abortInvalidSession(c, err.Error())
			return
		}

		// verify the browser and OS
		ua := useragent.New(c.Request.UserAgent())
		engineName, _ := ua.Engine()
		browserName, _ := ua.Browser()
		if session.OS != ua.OS() {
			abortInvalidSession(c, "os mismatch")
			return
		}
		if session.Platform != ua.Platform() {
			abortInvalidSession(c, "platform mismatch")
			return
		}
		if engineName != session.EngineName {
			abortInvalidSession(c, "engine mismatch")
			return
		}
		if browserName != session.BrowserName {
			abortInvalidSession(c, "browser mismatch")
			return
		}

		if session, err = serviceiamsession.ValidateSessionUserState(ctx, session); err != nil {
			_, _ = serviceiamsession.Store.DeleteSession(ctx, sessionID)
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

		if err = serviceiamsession.Store.TouchSession(ctx, sessionID, session, time.Now()); err != nil {
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
