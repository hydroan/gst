package grpcserver

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// recovered is the recovery handler of both chains. It logs the panic with
// the method, the trace id and the stack to the recovery log, the one the
// HTTP listener's panics go to, and answers codes.Internal with the message
// the HTTP envelope carries for the same case; what the panic was stays in
// the log, and the caller quotes back the trace id the response header
// carries.
func recovered(ctx context.Context, p any) error {
	if logger.Recovery != nil {
		method, _ := grpc.Method(ctx)
		logger.Recovery.Error(
			fmt.Sprintf("[recovery] panic recovered:\n%s\n%v\n%s", method, p, debug.Stack()),
			zap.String(consts.TRACE_ID, execctx.FromContext(ctx).TraceID),
		)
	}
	return status.Error(codes.Internal, "internal server error")
}
