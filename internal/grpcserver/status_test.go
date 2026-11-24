package grpcserver_test

import (
	"net/http"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/grpcserver"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/i18n"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestStatusErrorMapsTheStatusToTheCode pins the code a failure answers
// with over gRPC, from the HTTP status the service or the flow chose: the
// standard mapping of the HTTP statuses that have a gRPC code, the 499 of a
// client that went away as Canceled, a 409 as Aborted, a stale object's
// among them, save a duplicated key's, AlreadyExists, and a foreign key's,
// FailedPrecondition, any other client error as InvalidArgument and any
// other server error as Internal. The status carries the message the
// envelope would, and nothing else.
func TestStatusErrorMapsTheStatusToTheCode(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  *gst.Error
		want codes.Code
	}{
		{"400", gst.NewError(http.StatusBadRequest, "bad request"), codes.InvalidArgument},
		{"401", gst.NewError(http.StatusUnauthorized, "who are you"), codes.Unauthenticated},
		{"403", gst.NewError(http.StatusForbidden, "not yours"), codes.PermissionDenied},
		{"404", gst.NewError(http.StatusNotFound, "no such record"), codes.NotFound},
		{"408", gst.NewError(http.StatusRequestTimeout, "too slow"), codes.DeadlineExceeded},
		{"409", gst.NewError(http.StatusConflict, "busy"), codes.Aborted},
		{"409 of a duplicated key", gst.NewErrorWithCause(http.StatusConflict, "taken", errors.Wrap(database.ErrDuplicatedKey, "create sample")), codes.AlreadyExists},
		{"409 of a stale object", gst.NewErrorWithCause(http.StatusConflict, "reload", errors.Wrap(database.ErrStaleObject, "update sample")), codes.Aborted},
		{"409 of a foreign key", gst.NewErrorWithCause(http.StatusConflict, "refers", errors.Wrap(database.ErrForeignKeyViolated, "create sample")), codes.FailedPrecondition},
		{"412", gst.NewError(http.StatusPreconditionFailed, "not yet"), codes.FailedPrecondition},
		{"422", gst.NewError(http.StatusUnprocessableEntity, "cannot"), codes.InvalidArgument},
		{"429", gst.NewError(http.StatusTooManyRequests, "slow down"), codes.ResourceExhausted},
		{"499", gst.NewError(gst.StatusClientClosedRequest, "gone"), codes.Canceled},
		{"500", gst.NewError(http.StatusInternalServerError, "broken"), codes.Internal},
		{"501", gst.NewError(http.StatusNotImplemented, "not here"), codes.Unimplemented},
		{"502", gst.NewError(http.StatusBadGateway, "upstream broke"), codes.Internal},
		{"503", gst.NewError(http.StatusServiceUnavailable, "later"), codes.Unavailable},
		{"504", gst.NewError(http.StatusGatewayTimeout, "upstream"), codes.DeadlineExceeded},
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
// typed nil *gst.Error, the error a `var e *gst.Error; return e` hands its
// caller, answers Internal with gst.FailureMsg, as every failure of the
// server's own does.
func TestStatusErrorAnswersATypedNilServiceErrorAsTheServerFailure(t *testing.T) {
	st := status.Convert(grpcserver.StatusError((*gst.Error)(nil)))

	require.Equal(t, codes.Internal, st.Code())
	require.Equal(t, gst.FailureMsg, st.Message())
}

// TestStatusErrorAttachesTheFieldViolations pins that a service error
// carrying the fields the validator refused answers InvalidArgument with
// the message naming them, and a google.rpc.BadRequest detail listing each
// field and its description, the shape AIP-193 gives a validation failure.
func TestStatusErrorAttachesTheFieldViolations(t *testing.T) {
	violations := []gst.FieldViolation{
		{Field: "name", Description: "name is a required field"},
		{Field: "address.city", Description: "address.city is a required field"},
	}

	st := status.Convert(grpcserver.StatusError(gst.NewInvalidFields(violations, errors.New("validation failed"))))

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
	refused := status.Convert(grpcserver.StatusError(gst.NewErrorWithCause(http.StatusForbidden, "not yours", errors.New("row belongs to u-2"))))
	require.Equal(t, codes.PermissionDenied, refused.Code())
	require.Equal(t, "not yours", refused.Message())

	wrapped := status.Convert(grpcserver.StatusError(errors.Wrap(gst.NewError(http.StatusNotFound, "no such record"), "get sample")))
	require.Equal(t, codes.NotFound, wrapped.Code())
	require.Equal(t, "no such record", wrapped.Message())

	broken := status.Convert(grpcserver.StatusError(errors.New("dial tcp: connection refused")))
	require.Equal(t, codes.Internal, broken.Code())
	require.Equal(t, gst.FailureMsg, broken.Message())

	require.NoError(t, grpcserver.StatusError(nil))
}

// TestStatusErrorAnswersInTheLanguageOfTheProcess pins the language of a
// status: its message is the sentence in the language the process answers
// in, the one the HTTP envelope carries it in, the translation registered for
// it, the server's own failure among them, and the sentence as it is written
// where no translation of it is registered. The fields a request was refused
// on are described as the error carries them.
func TestStatusErrorAnswersInTheLanguageOfTheProcess(t *testing.T) {
	i18n.Register(i18n.Chinese, map[string]string{"the sample is sealed": "样本已封存"})
	require.NoError(t, i18n.SetLanguage(i18n.Chinese))
	t.Cleanup(func() { require.NoError(t, i18n.SetLanguage(i18n.English)) })

	sealed := status.Convert(grpcserver.StatusError(gst.NewError(http.StatusConflict, "the sample is sealed")))
	require.Equal(t, codes.Aborted, sealed.Code())
	require.Equal(t, "样本已封存", sealed.Message())

	open := status.Convert(grpcserver.StatusError(gst.NewError(http.StatusConflict, "the sample is open")))
	require.Equal(t, "the sample is open", open.Message())

	broken := status.Convert(grpcserver.StatusError(errors.New("dial tcp: connection refused")))
	require.Equal(t, codes.Internal, broken.Code())
	require.Equal(t, "服务器无法处理该请求。", broken.Message())

	refused := status.Convert(grpcserver.StatusError(gst.NewInvalidFields([]gst.FieldViolation{{Field: "name", Description: "name为必填字段"}}, nil)))
	require.Equal(t, "name为必填字段", refused.Message())
	require.Len(t, refused.Details(), 1)
	bad, ok := refused.Details()[0].(*errdetails.BadRequest)
	require.True(t, ok, "%T", refused.Details()[0])
	require.Equal(t, "name为必填字段", bad.GetFieldViolations()[0].GetDescription())
}
