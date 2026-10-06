package grpcserver

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// recovered is the recovery handler of both chains. It records the panic on
// the call's span the way the HTTP recovery middleware does on the request's,
// logs it with the method, the trace id and the stack to the recovery log,
// the one the HTTP listener's panics go to, and answers codes.Internal with
// the fixed message StatusError answers the server's own failures with;
// what the panic was stays in the log and the span, and the caller quotes
// back the trace id the response header carries.
func recovered(ctx context.Context, p any) error {
	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		gstotel.RecordError(span, errors.Newf("panic recovered: %v", p))
		span.SetAttributes(
			attribute.Bool("error.panic", true),
			attribute.String("error.recovered", fmt.Sprintf("%v", p)),
		)
	}
	if logger.Recovery != nil {
		method, _ := grpc.Method(ctx)
		logger.Recovery.Error(
			fmt.Sprintf("[recovery] panic recovered:\n%s\n%v\n%s", method, p, debug.Stack()),
			zap.String(consts.TRACE_ID, execctx.FromContext(ctx).TraceID),
		)
	}
	return status.Error(codes.Internal, types.FailureMsg)
}
