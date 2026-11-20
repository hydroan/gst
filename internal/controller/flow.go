package controller

import (
	"context"

	"github.com/hydroan/gst/internal/apiaction"
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
// with the request message (see call_grpc.go). A flow's failure is the
// error of errors.go, which the transport renders.

// serviceContextFunc builds the service context of one hook or service call
// from the context of that call and its phase. Only the transport knows what
// else the service context should carry — the HTTP handler's gin context,
// nothing at all for a transport without one — so the transport supplies it.
type serviceContextFunc func(ctx context.Context, phase apiaction.Phase) *types.ServiceContext

// clearAudit clears the creator and updater of records, what a client sent
// in them: who created or updated a record is the server's to write. The
// write names the caller the context carries as both (see database.Create),
// and with no caller, an anonymous request's, the records keep what a hook
// set, never what the client sent. The write flows call it first, before
// any hook runs, so a hook may still set them on purpose.
func clearAudit[M types.Model](records ...M) {
	for _, record := range records {
		record.SetCreatedBy("")
		record.SetUpdatedBy("")
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

// setID copies the id the request names a record by — the route parameter
// over HTTP, the id field of the message over gRPC — into the model and
// reports whether the model accepted it. UUID-keyed models (Base) accept any
// non-empty string, so the check never changes their behavior. Integer-keyed
// models (AutoBase) leave the id unset when the raw value does not parse
// into their key type; handlers must answer such requests with "not found"
// before any database access, because an unset id would silently drop the
// intended row filter and passing the raw value to SQL would rely on the
// database's implicit string-to-integer coercion (MySQL matches id=7 for
// '7abc').
//
// The caller must pass a non-empty id: UUID-keyed models generate a fresh id
// when given an empty value.
func setID(m types.Model, id string) bool {
	m.SetID(id)
	return len(m.GetID()) > 0
}
