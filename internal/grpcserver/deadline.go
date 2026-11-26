package grpcserver

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/i18n"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// deadline returns the interceptor holding a unary call to timeout, the time
// the server gives a request to be answered in, which chains takes from
// server.write_timeout: the counterpart of the deadline of the HTTP
// listener's built-in chain. The context of the call ends at the deadline,
// or sooner when the client gave the call less time, the context carrying
// the client's deadline already. A call whose time is up when its handler
// returns is answered DeadlineExceeded with gst.TimeoutMsg, in the language
// the process answers in, whatever the handler returned: the answer of a
// handler that stopped when its context ended and the late one of a handler
// that ignored the context alike, the way the HTTP listener drops a response
// started after the deadline for its 504. timeout is positive.
func deadline(timeout time.Duration) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		rsp, err := handler(ctx, req)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, status.Error(codes.DeadlineExceeded, i18n.Translate(gst.TimeoutMsg))
		}
		return rsp, err
	}
}
