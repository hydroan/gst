package grpc_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// TestBearerReadsTheCredentialOfTheCall pins what Bearer answers: the
// credential of an "authorization: Bearer <credential>" metadata, and no
// credential for another scheme or no authorization at all.
func TestBearerReadsTheCredentialOfTheCall(t *testing.T) {
	credential, ok := gstgrpc.Bearer(metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer s-1")))
	require.True(t, ok)
	require.Equal(t, "s-1", credential)

	_, ok = gstgrpc.Bearer(metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Basic YWxpY2U6cGFzcw==")))
	require.False(t, ok)
	_, ok = gstgrpc.Bearer(context.Background())
	require.False(t, ok)
}

// TestWithCallerNamesTheCallerForTheContext pins the pair an interceptor
// uses: WithCaller puts the caller on the request metadata the service
// context reads, and CallerOf reads it back, the zero Caller outside a call
// or before one is established.
func TestWithCallerNamesTheCallerForTheContext(t *testing.T) {
	require.Equal(t, gstgrpc.Caller{}, gstgrpc.CallerOf(context.Background()))

	ctx := gstgrpc.WithCaller(context.Background(), gstgrpc.Caller{Username: "alice", UserID: "u-1", SessionID: "s-1", TenantID: "t-1"})

	meta := requestctx.FromContext(ctx)
	require.Equal(t, "alice", meta.Username())
	require.Equal(t, "u-1", meta.UserID())
	require.Equal(t, "s-1", meta.SessionID())
	require.Equal(t, "t-1", meta.TenantID())
}

// TestRouteIsEmptyOutsideACall pins that Route answers nothing for a
// context no call runs on.
func TestRouteIsEmptyOutsideACall(t *testing.T) {
	httpMethod, route := gstgrpc.Route(context.Background())
	require.Empty(t, httpMethod)
	require.Empty(t, route)
}

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

// sampleModel is a model type the call functions are built for; building
// one touches no database.
type sampleModel struct {
	Name string `json:"name"`

	modelregistry.Base
}

func (sampleModel) TableName() string { return "grpc_samples" }

// TestCallFunctionsAreBuiltForAModelAndRoute pins the entry points the
// generated pb package uses: every call function returns the call of the
// model's action on the route, ready to be shared by every call, and
// ServiceCall refuses the phase of an HTTP-only action as it is built.
func TestCallFunctionsAreBuiltForAModelAndRoute(t *testing.T) {
	require.NotNil(t, gstgrpc.CreateCall[*sampleModel]("samples"))
	require.NotNil(t, gstgrpc.GetCall[*sampleModel]("samples"))
	require.NotNil(t, gstgrpc.ListCall[*sampleModel]("samples"))
	require.NotNil(t, gstgrpc.UpdateCall[*sampleModel]("samples"))
	require.NotNil(t, gstgrpc.PatchCall[*sampleModel]("samples"))
	require.NotNil(t, gstgrpc.DeleteCall[*sampleModel]("samples"))
	require.NotNil(t, gstgrpc.CreateManyCall[*sampleModel]("samples"))
	require.NotNil(t, gstgrpc.UpdateManyCall[*sampleModel]("samples"))
	require.NotNil(t, gstgrpc.PatchManyCall[*sampleModel]("samples"))
	require.NotNil(t, gstgrpc.DeleteManyCall[*sampleModel]("samples"))
	require.NotNil(t, gstgrpc.ServiceCall[*sampleModel, *sampleModel, *sampleModel](consts.PHASE_CREATE, "samples/seal"))
	require.PanicsWithValue(t, `controller: phase "sse" has no rpc; ServiceCall serves the actions of a model's gRPC service`, func() {
		gstgrpc.ServiceCall[*sampleModel, *sampleModel, *sampleModel](consts.PHASE_SSE, "samples")
	})
}
