package interceptor

import (
	"context"

	"github.com/hydroan/gst/authn/jwt"
	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/internal/requestctx"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// sessionIDKey is the metadata carrying the session id JwtAuth pairs with a
// token, the X-Session-Id header of the HTTP listener, in the lowercase gRPC
// metadata keeps its keys in.
const sessionIDKey = "x-session-id"

// JwtAuth authenticates a call from the bearer token in its authorization
// metadata, the way middleware.JwtAuth authenticates a request from the
// Authorization header, with the same token: "authorization: Bearer
// <token>", the x-session-id metadata standing for the X-Session-Id header.
//
// The token answers for itself: it is verified from its signature and
// claims, with nothing read from storage. Revoking one before it expires
// therefore is not something this interceptor can do, which is the trade a
// stateless token makes and the reason IAM's own sessions are not built on
// it.
func JwtAuth() gstgrpc.Interceptor {
	return func(ctx context.Context) (context.Context, error) {
		token, ok := gstgrpc.Bearer(ctx)
		var claims *jwt.Claims
		err := jwt.ErrInvalidToken
		if ok {
			claims, err = jwt.ParseToken(token)
		}
		if err == nil {
			err = jwt.Verify(claims)
		}
		if err != nil {
			// The reasons this layer rejects for are graded — missing,
			// malformed, expired, signed by something else — and answering
			// each in its own words hands the bearer of a stolen token a probe
			// it can read the server's checks off of, so only the log keeps
			// the distinction.
			zap.S().Warnw("jwt authentication rejected", "error", err.Error(), "method", requestctx.FromContext(ctx).Path())
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}
		var sessionID string
		if md, _ := metadata.FromIncomingContext(ctx); len(md.Get(sessionIDKey)) > 0 {
			sessionID = md.Get(sessionIDKey)[0]
		}
		return gstgrpc.WithCaller(ctx, gstgrpc.Caller{UserID: claims.UserID, Username: claims.Username, SessionID: sessionID}), nil
	}
}
