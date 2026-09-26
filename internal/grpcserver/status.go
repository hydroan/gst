package grpcserver

import (
	"net/http"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The ErrorInfo detail every failure status carries: its reason, one for
// every failure the framework answers, and the keys of its metadata, the
// business code and HTTP status the HTTP envelope would carry; its domain is
// the framework's name.
const (
	statusReason       = "SERVICE_ERROR"
	statusDetailCode   = "code"
	statusDetailStatus = "status"
)

// StatusError returns the status error a call answers err with: a service
// error answers with the status and message it was constructed with, mapped
// like any coder (see StatusOfCoder); any other error answers Internal with
// a fixed message, its text kept out of the answer the way the HTTP
// listener keeps internal detail out of the envelope, for the caller to log
// before mapping; nil stays nil. The public grpc.StatusError forwards
// to it.
func StatusError(err error) error {
	if err == nil {
		return nil
	}
	var serviceErr *serviceregistry.Error
	if errors.As(err, &serviceErr) {
		return StatusOfCoder(serviceErr)
	}
	return status.Error(codes.Internal, "internal server error")
}

// StatusOfCoder returns the status a call answers a failure with, from the
// code the HTTP listener would answer it with: the gRPC code codeOf maps
// the HTTP status to, the message the envelope would carry, and, in an
// ErrorInfo detail of domain "gst", the business code and HTTP status the
// envelope would carry as well, so a client can act on any of the three.
// The call functions of the controller answer a flow's failure through it.
func StatusOfCoder(coder types.Coder) error {
	st := status.New(codeOf(coder), coder.Msg())
	detailed, err := st.WithDetails(&errdetails.ErrorInfo{
		Reason: statusReason,
		Domain: consts.FrameworkName,
		Metadata: map[string]string{
			statusDetailCode:   strconv.Itoa(coder.Code()),
			statusDetailStatus: strconv.Itoa(coder.Status()),
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
