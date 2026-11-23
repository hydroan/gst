package pbschema

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/modelsource"
	"github.com/stoewer/go-strcase"
	"golang.org/x/mod/module"
)

// This file holds the naming rules of the definitions: protobuf packages, Go
// packages, rpc names and the messages of the standard actions.

// nonIdentifier matches a character a protobuf identifier cannot hold.
var nonIdentifier = regexp.MustCompile(`[^A-Za-z0-9_]`)

// identifier matches a protobuf identifier.
var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// AppName returns the name of the application the module path names, the
// last element of the path with its major version suffix left off: app for
// example.com/app and for example.com/app/v2, http-client for
// example.com/http-client. The protobuf definitions carry it in their
// package name.
func AppName(modulePath string) string {
	prefix, _, _ := module.SplitPathVersion(modulePath)
	return path.Base(prefix)
}

// protoPackage names the protobuf package of the files of dir, a directory
// relative to pb/: the application name (see AppName) followed by
// the directory elements, every character an identifier cannot hold
// replaced. It returns app for the root directory ".", app.archive for
// archive, app.archive.document for archive/document, and app.http_client
// for http-client.
func protoPackage(app, dir string) string {
	elements := []string{protoIdentifier(app)}
	if dir != "." {
		for element := range strings.SplitSeq(dir, "/") {
			elements = append(elements, protoIdentifier(element))
		}
	}
	return strings.Join(elements, ".")
}

// protoIdentifier makes s a protobuf identifier: every character one cannot
// hold becomes _, and a leading digit gets one in front. It returns
// http_client for http-client and _2fa for 2fa, and leaves v2 as it is.
func protoIdentifier(s string) string {
	s = nonIdentifier.ReplaceAllString(s, "_")
	if s == "" || (s[0] >= '0' && s[0] <= '9') {
		s = "_" + s
	}
	return s
}

// goPackageOption is the go_package option of the files of dir, relative to
// pb/: the import path of the Go package the stubs are generated into, then
// its package name after a semicolon. It returns example.com/app/pb;pb for
// the root directory ".", example.com/app/pb/archive/document;document for
// archive/document, and example.com/app/pb/record_item;recorditem for
// record_item.
func goPackageOption(modulePath, dir string) string {
	if dir == "." {
		return path.Join(modulePath, ggconst.DirPB) + ";" + ggconst.DirPB
	}
	return path.Join(modulePath, ggconst.DirPB, dir) + ";" + modelsource.ModelPackageName(path.Base(dir))
}

// RPCName names the rpc of an action on a route: the action name (Create,
// DeleteMany), or the role name of an action naming its service (Merge for
// Service("merge")), then the model name, then the suffix rpcSuffix derives
// from the route: CreateRecord, MergeItem, ListDocumentByBox. The rpc name
// carries the model so that the message names the pb generator derives from
// it are the ones Buf's standard rules want, GetRecordRequest for GetRecord.
// The AIPs would pluralize List and call the batch actions BatchCreate...,
// but one rule, action then model, keeps every rpc name equal to its message
// names without their suffix, so those forms are not followed.
func RPCName(m *modelsource.Model, route string, action *dsl.Action) string {
	return rpcBase(action) + m.ModelName + rpcSuffix(m, route)
}

// rpcBase is the name of the action itself, the service name aside (see
// RPCName).
func rpcBase(action *dsl.Action) string {
	if action.ServiceName != "" {
		return action.RoleName()
	}
	return action.Phase.Name()
}

// rpcSuffix distinguishes the rpcs of one action declared on several routes
// by the path parameters a route adds to the model's own, those of its
// endpoint and its item parameter (see modelsource.ItemParam): "" for a route adding
// none, such as items/:id/seal on a model whose endpoint is
// records/:record/items, ByBox for archive/boxes/:box/documents on a model
// whose own route is archive/documents/:document, and ByBoxAndShelf for a
// route adding box and shelf.
func rpcSuffix(m *modelsource.Model, route string) string {
	own := append(routeParams(m.Design.Endpoint), strings.TrimPrefix(modelsource.ItemParam(m.Design), ":"))
	var extra []string
	for _, param := range routeParams(route) {
		if !slices.Contains(own, param) {
			extra = append(extra, strcase.UpperCamelCase(param))
		}
	}
	if len(extra) == 0 {
		return ""
	}
	return "By" + strings.Join(extra, "And")
}

// messageName names the request or response message of an rpc: the rpc name
// followed by kind, Request or Response, as in CreateRecordRequest,
// ListDocumentByBoxRequest and MergeEntryResponse. Every rpc owns its two
// messages, however alike another rpc's, so that one of them can grow without
// touching the other.
func messageName(m *modelsource.Model, route string, action *dsl.Action, kind string) string {
	return RPCName(m, route, action) + kind
}

// routeParams returns the names of the path parameters of route, written :name,
// in order: box and shelf for archive/boxes/:box/shelves/:shelf/documents.
func routeParams(route string) []string {
	var params []string
	for part := range strings.SplitSeq(route, "/") {
		if name, ok := strings.CutPrefix(part, ":"); ok {
			params = append(params, name)
		}
	}
	return params
}

// RequestParam is a route parameter as the request message of an rpc carries
// it: param as the route writes it, name the string field holding it, and
// the comment of the field.
type RequestParam struct {
	Param   string
	Name    string
	comment string
}

// requestParams lists the parameters of the route the router registers the
// action under (see modelsource.RouterTargetForAction), in route order, as the
// leading fields of the rpc's request message: gRPC has no path to read them
// from. The model's own item parameter, :id when the design declares no
// Param, becomes the field id, "the id of the Item", "the id of the Item to
// delete" for its Delete action; every other parameter becomes a field named
// after itself, record for :record and box_id for :box-id, "the :record
// parameter of /api/records/:record/items". The Get action of Item on
// records/:record/items, registered on /api/records/:record/items/:id,
// carries record and id; its Create carries record alone; a Delete of
// Document, registered on /api/archive/documents/:document for
// Param("document"), carries id; DeleteMany, registered on
// /api/archive/documents/batch, carries nothing.
func requestParams(m *modelsource.Model, route string, action *dsl.Action) []RequestParam {
	registered, _ := modelsource.RouterTargetForAction(route, m.Design, action)
	own := strings.TrimPrefix(modelsource.ItemParam(m.Design), ":")
	var params []RequestParam
	for _, param := range routeParams(registered) {
		if param == own {
			params = append(params, RequestParam{Param: param, Name: "id", comment: "the id of the " + m.ModelName + ownParamPurpose(action.Phase)})
			continue
		}
		params = append(params, RequestParam{Param: param, Name: protoIdentifier(param), comment: "the :" + param + " parameter of " + registered})
	}
	return params
}

// ownParamPurpose is what the item actions do to the record the id names,
// appended to the comment of the id field: " to delete", " to replace",
// " to patch", and nothing for the others.
func ownParamPurpose(phase apiaction.Phase) string {
	switch phase {
	case apiaction.Delete:
		return " to delete"
	case apiaction.Update:
		return " to replace"
	case apiaction.Patch:
		return " to patch"
	}
	return ""
}

// ModelFieldName names the field carrying the model in a standard message:
// the model name in snake case, record for Record and item_link for ItemLink.
func ModelFieldName(m *modelsource.Model) string {
	return strcase.SnakeCase(m.ModelName)
}
