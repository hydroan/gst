package interceptor

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	gstgrpc "github.com/hydroan/gst/grpc"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/service"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// IAMSession authenticates a call from the IAM session it names as
// "authorization: Bearer <session id>", the way middleware.IAMSession
// authenticates a request from the session cookie: the session is loaded
// from the store and validated, the state of its user checked, and its user
// established as the caller for the handler and the access log, with the
// session kept on the context for the actions that read it. While the
// session requires a password change, only the actions a user needs for
// that are admitted, judged by the HTTP method and route the call's action
// is served at (see serviceiamsession.MustChangePasswordExempt). The
// browser and operating system the middleware binds a session to are not
// checked: a gRPC client has neither.
//
// The refusals carry the middleware's fixed messages and keep the reason in
// the log, for the reason the middleware gives: the bearer of a stolen
// session id must not learn which check objected.
func IAMSession() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		sessionID, ok := gstgrpc.Bearer(ctx)
		if !ok || sessionID == "" {
			return nil, status.Error(codes.Unauthenticated, "no session")
		}

		current, err := serviceiamsession.Store.LoadSession(ctx, sessionID)
		if err != nil {
			return nil, rejectSession(info, err.Error())
		}
		if err = serviceiamsession.ValidateSession(sessionID, current); err != nil {
			_, _ = serviceiamsession.Store.DeleteSession(ctx, sessionID)
			return nil, rejectSession(info, err.Error())
		}
		if current, err = serviceiamsession.ValidateSessionUserState(ctx, current); err != nil {
			_, _ = serviceiamsession.Store.DeleteSession(ctx, sessionID)
			// A service error carries a status and a message written for the
			// client. Anything else is an internal failure whose text belongs
			// in logs, not in the answer.
			var serviceErr *service.Error
			if errors.As(err, &serviceErr) {
				return nil, gstgrpc.StatusError(serviceErr)
			}
			zap.S().Warnw("iam session rejected", "reason", err.Error(), "method", info.FullMethod)
			return nil, status.Error(codes.PermissionDenied, "session invalid")
		}
		if current.MustChangePassword {
			if httpMethod, route := gstgrpc.Route(ctx); !serviceiamsession.MustChangePasswordExempt(httpMethod, route) {
				return nil, status.Error(codes.PermissionDenied, "password change required before using this resource")
			}
		}

		if err = serviceiamsession.Store.TouchSession(ctx, sessionID, current, time.Now()); err != nil {
			zap.S().Warnw("failed to touch iam session", "session_id", sessionID, "error", err)
		}
		ctx = serviceiamsession.WithCurrentSession(ctx, sessionID, current)
		ctx = gstgrpc.WithCaller(ctx, gstgrpc.Caller{UserID: current.UserID, Username: current.Username, SessionID: sessionID, TenantID: current.TenantID})
		return handler(ctx, req)
	}
}

// rejectSession keeps why a session was rejected in the log and answers
// Unauthenticated with the one fixed message.
func rejectSession(info *grpc.UnaryServerInfo, reason string) error {
	zap.S().Warnw("iam session rejected", "reason", reason, "method", info.FullMethod)
	return status.Error(codes.Unauthenticated, "session invalid")
}
