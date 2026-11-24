package grpcserver

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/i18n"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// StatusError returns the status error a call answers err with: a service
// error, anywhere in the wrap chain, answers with the gRPC code its HTTP
// status maps to (see codeOf) and the client-safe message it was constructed
// with, the answer the HTTP envelope would carry, and, for the fields the
// request was refused on (see gst.FieldViolations), a google.rpc.BadRequest
// detail listing each field and its description; any other error answers
// Internal with gst.FailureMsg, the message the HTTP envelope answers the
// server's own failure with, its text kept out of the answer for the caller to
// log before mapping; nil stays nil. The message is answered in the language
// the process answers in (see i18n.Translate), the one the HTTP envelope
// carries it in. The public grpc.StatusError forwards to it.
func StatusError(err error) error {
	if err == nil {
		return nil
	}
	var serviceErr *gst.Error
	if !errors.As(err, &serviceErr) {
		return status.Error(codes.Internal, i18n.Translate(gst.FailureMsg))
	}
	st := status.New(codeOf(serviceErr.Status(), err), i18n.Translate(serviceErr.Msg()))
	if violations := gst.FieldViolations(serviceErr); len(violations) > 0 {
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
// status of a client that went away (see gst.StatusClientClosedRequest)
// as Canceled among them; and, for the rest, InvalidArgument for a client
// error and Internal for a server error.
//
// A 409 is a conflict with what is stored or under way, and its cause
// chooses the code: Aborted, a conflict to retry after, the optimistic-lock
// miss of a versioned model among them, unless err wraps
// database.ErrDuplicatedKey, the record the request creates being there
// already, AlreadyExists, the one code a caller may take for the creation
// done; or wraps database.ErrForeignKeyViolated, a record the request refers
// to missing or still referred to, FailedPrecondition, the code AIP-135
// gives a delete the records referring to the resource hold up. A service
// answering 409 for a duplicate it found by reading first carries
// database.ErrDuplicatedKey as the cause to answer AlreadyExists.
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
	case gst.StatusClientClosedRequest:
		return codes.Canceled
	case http.StatusConflict:
		switch {
		case errors.Is(err, database.ErrDuplicatedKey):
			return codes.AlreadyExists
		case errors.Is(err, database.ErrForeignKeyViolated):
			return codes.FailedPrecondition
		}
		return codes.Aborted
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
