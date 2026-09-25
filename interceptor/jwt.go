package interceptor

import (
	"context"
	"strings"

	"github.com/hydroan/gst/authn/jwt"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

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
func JwtAuth() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		claims, err := parseBearer(md)
		if err == nil {
			err = jwt.Verify(claims)
		}
		if err != nil {
			// The reasons this layer rejects for are graded — missing,
			// malformed, expired, signed by something else — and answering
			// each in its own words hands the bearer of a stolen token a probe
			// it can read the server's checks off of, so only the log keeps
			// the distinction.
			zap.S().Warnw("jwt authentication rejected", "error", err.Error(), "method", info.FullMethod)
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}
		var sessionID string
		if values := md.Get("x-session-id"); len(values) > 0 {
			sessionID = values[0]
		}
		return handler(WithIdentity(ctx, Identity{UserID: claims.UserID, Username: claims.Username, SessionID: sessionID}), req)
	}
}

// parseBearer parses the token the authorization metadata carries as
// "Bearer <token>", the check jwt.ParseTokenFromHeader makes of the
// Authorization header.
func parseBearer(md metadata.MD) (*jwt.Claims, error) {
	values := md.Get("authorization")
	if len(values) == 0 {
		return nil, jwt.ErrInvalidToken
	}
	scheme, token, found := strings.Cut(values[0], " ")
	if !found || scheme != "Bearer" {
		return nil, jwt.ErrInvalidToken
	}
	return jwt.ParseToken(token)
}
