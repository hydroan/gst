package pbschema_test

import (
	"testing"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/modelsource"
	"github.com/hydroan/gst/internal/pbschema"
	"github.com/stretchr/testify/require"
)

// TestRPCNameJoinsActionModelAndRouteParameters pins the examples of the
// RPCName and rpcSuffix doc comments: the action name, or the role name of
// an action naming its service, then the model name, then the parameters a
// route adds to the model's own.
func TestRPCNameJoinsActionModelAndRouteParameters(t *testing.T) {
	document := &modelsource.Model{ModelName: "Document", Design: &dsl.Design{Endpoint: "archive/documents", Param: ":document"}}
	list := &dsl.Action{Phase: apiaction.List}
	require.Equal(t, "ListDocument", pbschema.RPCName(document, "archive/documents", list))
	require.Equal(t, "ListDocumentByBox", pbschema.RPCName(document, "archive/boxes/:box/documents", list))
	require.Equal(t, "ListDocumentByBoxAndShelf", pbschema.RPCName(document, "archive/boxes/:box/shelves/:shelf/documents", list))
	require.Equal(t, "MergeDocument", pbschema.RPCName(document, "archive/documents/merge", &dsl.Action{Phase: apiaction.Create, ServiceName: "merge"}))

	// A nested model's own parameters are those of its endpoint and its
	// item parameter, :id when it declares none.
	item := &modelsource.Model{ModelName: "Item", Design: &dsl.Design{Endpoint: "records/:record/items"}}
	require.Equal(t, "DeleteManyItem", pbschema.RPCName(item, "records/:record/items", &dsl.Action{Phase: apiaction.DeleteMany}))
	require.Equal(t, "SealItem", pbschema.RPCName(item, "items/:id/seal", &dsl.Action{Phase: apiaction.Create, ServiceName: "seal"}))
	require.Equal(t, "SealItemByOwner", pbschema.RPCName(item, "owners/:owner/items/:id/seal", &dsl.Action{Phase: apiaction.Create, ServiceName: "seal"}))
}

// TestAppNameIsTheModulePathWithoutItsVersion pins the examples of the
// AppName doc comment.
func TestAppNameIsTheModulePathWithoutItsVersion(t *testing.T) {
	require.Equal(t, "app", pbschema.AppName("example.com/app"))
	require.Equal(t, "app", pbschema.AppName("example.com/app/v2"))
	require.Equal(t, "http-client", pbschema.AppName("example.com/http-client"))
}
