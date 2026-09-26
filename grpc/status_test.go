package grpc_test

import (
	"net/http"
	"testing"

	"github.com/cockroachdb/errors"
	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestStatusErrorAnswersAServiceErrorWithItsStatus pins the mapping an
// interceptor relies on: a service error answers with the code its HTTP
// status maps to and its own message, any other error answers Internal
// without its text.
func TestStatusErrorAnswersAServiceErrorWithItsStatus(t *testing.T) {
	refused := status.Convert(gstgrpc.StatusError(serviceregistry.NewError(http.StatusForbidden, "not yours")))
	require.Equal(t, codes.PermissionDenied, refused.Code())
	require.Equal(t, "not yours", refused.Message())

	broken := status.Convert(gstgrpc.StatusError(errors.New("dial tcp: connection refused")))
	require.Equal(t, codes.Internal, broken.Code())
	require.Equal(t, "internal server error", broken.Message())
	require.NoError(t, gstgrpc.StatusError(nil))
}
