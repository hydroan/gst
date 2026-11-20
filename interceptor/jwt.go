package interceptor

import (
	"context"
	"strings"

	"github.com/hydroan/gst/authn/jwt"
	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/internal/requestctx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// keySessionID is the metadata the session id arrives under:
// jwt.HeaderSessionID, lowercased once here. Metadata lowercases a key on
// every read, and allocates to do it when the key is not lowercase already.
var keySessionID = strings.ToLower(jwt.HeaderSessionID)

// JwtAuth authenticates a call from the bearer token in its authorization
// metadata, the way middleware.JwtAuth authenticates a request from the
// Authorization header, with the same token: "authorization: Bearer <token>",
// the x-session-id metadata standing for the X-Session-Id header (see
// jwt.HeaderSessionID). A call it refuses, its token missing, malformed,
// expired or signed by something else, is answered Unauthenticated with one
// fixed message, the reason kept to the log (see jwt.LogRefusal).
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
			meta := requestctx.FromContext(ctx)
			return nil, status.Error(codes.Unauthenticated, jwt.LogRefusal(err, meta.Path(), meta.Method()))
		}
		var sessionID string
		if md, _ := metadata.FromIncomingContext(ctx); len(md.Get(keySessionID)) > 0 {
			sessionID = md.Get(keySessionID)[0]
		}
		return gstgrpc.WithCaller(ctx, gstgrpc.Caller{UserID: claims.UserID, Username: claims.Username, SessionID: sessionID}), nil
	}
}
