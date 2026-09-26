package middleware

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/requestctx"
	"github.com/hydroan/gst/response"
	"github.com/hydroan/gst/service"
)

// IAMSession authenticates a request from the IAM session its cookie names,
// through serviceiamsession.Authenticate, the one path the gRPC interceptor
// of the module takes as well: the session is admitted or refused the same
// way over both listeners, and what differs here is only where the session
// id comes from, the cookie, and how a refusal is answered, in the API
// envelope with the status and message the service error carries.
func IAMSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID, _ := c.Cookie(serviceiamsession.SessionCookieName)

		// Every storage call runs on this context, so it carries the request
		// metadata that lets their logs name a route. The identity fields are
		// empty by design: this middleware is what resolves who is calling,
		// and it publishes that onto the gin context only once every check
		// has passed.
		ctx := requestctx.WithGinMetadata(c)
		current, err := serviceiamsession.Authenticate(ctx, sessionID, c.Request.UserAgent(), c.Request.Method, c.Request.URL.Path)
		if err != nil {
			status, msg := http.StatusInternalServerError, "internal server error"
			var serviceErr *service.Error
			if errors.As(err, &serviceErr) {
				status, msg = serviceErr.Status(), serviceErr.Msg()
			}
			response.Abort(c, status, msg)
			return
		}

		c.Request = c.Request.WithContext(serviceiamsession.WithCurrentSession(ctx, sessionID, current))
		c.Set(consts.CTX_USER_ID, current.UserID)
		c.Set(consts.CTX_USERNAME, current.Username)
		c.Set(consts.CTX_SESSION_ID, sessionID)
		c.Set(consts.CTX_TENANT_ID, current.TenantID)
	}
}
