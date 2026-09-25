package grpcserver

import (
	"net/http"
	"strconv"

	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// statusError returns the status a call answers a failure with, from the
// code the HTTP listener would answer it with: the gRPC code codeOf maps
// the HTTP status to, the message the envelope would carry, and, in an
// ErrorInfo detail of domain "gst", the business code and HTTP status the
// envelope would carry as well, so a client can act on any of the three.
func statusError(coder types.Coder) error {
	st := status.New(codeOf(coder), coder.Msg())
	detailed, err := st.WithDetails(&errdetails.ErrorInfo{
		Reason: "SERVICE_ERROR",
		Domain: "gst",
		Metadata: map[string]string{
			"code":   strconv.Itoa(coder.Code()),
			"status": strconv.Itoa(coder.Status()),
		},
	})
	if err != nil {
		// A detail fails to attach only when it cannot be marshaled, which a
		// message of strings never does; the status still says what failed.
		return st.Err()
	}
	return detailed.Err()
}

// codeOf maps the HTTP status a failure answers with to its gRPC code: the
// standard correspondence for the statuses that have one, a stale object's
// 409 as Aborted rather than AlreadyExists, since it is a concurrent change
// to retry after and not a duplicate, and, for the rest, InvalidArgument
// for a client error and Internal for a server error.
func codeOf(coder types.Coder) codes.Code {
	if coder.Code() == response.CodeStaleObject.Code() {
		return codes.Aborted
	}
	switch coder.Status() {
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
	if coder.Status() >= http.StatusInternalServerError {
		return codes.Internal
	}
	return codes.InvalidArgument
}
