package grpc_test

import (
	"testing"

	"github.com/hydroan/gst/consts"
	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
)

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
