package grpcserver

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/serviceregistry"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// StatusError returns the status error a call answers err with: a service
// error, anywhere in the wrap chain, answers with the gRPC code its HTTP
// status maps to (see codeOf) and the client-safe message it was constructed
// with, the answer the HTTP envelope would carry, and, for the fields a
// validator refused (see serviceregistry.FieldViolations), a
// google.rpc.BadRequest detail listing each field and its description; any
// other error answers Internal with serviceregistry.FailureMsg, the message
// the HTTP envelope answers the server's own failure with, its text kept
// out of the answer for the caller to log before mapping; nil stays nil.
// The public grpc.StatusError forwards to it.
func StatusError(err error) error {
	if err == nil {
		return nil
	}
	var serviceErr *serviceregistry.Error
	if !errors.As(err, &serviceErr) {
		return status.Error(codes.Internal, serviceregistry.FailureMsg)
	}
	st := status.New(codeOf(serviceErr.Status(), err), serviceErr.Msg())
	if violations := serviceErr.FieldViolations(); len(violations) > 0 {
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
// standard correspondence for the statuses that have one; the 409 of a stale
// object — err wraps database.ErrStaleObject, the optimistic-lock miss of a
// versioned model — as Aborted rather than AlreadyExists, since it is a
// concurrent change to retry after and not a duplicate; the 409 of a
// foreign key — err wraps database.ErrForeignKeyViolated, a record the
// request refers to missing or still referred to — as FailedPrecondition,
// the code AIP-135 gives a delete the records referring to the resource
// hold up; and, for the rest, InvalidArgument for a client error and
// Internal for a server error.
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
