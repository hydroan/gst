package pb

import (
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/modelinfo"
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

// TestRPCNameJoinsActionModelAndRouteParameters pins the examples of the
// rpcName and rpcSuffix doc comments: the action name, or the role name of an
// action declaring Filename, then the model name, then the parameters a route
// adds to the model's own.
func TestRPCNameJoinsActionModelAndRouteParameters(t *testing.T) {
	document := &modelinfo.Model{ModelName: "Document", Design: &dsl.Design{Endpoint: "archive/documents", Param: ":document"}}
	list := &dsl.Action{Phase: consts.PHASE_LIST}

	require.Equal(t, "ListDocument", rpcName(document, "archive/documents", list))
	require.Equal(t, "ListDocumentByBox", rpcName(document, "archive/boxes/:box/documents", list))
	require.Equal(t, "ListDocumentByBoxAndShelf", rpcName(document, "archive/boxes/:box/shelves/{shelf}/documents", list))
	require.Equal(t, "MergeDocument", rpcName(document, "archive/documents/merge", &dsl.Action{Phase: consts.PHASE_CREATE, Filename: "merge"}))

	// A parameter the model's own route carries, propagated from a parent
	// resource, is not an addition.
	item := &modelinfo.Model{ModelName: "Item", Design: &dsl.Design{Endpoint: "records/:record/items"}}
	require.Equal(t, "DeleteManyItem", rpcName(item, "records/:record/items", &dsl.Action{Phase: consts.PHASE_DELETE_MANY}))

	// Nor is :id, the parameter of a model declaring no Param.
	require.Equal(t, "SealItem", rpcName(item, "items/:id/seal", &dsl.Action{Phase: consts.PHASE_CREATE, Filename: "seal"}))
	require.Equal(t, "SealItemByOwner", rpcName(item, "owners/:owner/items/:id/seal", &dsl.Action{Phase: consts.PHASE_CREATE, Filename: "seal"}))
}

// TestRequestParamsCarryEveryParameterOfTheRegisteredRoute pins the examples
// of the requestParams doc comment: one string field per parameter of the
// route the router registers the action under, in route order, the model's
// own item parameter named id and every other one after itself.
func TestRequestParamsCarryEveryParameterOfTheRegisteredRoute(t *testing.T) {
	item := &modelinfo.Model{ModelName: "Item", Design: &dsl.Design{Endpoint: "records/:record/items"}}
	document := &modelinfo.Model{ModelName: "Document", Design: &dsl.Design{Endpoint: "archive/documents", Param: ":document"}}

	require.Equal(t, []requestParam{
		{param: "record", name: "record", comment: "the :record parameter of /api/records/:record/items/:id"},
		{param: "id", name: "id", comment: "the id of the Item"},
	}, requestParams(item, "records/:record/items", &dsl.Action{Phase: consts.PHASE_GET}))
	require.Equal(t, []requestParam{{param: "record", name: "record", comment: "the :record parameter of /api/records/:record/items"}},
		requestParams(item, "records/:record/items", &dsl.Action{Phase: consts.PHASE_CREATE}))
	require.Equal(t, []requestParam{{param: "id", name: "id", comment: "the id of the Item"}},
		requestParams(item, "items/:id/seal", &dsl.Action{Phase: consts.PHASE_CREATE, Filename: "seal"}))
	require.Equal(t, []requestParam{{param: "document", name: "id", comment: "the id of the Document to delete"}},
		requestParams(document, "archive/documents", &dsl.Action{Phase: consts.PHASE_DELETE}))
	require.Equal(t, []requestParam{{param: "box-id", name: "box_id", comment: "the :box-id parameter of /api/archive/boxes/:box-id/documents"}},
		requestParams(document, "archive/boxes/:box-id/documents", &dsl.Action{Phase: consts.PHASE_LIST}))
	require.Empty(t, requestParams(document, "archive/documents", &dsl.Action{Phase: consts.PHASE_DELETE_MANY}))
}

// TestMessageNameIsTheRPCNameAndTheKind pins the examples of the messageName
// doc comment.
func TestMessageNameIsTheRPCNameAndTheKind(t *testing.T) {
	record := &modelinfo.Model{ModelName: "Record", Design: &dsl.Design{Endpoint: "records"}}
	document := &modelinfo.Model{ModelName: "Document", Design: &dsl.Design{Endpoint: "archive/documents", Param: ":document"}}
	entry := &modelinfo.Model{ModelName: "Entry", Design: &dsl.Design{Endpoint: "entries"}}

	require.Equal(t, "CreateRecordRequest", messageName(record, "records", &dsl.Action{Phase: consts.PHASE_CREATE}, "Request"))
	require.Equal(t, "ListDocumentByBoxRequest", messageName(document, "archive/boxes/:box/documents", &dsl.Action{Phase: consts.PHASE_LIST}, "Request"))
	require.Equal(t, "MergeEntryResponse", messageName(entry, "entries/merge", &dsl.Action{Phase: consts.PHASE_CREATE, Filename: "merge"}, "Response"))
}

// TestModelFieldNameIsTheModelInSnakeCase pins the examples of the
// modelFieldName doc comment.
func TestModelFieldNameIsTheModelInSnakeCase(t *testing.T) {
	require.Equal(t, "record", modelFieldName(&modelinfo.Model{ModelName: "Record"}))
	require.Equal(t, "item_link", modelFieldName(&modelinfo.Model{ModelName: "ItemLink"}))
}

// TestCommentTextRecordsCommentsLikeProtoc pins the examples of the
// commentText doc comment.
func TestCommentTextRecordsCommentsLikeProtoc(t *testing.T) {
	require.Equal(t, " Title is the display title.\n", commentText("Title is the display title."))
	require.Equal(t, " Title is the display title.\n\n Two lines.\n", commentText("Title is the display title.\n\nTwo lines.\n"))
}
