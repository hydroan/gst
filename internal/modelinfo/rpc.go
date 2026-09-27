package modelinfo

import (
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/stoewer/go-strcase"
)

// This file holds what the gRPC side derives from a model and its design:
// the name of the rpc of an action, and the Go package the protobuf stubs of
// a model directory are generated into. The pb generator names the rpcs and
// the stubs with them, and the test scaffolds call the rpcs by them.

// RPCName names the rpc of an action on a route: the action name (Create,
// DeleteMany), or the role name of an action naming its service (Merge for
// Service("merge")), then the model name, then the suffix rpcSuffix derives
// from the route: CreateRecord, MergeItem, ListDocumentByBox. The rpc name
// carries the model so that the message names the pb generator derives from
// it are the ones Buf's standard rules want, GetRecordRequest for GetRecord.
// The AIPs would pluralize List and call the batch actions BatchCreate...,
// but one rule, action then model, keeps every rpc name equal to its message
// names without their suffix, so those forms are not followed.
func RPCName(m *Model, route string, action *dsl.Action) string {
	return rpcBase(action) + m.ModelName + rpcSuffix(m, route)
}

// rpcBase is the name of the action itself, the service name aside (see RPCName).
func rpcBase(action *dsl.Action) string {
	if action.ServiceName != "" {
		return action.RoleName()
	}
	return action.Phase.Name()
}

// rpcSuffix distinguishes the rpcs of one action declared on several routes
// by the path parameters a route adds to the model's own, those of its
// endpoint and its item parameter (see ItemParam): "" for a route adding
// none, such as items/:id/seal on a model whose endpoint is
// records/:record/items, ByBox for archive/boxes/:box/documents on a model
// whose own route is archive/documents/:document, and ByBoxAndShelf for a
// route adding box and shelf.
func rpcSuffix(m *Model, route string) string {
	own := append(RouteParams(m.Design.Endpoint), strings.TrimPrefix(ItemParam(m.Design), ":"))
	var extra []string
	for _, param := range RouteParams(route) {
		if !slices.Contains(own, param) {
			extra = append(extra, strcase.UpperCamelCase(param))
		}
	}
	if len(extra) == 0 {
		return ""
	}
	return "By" + strings.Join(extra, "And")
}

// RouteParams returns the names of the path parameters of route, written
// :name or {name}, in order: box and shelf for
// archive/boxes/:box/shelves/{shelf}/documents.
func RouteParams(route string) []string {
	var params []string
	for part := range strings.SplitSeq(route, "/") {
		switch {
		case strings.HasPrefix(part, ":"):
			params = append(params, strings.TrimPrefix(part, ":"))
		case strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}"):
			params = append(params, strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}"))
		}
	}
	return params
}

// PBPackage returns the import path and the package name of the Go package
// the protobuf stubs of the model directory dir, relative to pb/, are
// generated into: example.com/app/pb and pb for the root directory ".",
// example.com/app/pb/archive/document and document for archive/document,
// and example.com/app/pb/record_item and recorditem for record_item.
func PBPackage(modulePath, dir string) (importPath, name string) {
	if dir == "." {
		return path.Join(modulePath, ggconst.DirPB), ggconst.DirPB
	}
	return path.Join(modulePath, ggconst.DirPB, dir), ModelPackageName(path.Base(dir))
}

// PBDir returns the directory of the model's protobuf stubs, relative to
// pb/, which mirrors the model file's directory under model/: "." for
// model/feed.go and board for model/board/feed.go.
func (m *Model) PBDir() string {
	return path.Dir(filepath.ToSlash(strings.TrimPrefix(m.ModelFilePath, ggconst.DirModel+"/")))
}
