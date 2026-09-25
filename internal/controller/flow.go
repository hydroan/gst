package controller

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/execctx"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/requestctx"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	gstotel "github.com/hydroan/gst/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// This file holds what the CRUD flows share. A flow is the half of a handler
// that does not depend on the transport the request arrived on: the service
// hooks, the database access and the operation log, in the order the action
// defines. It runs on a context carrying the request metadata (see
// requestctx) and the controller span, and builds the service contexts of
// its hooks through a function the transport supplies, which is all a
// transport contributes to it. The HTTP handlers of this package bind the
// request, run the flow and write its answer; another transport does the
// same with a binding and a writing of its own.

// serviceContextFunc builds the service context of one hook or service call
// from the context of that call and its phase. Only the transport knows what
// else the service context should carry -- the HTTP handler's gin context,
// nothing at all for a transport without one -- so the transport supplies it.
type serviceContextFunc func(ctx context.Context, phase consts.Phase) *types.ServiceContext

// ginServiceContext returns the serviceContextFunc of the HTTP handler
// serving c: its service contexts carry the gin context, so the response
// helpers of ServiceContext keep working inside the hooks.
func ginServiceContext(c *gin.Context) serviceContextFunc {
	return func(ctx context.Context, phase consts.Phase) *types.ServiceContext {
		return types.NewServiceContext(c, ctx, phase)
	}
}

// failure is the error a flow returns for a request it could not serve: the
// canonical code answering it, and the error behind, when there is one. The
// flow has logged the error and recorded it on the controller span before
// returning, so the transport only renders the code.
type failure struct {
	coder types.Coder
	err   error
}

func (f *failure) Error() string {
	if f.err != nil {
		return f.err.Error()
	}
	return f.coder.Msg()
}

func (f *failure) Unwrap() error { return f.err }

// failureCoder returns the code a flow's error answers with: the failure's
// own, or the generic failure for an error that is not one, which no flow
// returns.
func failureCoder(err error) types.Coder {
	var f *failure
	if errors.As(err, &f) {
		return f.coder
	}
	return CodeFailure
}

// failWith logs err under msg, records it on the controller span ctx
// carries, and returns it as a failure answering with coder.
func failWith(ctx context.Context, log types.Logger, msg string, coder types.Coder, err error) error {
	log.Errorz(msg, zap.Error(err))
	gstotel.RecordError(trace.SpanFromContext(ctx), err)
	return &failure{coder: coder, err: err}
}

// failService reports a service hook or operation that refused or failed,
// answered with the service's own code (see serviceErrorCoder).
func failService(ctx context.Context, log types.Logger, err error) error {
	return failWith(ctx, log, "service operation failed", serviceErrorCoder(err), err)
}

// failDatabase reports a failure of the flow's own database access, answered
// through databaseErrorCoder.
func failDatabase(ctx context.Context, log types.Logger, err error) error {
	return failWith(ctx, log, "database operation failed", databaseErrorCoder(err), err)
}

// operationLog returns the operation log entry of the request ctx carries,
// with the fields every action records filled in: who acted, from where, on
// which request, in which trace. The flow adds the record and the payloads.
func operationLog(ctx context.Context, model string) *modellogmgmt.OperationLog {
	meta := requestctx.FromContext(ctx)
	return &modellogmgmt.OperationLog{
		Model:     model,
		IP:        meta.ClientIP(),
		User:      meta.Username(),
		TraceID:   execctx.FromContext(ctx).TraceID,
		URI:       meta.RequestURI(),
		Method:    meta.Method(),
		UserAgent: meta.UserAgent(),
	}
}
