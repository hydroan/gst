package interceptor

import (
	"context"

	gstgrpc "github.com/hydroan/gst/grpc"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

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
// the context for the actions that read it.
func IAMSession() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
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
		return handler(ctx, req)
	}
}
