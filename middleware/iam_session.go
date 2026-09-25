package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/requestctx"
	"github.com/hydroan/gst/response"
	"github.com/hydroan/gst/service"
	"github.com/mssola/useragent"
	"go.uber.org/zap"
)

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
		current, e := serviceiamsession.Store.LoadSession(ctx, sessionID)
		if e != nil {
			abortInvalidSession(c, e.Error())
			return
		}
		if err = serviceiamsession.ValidateSession(sessionID, current); err != nil {
			_, _ = serviceiamsession.Store.DeleteSession(ctx, sessionID)
			abortInvalidSession(c, err.Error())
			return
		}

		// verify the browser and OS
		ua := useragent.New(c.Request.UserAgent())
		engineName, _ := ua.Engine()
		browserName, _ := ua.Browser()
		if current.OS != ua.OS() {
			abortInvalidSession(c, "os mismatch")
			return
		}
		if current.Platform != ua.Platform() {
			abortInvalidSession(c, "platform mismatch")
			return
		}
		if engineName != current.EngineName {
			abortInvalidSession(c, "engine mismatch")
			return
		}
		if browserName != current.BrowserName {
			abortInvalidSession(c, "browser mismatch")
			return
		}

		if current, err = serviceiamsession.ValidateSessionUserState(ctx, current); err != nil {
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

		if current.MustChangePassword && !serviceiamsession.MustChangePasswordExempt(c.Request.Method, c.Request.URL.Path) {
			response.Abort(c, http.StatusForbidden, "password change required before using this resource")
			return
		}

		if err = serviceiamsession.Store.TouchSession(ctx, sessionID, current, time.Now()); err != nil {
			zap.S().Warnw("failed to touch iam session", "session_id", sessionID, "error", err)
		}

		c.Request = c.Request.WithContext(serviceiamsession.WithCurrentSession(ctx, sessionID, current))
		c.Set(consts.CTX_USER_ID, current.UserID)
		c.Set(consts.CTX_USERNAME, current.Username)
		c.Set(consts.CTX_SESSION_ID, sessionID)
		c.Set(consts.CTX_TENANT_ID, current.TenantID)
	}
}
