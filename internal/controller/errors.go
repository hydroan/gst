package controller

import (
	"context"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/types"
	gstotel "github.com/hydroan/gst/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// This file holds how a failure becomes an answer: the service error a
// flow's failure carries for the transport to render — its status and
// client-safe message, with the error behind as its cause — and the errors
// the database ones map to. A failure carrying no service error is the
// server's own, which each transport answers with its fixed message (see
// response.Error and grpcserver.StatusError).

// The messages of the controller's fixed refusals, each answered with the
// status named beside it.
const (
	invalidArgumentMsg = "The request contains invalid parameters."                               // 400
	notFoundMsg        = "The requested resource was not found."                                  // 404
	alreadyExistsMsg   = "The resource already exists."                                           // 409
	staleObjectMsg     = "The resource was modified by another operation. Reload and retry."      // 409
	foreignKeyMsg      = "The request refers to a record that does not exist or is still in use." // 409
)

// invalidArgument returns the refusal of a request the controller could not
// carry, err: 400 with the client-safe text of err — the message of the
// service error err wraps, when it does, err's own text otherwise — and err
// as the cause.
func invalidArgument(err error) *types.Error {
	msg := err.Error()
	var serviceErr *types.Error
	if errors.As(err, &serviceErr) {
		msg = serviceErr.Msg()
	}
	return types.NewErrorWithCause(http.StatusBadRequest, msg, err)
}

// badRequest returns the 400 refusal carrying msg.
func badRequest(msg string) *types.Error {
	return types.NewError(http.StatusBadRequest, msg)
}

// notFound returns the 404 refusal of a record the flow could not find, with
// cause, when there is one, behind it.
func notFound(cause error) *types.Error {
	return types.NewErrorWithCause(http.StatusNotFound, notFoundMsg, cause)
}

// failWith logs err under msg, records it on the controller span ctx
// carries, and returns answer, the error the flow's failure answers with.
func failWith(ctx context.Context, log types.Logger, msg string, err, answer error) error {
	log.Errorz(msg, zap.Error(err))
	gstotel.RecordError(trace.SpanFromContext(ctx), err)
	return answer
}

// failService reports a service hook or operation that refused or failed,
// answered with err itself: the service error it carries, when it does,
// keeps the status and message it was constructed with, and anything else
// is the server's own failure. Internal error text — database drivers
// naming tables and columns, third-party client output — never reaches the
// envelope; the transports render only a service error's message.
func failService(ctx context.Context, log types.Logger, err error) error {
	return failWith(ctx, log, "service operation failed", err, err)
}

// failDatabase reports a failure of the flow's own database access, answered
// through databaseError.
func failDatabase(ctx context.Context, log types.Logger, err error) error {
	return failWith(ctx, log, "database operation failed", err, databaseError(err))
}

// databaseError maps a database error to the error the flow answers with: a
// service-layer error keeps the status and message it was constructed with;
// database.ErrRecordNotFound answers 404; database.ErrDuplicatedKey,
// database.ErrStaleObject and database.ErrForeignKeyViolated 409;
// database.ErrVersionRequired, database.ErrIDRequired,
// database.ErrCheckConstraintViolated, database.ErrValueTooLong and
// database.ErrNotNullViolated, request defects all, 400, each with its fixed
// client-safe message and err behind it as the cause; anything else is
// answered as it is, the server's own failure. Handlers log the full error
// themselves, so every branch deliberately keeps internal detail out of the
// response.
//
// The service error is honored first, and here as well as in the action path:
// a model hook refusing an operation states its status deliberately — a guard
// answering 403 — and flattening that to the server's own failure would
// misreport a permission boundary as a server fault.
func databaseError(err error) error {
	var serviceErr *types.Error
	switch {
	case errors.As(err, &serviceErr):
		return err
	case errors.Is(err, database.ErrRecordNotFound):
		return notFound(err)
	case errors.Is(err, database.ErrDuplicatedKey):
		return types.NewErrorWithCause(http.StatusConflict, alreadyExistsMsg, err)
	case errors.Is(err, database.ErrStaleObject):
		// The optimistic-lock miss of a versioned model: modified or deleted
		// by someone else after this caller read it. 409, reload and retry.
		return types.NewErrorWithCause(http.StatusConflict, staleObjectMsg, err)
	case errors.Is(err, database.ErrVersionRequired):
		// A versioned record arrived without the version it was read with —
		// a request defect, not a conflict.
		return types.NewErrorWithCause(http.StatusBadRequest, invalidArgumentMsg, err)
	case errors.Is(err, database.ErrIDRequired):
		// A batch item arrived without the id naming its record — a request
		// defect as well.
		return types.NewErrorWithCause(http.StatusBadRequest, invalidArgumentMsg, err)
	case errors.Is(err, database.ErrForeignKeyViolated):
		// The request names a record that is not there, or would leave one
		// other records still refer to: a conflict with the records as they
		// are, to retry once they are in place, FailedPrecondition over gRPC
		// (see grpcserver.StatusError).
		return types.NewErrorWithCause(http.StatusConflict, foreignKeyMsg, err)
	case errors.Is(err, database.ErrCheckConstraintViolated), errors.Is(err, database.ErrValueTooLong), errors.Is(err, database.ErrNotNullViolated):
		// A value the table refuses, by a check, by the length of the
		// column or by a column that requires one: the request's own defect.
		return types.NewErrorWithCause(http.StatusBadRequest, invalidArgumentMsg, err)
	default:
		// Any other database error is the server's own failure.
		return err
	}
}
