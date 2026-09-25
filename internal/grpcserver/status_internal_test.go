package grpcserver

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestStatusErrorMapsTheAnswerToTheCode pins the code a failure answers with
// over gRPC, from the HTTP status the service or the flow chose: the
// standard mapping of the HTTP statuses that have a gRPC code, a stale
// object's 409 as Aborted rather than AlreadyExists, any other client error
// as InvalidArgument and any other server error as Internal. The status
// carries the message the envelope would, and, in its details, the business
// code and HTTP status the envelope would carry.
func TestStatusErrorMapsTheAnswerToTheCode(t *testing.T) {
	for _, tc := range []struct {
		name  string
		coder types.Coder
		want  codes.Code
	}{
		{"400", serviceregistry.NewError(http.StatusBadRequest, "bad request"), codes.InvalidArgument},
		{"401", serviceregistry.NewError(http.StatusUnauthorized, "who are you"), codes.Unauthenticated},
		{"403", serviceregistry.NewError(http.StatusForbidden, "not yours"), codes.PermissionDenied},
		{"404", serviceregistry.NewError(http.StatusNotFound, "no such record"), codes.NotFound},
		{"408", serviceregistry.NewError(http.StatusRequestTimeout, "too slow"), codes.DeadlineExceeded},
		{"409", serviceregistry.NewError(http.StatusConflict, "taken"), codes.AlreadyExists},
		{"412", serviceregistry.NewError(http.StatusPreconditionFailed, "not yet"), codes.FailedPrecondition},
		{"422", serviceregistry.NewError(http.StatusUnprocessableEntity, "cannot"), codes.InvalidArgument},
		{"429", serviceregistry.NewError(http.StatusTooManyRequests, "slow down"), codes.ResourceExhausted},
		{"500", serviceregistry.NewError(http.StatusInternalServerError, "broken"), codes.Internal},
		{"501", serviceregistry.NewError(http.StatusNotImplemented, "not here"), codes.Unimplemented},
		{"503", serviceregistry.NewError(http.StatusServiceUnavailable, "later"), codes.Unavailable},
		{"504", serviceregistry.NewError(http.StatusGatewayTimeout, "upstream"), codes.DeadlineExceeded},
		{"502", serviceregistry.NewError(http.StatusBadGateway, "upstream broke"), codes.Internal},
		{"generic failure", response.CodeFailure, codes.InvalidArgument},
		{"not found", response.CodeNotFound, codes.NotFound},
		{"already exists", response.CodeAlreadyExist, codes.AlreadyExists},
		{"stale object", response.CodeStaleObject, codes.Aborted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := status.Convert(statusError(tc.coder))

			require.Equal(t, tc.want, st.Code())
			require.Equal(t, tc.coder.Msg(), st.Message())
			var info *errdetails.ErrorInfo
			for _, detail := range st.Details() {
				if d, ok := detail.(*errdetails.ErrorInfo); ok {
					info = d
				}
			}
			require.NotNil(t, info, "the details carry what the envelope would")
			require.Equal(t, "gst", info.GetDomain())
			require.Equal(t, strconv.Itoa(tc.coder.Code()), info.GetMetadata()["code"])
			require.Equal(t, strconv.Itoa(tc.coder.Status()), info.GetMetadata()["status"])
		})
	}
}
