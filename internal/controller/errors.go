package controller

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/grpcserver"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
	gstotel "github.com/hydroan/gst/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// This file holds how a failure becomes an answer: the code a flow's
// failure carries for the transport to render, the codes the service and
// database errors map to, and the envelope or status each transport answers
// with.

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

// serviceErrorCoder maps a service-layer failure to its code: a service error
// keeps the status and message it was constructed with, and anything else
// renders the generic failure message. Internal error text — database drivers
// naming tables and columns, third-party client output — never reaches the
// envelope; callers log the full error themselves before mapping it here.
func serviceErrorCoder(err error) types.Coder {
	var serviceErr *serviceregistry.Error
	if errors.As(err, &serviceErr) {
		return serviceErr
	}
	return CodeFailure
}

// handleServiceError renders a service-layer failure through
// serviceErrorCoder.
func handleServiceError(c *gin.Context, err error) {
	JSON(c, serviceErrorCoder(err))
}

// databaseErrorCoder maps database errors to their canonical API codes: a
// service-layer error keeps the status and message it was constructed with;
// database.ErrRecordNotFound renders 404, database.ErrDuplicatedKey and
// database.ErrStaleObject render 409, and database.ErrVersionRequired and
// database.ErrIDRequired, request defects both, render 400, each with its
// fixed client-safe message; anything else falls back to the generic failure
// message. Handlers log the full error themselves, so every branch
// deliberately keeps internal detail out of the response.
//
// The service error is honored first, and here as well as in the action path:
// a model hook refusing an operation states its status deliberately — a guard
// answering 403 — and flattening that to a generic failure would misreport a
// permission boundary as a malformed request.
func databaseErrorCoder(err error) types.Coder {
	var serviceErr *serviceregistry.Error
	switch {
	case errors.As(err, &serviceErr):
		return serviceErr
	case errors.Is(err, database.ErrRecordNotFound):
		return CodeNotFound
	case errors.Is(err, database.ErrDuplicatedKey):
		return CodeAlreadyExist
	case errors.Is(err, database.ErrStaleObject):
		// The optimistic-lock miss of a versioned model: modified or deleted
		// by someone else after this caller read it. 409, reload and retry.
		return CodeStaleObject
	case errors.Is(err, database.ErrVersionRequired):
		// A versioned record arrived without the version it was read with —
		// a request defect, not a conflict.
		return CodeInvalidParam
	case errors.Is(err, database.ErrIDRequired):
		// A batch item arrived without the id naming its record — a request
		// defect as well.
		return CodeInvalidParam
	default:
		// TODO: this fallback, like handleServiceError's, answers an
		// unexpected failure with 400, reporting a server-side problem as the
		// client's. Server-side failures should answer 5xx and client-side
		// ones 4xx; doing it right also takes mapping the database errors
		// client data causes (a value too long, a missing foreign key, a
		// failed check) to 4xx, and giving the validation errors of the authz
		// model hooks a 4xx status.
		return CodeFailure
	}
}

// statusOf returns the status a call answers err with, the failure of a flow
// or a service, from the code the HTTP listener would answer it with: the
// code's mapping (see grpcserver.StatusOfCoder) for every failure the
// listener recognizes — a refusal, a missing record, a conflict, a service
// error with a status of its own — and Internal with a fixed message for the
// generic failure, the one the listener answers a failure it did not
// recognize with, an unknown database error or a plain error of a hook or
// service: a failure of the server's own is the server's, not, as the HTTP
// envelope has it, the client's, and its text stays out of the answer the
// way grpcserver.StatusError keeps it out.
func statusOf(coder types.Coder, err error) error {
	if coder == CodeFailure {
		return grpcserver.StatusError(err)
	}
	return grpcserver.StatusOfCoder(coder)
}
