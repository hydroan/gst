package modelinfo_test

import (
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/modelinfo"
	"github.com/stretchr/testify/require"
)

// TestRPCNameJoinsActionModelAndRouteParameters pins the examples of the
// RPCName and rpcSuffix doc comments: the action name, or the role name of
// an action naming its service, then the model name, then the parameters a
// route adds to the model's own.
func TestRPCNameJoinsActionModelAndRouteParameters(t *testing.T) {
	document := &modelinfo.Model{ModelName: "Document", Design: &dsl.Design{Endpoint: "archive/documents", Param: ":document"}}
	list := &dsl.Action{Phase: consts.List}
	require.Equal(t, "ListDocument", modelinfo.RPCName(document, "archive/documents", list))
	require.Equal(t, "ListDocumentByBox", modelinfo.RPCName(document, "archive/boxes/:box/documents", list))
	require.Equal(t, "ListDocumentByBoxAndShelf", modelinfo.RPCName(document, "archive/boxes/:box/shelves/:shelf/documents", list))
	require.Equal(t, "MergeDocument", modelinfo.RPCName(document, "archive/documents/merge", &dsl.Action{Phase: consts.Create, ServiceName: "merge"}))

	// A nested model's own parameters are those of its endpoint and its
	// item parameter, :id when it declares none.
	item := &modelinfo.Model{ModelName: "Item", Design: &dsl.Design{Endpoint: "records/:record/items"}}
	require.Equal(t, "DeleteManyItem", modelinfo.RPCName(item, "records/:record/items", &dsl.Action{Phase: consts.DeleteMany}))
	require.Equal(t, "SealItem", modelinfo.RPCName(item, "items/:id/seal", &dsl.Action{Phase: consts.Create, ServiceName: "seal"}))
	require.Equal(t, "SealItemByOwner", modelinfo.RPCName(item, "owners/:owner/items/:id/seal", &dsl.Action{Phase: consts.Create, ServiceName: "seal"}))
}

// TestRouteParamsReadsBothParameterForms pins the example of the RouteParams
// doc comment.
func TestRouteParamsReadsBothParameterForms(t *testing.T) {
	require.Equal(t, []string{"box", "shelf"}, modelinfo.RouteParams("archive/boxes/:box/shelves/:shelf/documents"))
	require.Empty(t, modelinfo.RouteParams("archive/documents"))
}

// TestAppNameIsTheModulePathWithoutItsVersion pins the examples of the
// AppName doc comment.
func TestAppNameIsTheModulePathWithoutItsVersion(t *testing.T) {
	require.Equal(t, "app", modelinfo.AppName("example.com/app"))
	require.Equal(t, "app", modelinfo.AppName("example.com/app/v2"))
	require.Equal(t, "http-client", modelinfo.AppName("example.com/http-client"))
}

// TestPBPackageMirrorsTheDirectoryUnderPB pins the examples of the PBPackage
// and PBDir doc comments.
func TestPBPackageMirrorsTheDirectoryUnderPB(t *testing.T) {
	importPath, name := modelinfo.PBPackage("example.com/app", ".")
	require.Equal(t, "example.com/app/pb", importPath)
	require.Equal(t, "pb", name)
	importPath, name = modelinfo.PBPackage("example.com/app", "archive/document")
	require.Equal(t, "example.com/app/pb/archive/document", importPath)
	require.Equal(t, "document", name)
	importPath, name = modelinfo.PBPackage("example.com/app", "record_item")
	require.Equal(t, "example.com/app/pb/record_item", importPath)
	require.Equal(t, "recorditem", name)

	require.Equal(t, ".", (&modelinfo.Model{ModelFilePath: "model/feed.go"}).PBDir())
	require.Equal(t, "board", (&modelinfo.Model{ModelFilePath: "model/board/feed.go"}).PBDir())
}
