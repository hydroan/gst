package grpcserver

import (
	"log/slog"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/types"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// StatusError returns the status error a call answers err with: a service
// error, anywhere in the wrap chain, answers with the gRPC code its HTTP
// status maps to (see codeOf) and the client-safe message it was constructed
// with, the answer the HTTP envelope would carry, and, for the fields the
// request was refused on (see types.FieldViolations), a google.rpc.BadRequest
// detail listing each field and its description; any other error answers
// Internal with types.FailureMsg, the message the HTTP envelope answers the
// server's own failure with, its text kept out of the answer for the caller to
// log before mapping; nil stays nil. The public grpc.StatusError forwards to
// it.
func StatusError(err error) error {
	if err == nil {
		return nil
	}
	var serviceErr *types.Error
	if !errors.As(err, &serviceErr) {
		return status.Error(codes.Internal, types.FailureMsg)
	}
	st := status.New(codeOf(serviceErr.Status(), err), serviceErr.Msg())
	if violations := types.FieldViolations(serviceErr); len(violations) > 0 {
		bad := &errdetails.BadRequest{FieldViolations: make([]*errdetails.BadRequest_FieldViolation, 0, len(violations))}
		for _, v := range violations {
			bad.FieldViolations = append(bad.FieldViolations, &errdetails.BadRequest_FieldViolation{Field: v.Field, Description: v.Description})
		}
		if detailed, detailErr := st.WithDetails(bad); detailErr == nil {
			st = detailed
		}
	}
	return st.Err()
}

// codeOf maps the HTTP status a failure answers with to its gRPC code: the
// standard correspondence for the statuses that have one, the client-closed
// status of a client that went away (see types.StatusClientClosedRequest)
// as Canceled among them; the 409 of a stale object — err wraps
// database.ErrStaleObject, the optimistic-lock miss of a versioned model —
// as Aborted rather than AlreadyExists, since it is a concurrent change to
// retry after and not a duplicate; the 409 of a foreign key — err wraps
// database.ErrForeignKeyViolated, a record the request refers to missing or
// still referred to — as FailedPrecondition, the code AIP-135 gives a delete
// the records referring to the resource hold up; and, for the rest,
// InvalidArgument for a client error and Internal for a server error.
func codeOf(httpStatus int, err error) codes.Code {
	switch httpStatus {
	case http.StatusBadRequest:
		return codes.InvalidArgument
	case http.StatusUnauthorized:
		return codes.Unauthenticated
	case http.StatusForbidden:
		return codes.PermissionDenied
	case http.StatusNotFound:
		return codes.NotFound
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return codes.DeadlineExceeded
	case types.StatusClientClosedRequest:
		return codes.Canceled
	case http.StatusConflict:
		if errors.Is(err, database.ErrStaleObject) {
			return codes.Aborted
		}
		if errors.Is(err, database.ErrForeignKeyViolated) {
			return codes.FailedPrecondition
		}
		return codes.AlreadyExists
	case http.StatusPreconditionFailed:
		return codes.FailedPrecondition
	case http.StatusTooManyRequests:
		return codes.ResourceExhausted
	case http.StatusNotImplemented:
		return codes.Unimplemented
	case http.StatusServiceUnavailable:
		return codes.Unavailable
	}
	if httpStatus >= http.StatusInternalServerError {
		return codes.Internal
	}
	return codes.InvalidArgument
}

// StatusLevel returns the level the failure StatusError answers err for is
// logged at, the same on either transport. The server's own failure (see
// types.ServerFailure) — err carrying no service error, which StatusError
// answers Internal, or one answering 500 or above — is logged at error. A
// failure the client brought on itself is logged at the level the logging
// interceptors of go-grpc-middleware log a call ending with its code at (see
// logging.DefaultServerCodeToLevel and codeOf): info for a request that is
// malformed, unauthenticated or canceled, or that names a record that is
// missing or exists already; warn for one refused permission, a
// precondition, a concurrent change or its rate, or that took too long to
// arrive.
//
// The table is read for the client's failures alone. It grades two of the
// server's warn, DeadlineExceeded and Unavailable, which the tracing
// conventions count as the server's error, and DeadlineExceeded is the code
// of a 408 as well, the client not sending in time, which is the client's:
// the status tells a 408 from a 504, so it is read before the code. A
// failure is thus logged at error exactly when its answer is the server's,
// which is when a server span is marked failed for it.
func StatusLevel(err error) slog.Level {
	var serviceErr *types.Error
	if types.ServerFailure(err) || !errors.As(err, &serviceErr) {
		return slog.LevelError
	}
	// The library's levels are slog's own, value for value.
	return slog.Level(logging.DefaultServerCodeToLevel(codeOf(serviceErr.Status(), err)))
}
