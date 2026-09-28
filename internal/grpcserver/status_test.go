package grpcserver_test

import (
	"net/http"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/grpcserver"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestStatusErrorMapsTheStatusToTheCode pins the code a failure answers
// with over gRPC, from the HTTP status the service or the flow chose: the
// standard mapping of the HTTP statuses that have a gRPC code, a stale
// object's 409 as Aborted rather than AlreadyExists, any other client error
// as InvalidArgument and any other server error as Internal. The status
// carries the message the envelope would, and nothing else.
func TestStatusErrorMapsTheStatusToTheCode(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  *serviceregistry.Error
		want codes.Code
	}{
		{"400", serviceregistry.NewError(http.StatusBadRequest, "bad request"), codes.InvalidArgument},
		{"401", serviceregistry.NewError(http.StatusUnauthorized, "who are you"), codes.Unauthenticated},
		{"403", serviceregistry.NewError(http.StatusForbidden, "not yours"), codes.PermissionDenied},
		{"404", serviceregistry.NewError(http.StatusNotFound, "no such record"), codes.NotFound},
		{"408", serviceregistry.NewError(http.StatusRequestTimeout, "too slow"), codes.DeadlineExceeded},
		{"409", serviceregistry.NewError(http.StatusConflict, "taken"), codes.AlreadyExists},
		{"409 of a stale object", serviceregistry.NewErrorWithCause(http.StatusConflict, "reload", errors.Wrap(database.ErrStaleObject, "update sample")), codes.Aborted},
		{"412", serviceregistry.NewError(http.StatusPreconditionFailed, "not yet"), codes.FailedPrecondition},
		{"422", serviceregistry.NewError(http.StatusUnprocessableEntity, "cannot"), codes.InvalidArgument},
		{"429", serviceregistry.NewError(http.StatusTooManyRequests, "slow down"), codes.ResourceExhausted},
		{"500", serviceregistry.NewError(http.StatusInternalServerError, "broken"), codes.Internal},
		{"501", serviceregistry.NewError(http.StatusNotImplemented, "not here"), codes.Unimplemented},
		{"503", serviceregistry.NewError(http.StatusServiceUnavailable, "later"), codes.Unavailable},
		{"504", serviceregistry.NewError(http.StatusGatewayTimeout, "upstream"), codes.DeadlineExceeded},
		{"502", serviceregistry.NewError(http.StatusBadGateway, "upstream broke"), codes.Internal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := status.Convert(grpcserver.StatusError(tc.err))

			require.Equal(t, tc.want, st.Code())
			require.Equal(t, tc.err.Msg(), st.Message())
			require.Empty(t, st.Details())
		})
	}
}

// TestStatusErrorAnswersServiceErrorsAndHidesTheRest pins what an error
// reaching StatusError becomes: a service error, wherever it sits in the wrap
// chain, answers with its own status and message; any other error answers
// Internal with a fixed message, its text kept out of the answer the way the
// HTTP listener keeps internal detail out of the envelope; nil stays nil.
func TestStatusErrorAnswersServiceErrorsAndHidesTheRest(t *testing.T) {
	refused := status.Convert(grpcserver.StatusError(serviceregistry.NewErrorWithCause(http.StatusForbidden, "not yours", errors.New("row belongs to u-2"))))
	require.Equal(t, codes.PermissionDenied, refused.Code())
	require.Equal(t, "not yours", refused.Message())

	wrapped := status.Convert(grpcserver.StatusError(errors.Wrap(serviceregistry.NewError(http.StatusNotFound, "no such record"), "get sample")))
	require.Equal(t, codes.NotFound, wrapped.Code())
	require.Equal(t, "no such record", wrapped.Message())

	broken := status.Convert(grpcserver.StatusError(errors.New("dial tcp: connection refused")))
	require.Equal(t, codes.Internal, broken.Code())
	require.Equal(t, serviceregistry.FailureMsg, broken.Message())

	require.NoError(t, grpcserver.StatusError(nil))
}
