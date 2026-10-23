package grpc_test

import (
	"testing"

	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	require.NotNil(t, gstgrpc.ServiceCall[*sampleModel, *sampleModel, *sampleModel](consts.Create, "samples/seal"))
	require.PanicsWithValue(t, `controller: phase "sse" has no rpc; ServiceCall serves the actions of a model's gRPC service`, func() {
		gstgrpc.ServiceCall[*sampleModel, *sampleModel, *sampleModel](consts.SSE, "samples")
	})
}

// TestItemErrorNamesTheItemOfARefusedRecord pins ItemError, what a generated
// batch handler answers an item's FromProto refusal through: the field
// refused is named as the item's, items[1].count, in the message and in the
// BadRequest detail alike, the description kept.
func TestItemErrorNamesTheItemOfARefusedRecord(t *testing.T) {
	_, refused := gstgrpc.Narrow[int8]("count", int32(300))

	err := gstgrpc.ItemError(1, refused)

	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Equal(t, "invalid value for field 'items[1].count'", status.Convert(err).Message())
	requireFieldViolation(t, err, "items[1].count", "300 is out of range")
}

// TestPathErrorNamesTheElementOfARefusedValue pins PathError with Element
// and Entry, what a generated conversion names the refused value of a
// nested message held in a container through: the path of the element or
// the map value in front of the field refused, in the message and in the
// BadRequest detail alike.
func TestPathErrorNamesTheElementOfARefusedValue(t *testing.T) {
	_, refused := gstgrpc.Narrow[int8]("x", int32(300))

	err := gstgrpc.PathError(gstgrpc.Element("points", 1), refused)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Equal(t, "invalid value for field 'points[1].x'", status.Convert(err).Message())
	requireFieldViolation(t, err, "points[1].x", "300 is out of range")

	err = gstgrpc.PathError(gstgrpc.Entry("by_code", int32(7)), refused)
	require.Equal(t, "invalid value for field 'by_code[7].x'", status.Convert(err).Message())
}
