package grpcserver_test

import (
	"net/http"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/grpcserver"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestStatusErrorMapsTheStatusToTheCode pins the code a failure answers
// with over gRPC, from the HTTP status the service or the flow chose: the
// standard mapping of the HTTP statuses that have a gRPC code, a stale
// object's 409 as Aborted and a foreign key's as FailedPrecondition rather
// than AlreadyExists, any other client error
// as InvalidArgument and any other server error as Internal. The status
// carries the message the envelope would, and nothing else.
func TestStatusErrorMapsTheStatusToTheCode(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  *types.Error
		want codes.Code
	}{
		{"400", types.NewError(http.StatusBadRequest, "bad request"), codes.InvalidArgument},
		{"401", types.NewError(http.StatusUnauthorized, "who are you"), codes.Unauthenticated},
		{"403", types.NewError(http.StatusForbidden, "not yours"), codes.PermissionDenied},
		{"404", types.NewError(http.StatusNotFound, "no such record"), codes.NotFound},
		{"408", types.NewError(http.StatusRequestTimeout, "too slow"), codes.DeadlineExceeded},
		{"409", types.NewError(http.StatusConflict, "taken"), codes.AlreadyExists},
		{"409 of a stale object", types.NewErrorWithCause(http.StatusConflict, "reload", errors.Wrap(database.ErrStaleObject, "update sample")), codes.Aborted},
		{"409 of a foreign key", types.NewErrorWithCause(http.StatusConflict, "refers", errors.Wrap(database.ErrForeignKeyViolated, "create sample")), codes.FailedPrecondition},
		{"412", types.NewError(http.StatusPreconditionFailed, "not yet"), codes.FailedPrecondition},
		{"422", types.NewError(http.StatusUnprocessableEntity, "cannot"), codes.InvalidArgument},
		{"429", types.NewError(http.StatusTooManyRequests, "slow down"), codes.ResourceExhausted},
		{"500", types.NewError(http.StatusInternalServerError, "broken"), codes.Internal},
		{"501", types.NewError(http.StatusNotImplemented, "not here"), codes.Unimplemented},
		{"503", types.NewError(http.StatusServiceUnavailable, "later"), codes.Unavailable},
		{"504", types.NewError(http.StatusGatewayTimeout, "upstream"), codes.DeadlineExceeded},
		{"502", types.NewError(http.StatusBadGateway, "upstream broke"), codes.Internal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := status.Convert(grpcserver.StatusError(tc.err))

			require.Equal(t, tc.want, st.Code())
			require.Equal(t, tc.err.Msg(), st.Message())
			require.Empty(t, st.Details())
		})
	}
}

// TestStatusErrorAnswersATypedNilServiceErrorAsTheServerFailure pins that a
// typed nil *types.Error, the error a `var e *gst.Error; return e` hands its
// caller, answers Internal with types.FailureMsg, as every failure of the
// server's own does.
func TestStatusErrorAnswersATypedNilServiceErrorAsTheServerFailure(t *testing.T) {
	st := status.Convert(grpcserver.StatusError((*types.Error)(nil)))

	require.Equal(t, codes.Internal, st.Code())
	require.Equal(t, types.FailureMsg, st.Message())
}

// TestStatusErrorAttachesTheFieldViolations pins that a service error
// carrying the fields the validator refused answers InvalidArgument with
// the message naming them, and a google.rpc.BadRequest detail listing each
// field and its description, the shape AIP-193 gives a validation failure.
func TestStatusErrorAttachesTheFieldViolations(t *testing.T) {
	violations := []types.FieldViolation{
		{Field: "name", Description: "name is a required field"},
		{Field: "address.city", Description: "address.city is a required field"},
	}

	st := status.Convert(grpcserver.StatusError(types.NewInvalidFields(violations, errors.New("validation failed"))))

	require.Equal(t, codes.InvalidArgument, st.Code())
	require.Equal(t, "name is a required field; address.city is a required field", st.Message())
	require.Len(t, st.Details(), 1)
	bad, ok := st.Details()[0].(*errdetails.BadRequest)
	require.True(t, ok, "%T", st.Details()[0])
	require.Len(t, bad.GetFieldViolations(), 2)
	for i, v := range violations {
		require.Equal(t, v.Field, bad.GetFieldViolations()[i].GetField())
		require.Equal(t, v.Description, bad.GetFieldViolations()[i].GetDescription())
	}
}

// TestStatusErrorAnswersServiceErrorsAndHidesTheRest pins what an error
// reaching StatusError becomes: a service error, wherever it sits in the wrap
// chain, answers with its own status and message; any other error answers
// Internal with a fixed message, its text kept out of the answer the way the
// HTTP listener keeps internal detail out of the envelope; nil stays nil.
func TestStatusErrorAnswersServiceErrorsAndHidesTheRest(t *testing.T) {
	refused := status.Convert(grpcserver.StatusError(types.NewErrorWithCause(http.StatusForbidden, "not yours", errors.New("row belongs to u-2"))))
	require.Equal(t, codes.PermissionDenied, refused.Code())
	require.Equal(t, "not yours", refused.Message())

	wrapped := status.Convert(grpcserver.StatusError(errors.Wrap(types.NewError(http.StatusNotFound, "no such record"), "get sample")))
	require.Equal(t, codes.NotFound, wrapped.Code())
	require.Equal(t, "no such record", wrapped.Message())

	broken := status.Convert(grpcserver.StatusError(errors.New("dial tcp: connection refused")))
	require.Equal(t, codes.Internal, broken.Code())
	require.Equal(t, types.FailureMsg, broken.Message())

	require.NoError(t, grpcserver.StatusError(nil))
}
