package interceptor

import (
	"context"

	gstgrpc "github.com/hydroan/gst/grpc"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"google.golang.org/grpc/metadata"
)

// userAgentKey is the metadata the User-Agent header arrives as, in the
// lowercase gRPC metadata keeps its keys in. It is declared here rather than
// beside sessionIDKey because gg module copy copies this file alone into a
// project, where it must compile on its own.
const userAgentKey = "user-agent"

// IAMSession authenticates a call from the IAM session it names as
// "authorization: Bearer <session id>", through
// serviceiamsession.Authenticate, the one path middleware.IAMSession takes
// as well: the session is admitted or refused the same way over both
// listeners, the device it was established from checked against the call's
// user agent like the request's, and what differs here is only where the
// session id comes from, the metadata, and how a refusal is answered, as
// the gRPC status the service error maps to. A session is therefore only
// good to the client that established it: a program calling with one logs in
// presenting the user agent it calls with, "grpc-go/<version>" for a grpc-go
// client that sets none of its own, and a browser's session is refused here
// the way it is refused to another browser. The admitted session's user is
// the caller for the handler and the access log, and the session stays on
// the context for the actions that read it through
// serviceiamsession.CurrentSession.
func IAMSession() gstgrpc.Interceptor {
	return func(ctx context.Context) (context.Context, error) {
		sessionID, _ := gstgrpc.Bearer(ctx)
		md, _ := metadata.FromIncomingContext(ctx)
		var userAgent string
		if values := md.Get(userAgentKey); len(values) > 0 {
			userAgent = values[0]
		}
		httpMethod, route := gstgrpc.Route(ctx)
		current, err := serviceiamsession.Authenticate(ctx, sessionID, userAgent, httpMethod, route)
		if err != nil {
			return nil, gstgrpc.StatusError(err)
		}

		ctx = serviceiamsession.WithCurrentSession(ctx, sessionID, current)
		ctx = gstgrpc.WithCaller(ctx, gstgrpc.Caller{UserID: current.UserID, Username: current.Username, SessionID: sessionID, TenantID: current.TenantID})
		return ctx, nil
	}
}
