package controller

import (
	"context"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
	gstotel "github.com/hydroan/gst/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// This file holds how a failure becomes an answer: the service error a
// flow's failure carries for the transport to render — its status and
// client-safe message, with the error behind as its cause — and the errors
// the database ones map to. A failure carrying no service error is the
// generic one, which each transport answers in its own way (see
// response.Error and grpcserver.StatusError).

// The messages of the controller's fixed refusals, each answered with the
// status named beside it.
const (
	invalidArgumentMsg = "The request contains invalid parameters."                          // 400
	notFoundMsg        = "The requested resource was not found."                             // 404
	alreadyExistsMsg   = "The resource already exists."                                      // 409
	staleObjectMsg     = "The resource was modified by another operation. Reload and retry." // 409
)

// invalidArgument returns the refusal of a request the controller could not
// carry, err: 400 with the client-safe text of err — the message of the
// service error err wraps, when it does, err's own text otherwise — and err
// as the cause.
func invalidArgument(err error) *serviceregistry.Error {
	msg := err.Error()
	var serviceErr *serviceregistry.Error
	if errors.As(err, &serviceErr) {
		msg = serviceErr.Msg()
	}
	return serviceregistry.NewErrorWithCause(http.StatusBadRequest, msg, err)
}

// badRequest returns the 400 refusal carrying msg.
func badRequest(msg string) *serviceregistry.Error {
	return serviceregistry.NewError(http.StatusBadRequest, msg)
}

// notFound returns the 404 refusal of a record the flow could not find, with
// cause, when there is one, behind it.
func notFound(cause error) *serviceregistry.Error {
	return serviceregistry.NewErrorWithCause(http.StatusNotFound, notFoundMsg, cause)
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
// is the generic failure. Internal error text — database drivers naming
// tables and columns, third-party client output — never reaches the
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
// database.ErrRecordNotFound answers 404, database.ErrDuplicatedKey and
// database.ErrStaleObject 409, and database.ErrVersionRequired and
// database.ErrIDRequired, request defects both, 400, each with its fixed
// client-safe message and err behind it as the cause; anything else is
// answered as it is, the generic failure. Handlers log the full error
// themselves, so every branch deliberately keeps internal detail out of the
// response.
//
// The service error is honored first, and here as well as in the action path:
// a model hook refusing an operation states its status deliberately — a guard
// answering 403 — and flattening that to a generic failure would misreport a
// permission boundary as a malformed request.
func databaseError(err error) error {
	var serviceErr *serviceregistry.Error
	switch {
	case errors.As(err, &serviceErr):
		return err
	case errors.Is(err, database.ErrRecordNotFound):
		return notFound(err)
	case errors.Is(err, database.ErrDuplicatedKey):
		return serviceregistry.NewErrorWithCause(http.StatusConflict, alreadyExistsMsg, err)
	case errors.Is(err, database.ErrStaleObject):
		// The optimistic-lock miss of a versioned model: modified or deleted
		// by someone else after this caller read it. 409, reload and retry.
		return serviceregistry.NewErrorWithCause(http.StatusConflict, staleObjectMsg, err)
	case errors.Is(err, database.ErrVersionRequired):
		// A versioned record arrived without the version it was read with —
		// a request defect, not a conflict.
		return serviceregistry.NewErrorWithCause(http.StatusBadRequest, invalidArgumentMsg, err)
	case errors.Is(err, database.ErrIDRequired):
		// A batch item arrived without the id naming its record — a request
		// defect as well.
		return serviceregistry.NewErrorWithCause(http.StatusBadRequest, invalidArgumentMsg, err)
	default:
		// TODO: the generic failure this falls back to, like the one a plain
		// error of a hook or service is, answers 400 on HTTP, reporting a
		// server-side problem as the client's. Server-side failures should
		// answer 5xx and client-side ones 4xx; doing it right also takes
		// mapping the database errors client data causes (a value too long,
		// a missing foreign key, a failed check) to 4xx, and giving the
		// validation errors of the authz model hooks a 4xx status.
		return err
	}
}
