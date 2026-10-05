package controller

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/execctx"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/types"
)

// This file holds what the CRUD flows share. A flow is the half of a handler
// that does not depend on the transport the request arrived on: the service
// hooks, the database access and the operation log, in the order the action
// defines. It runs on a context carrying the request metadata (see
// requestctx) and the controller span, and builds the service contexts of
// its hooks through a function the transport supplies, which is all a
// transport contributes to it. The HTTP handlers of this package bind the
// request, run the flow and write its answer; the gRPC calls do the same
// with the request message (see call.go). A flow's failure is the error of
// errors.go, which the transport renders.

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
