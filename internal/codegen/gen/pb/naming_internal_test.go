package pb

import (
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/stretchr/testify/require"
)

// TestProtoPackageJoinsTheAppAndTheDirectory pins the examples of the
// protoPackage and protoIdentifier doc comments.
func TestProtoPackageJoinsTheAppAndTheDirectory(t *testing.T) {
	require.Equal(t, "app", protoPackage("app", "."))
	require.Equal(t, "app.archive", protoPackage("app", "archive"))
	require.Equal(t, "app.archive.document", protoPackage("app", "archive/document"))
	require.Equal(t, "app.http_client", protoPackage("app", "http-client"))
	require.Equal(t, "my_app._2fa", protoPackage("my-app", "2fa"))
}

// TestGoPackageOptionMirrorsTheDirectoryUnderPB pins the examples of the
// goPackageOption doc comment.
func TestGoPackageOptionMirrorsTheDirectoryUnderPB(t *testing.T) {
	require.Equal(t, "example.com/app/pb;pb", goPackageOption("example.com/app", "."))
	require.Equal(t, "example.com/app/pb/archive/document;document", goPackageOption("example.com/app", "archive/document"))
	require.Equal(t, "example.com/app/pb/record_item;recorditem", goPackageOption("example.com/app", "record_item"))
}

// TestRPCNameSuffixesTheParametersARouteAdds pins the examples of the rpcName
// and rpcSuffix doc comments: the action name, or the role name of an action
// declaring Filename, followed by the parameters a route adds to the model's
// own.
func TestRPCNameSuffixesTheParametersARouteAdds(t *testing.T) {
	document := &gen.ModelInfo{ModelName: "Document", Design: &dsl.Design{Endpoint: "archive/documents", Param: ":document"}}
	list := &dsl.Action{Phase: consts.PHASE_LIST}

	require.Equal(t, "List", rpcName(document, "archive/documents", list))
	require.Equal(t, "ListByBox", rpcName(document, "archive/boxes/:box/documents", list))
	require.Equal(t, "ListByBoxAndShelf", rpcName(document, "archive/boxes/:box/shelves/{shelf}/documents", list))
	require.Equal(t, "Merge", rpcName(document, "archive/documents/merge", &dsl.Action{Phase: consts.PHASE_CREATE, Filename: "merge"}))

	// A parameter the model's own route carries, propagated from a parent
	// resource, is not an addition.
	item := &gen.ModelInfo{ModelName: "Item", Design: &dsl.Design{Endpoint: "records/:record/items"}}
	require.Equal(t, "DeleteMany", rpcName(item, "records/:record/items", &dsl.Action{Phase: consts.PHASE_DELETE_MANY}))

	// Nor is :id, the parameter of a model declaring no Param.
	require.Equal(t, "Seal", rpcName(item, "items/:id/seal", &dsl.Action{Phase: consts.PHASE_CREATE, Filename: "seal"}))
	require.Equal(t, "SealByOwner", rpcName(item, "owners/:owner/items/:id/seal", &dsl.Action{Phase: consts.PHASE_CREATE, Filename: "seal"}))
}

// TestStandardMessageNameReadsActionModelSuffixKind pins the examples of the
// standardMessageName doc comment.
func TestStandardMessageNameReadsActionModelSuffixKind(t *testing.T) {
	record := &gen.ModelInfo{ModelName: "Record", Design: &dsl.Design{Endpoint: "records"}}
	document := &gen.ModelInfo{ModelName: "Document", Design: &dsl.Design{Endpoint: "archive/documents", Param: ":document"}}
	entry := &gen.ModelInfo{ModelName: "Entry", Design: &dsl.Design{Endpoint: "entries"}}

	require.Equal(t, "CreateRecordRequest", standardMessageName(record, "records", &dsl.Action{Phase: consts.PHASE_CREATE}, "Request"))
	require.Equal(t, "ListDocumentByBoxRequest", standardMessageName(document, "archive/boxes/:box/documents", &dsl.Action{Phase: consts.PHASE_LIST}, "Request"))
	require.Equal(t, "MergeEntryResponse", standardMessageName(entry, "entries/merge", &dsl.Action{Phase: consts.PHASE_CREATE, Filename: "merge"}, "Response"))
}

// TestModelFieldNameIsTheModelInSnakeCase pins the examples of the
// modelFieldName doc comment.
func TestModelFieldNameIsTheModelInSnakeCase(t *testing.T) {
	require.Equal(t, "record", modelFieldName(&gen.ModelInfo{ModelName: "Record"}))
	require.Equal(t, "item_link", modelFieldName(&gen.ModelInfo{ModelName: "ItemLink"}))
}

// TestCommentTextRecordsCommentsLikeProtoc pins the examples of the
// commentText doc comment.
func TestCommentTextRecordsCommentsLikeProtoc(t *testing.T) {
	require.Equal(t, " Title is the display title.\n", commentText("Title is the display title."))
	require.Equal(t, " Title is the display title.\n\n Two lines.\n", commentText("Title is the display title.\n\nTwo lines.\n"))
}
