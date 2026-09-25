package pb

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/modelinfo"
	"github.com/stoewer/go-strcase"
)

// This file holds the naming rules of the definitions: protobuf packages, Go
// packages, rpc names and the messages of the standard actions.

// nonIdentifier matches a character a protobuf identifier cannot hold.
var nonIdentifier = regexp.MustCompile(`[^A-Za-z0-9_]`)

// identifier matches a protobuf identifier.
var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// protoPackage names the protobuf package of the files of dir, a directory
// relative to pb/: the application name, the last element of the module path,
// followed by the directory elements, every character an identifier cannot
// hold replaced. It returns app for the root directory ".", app.archive for
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
// http_client for http-client and _v2 for v2 is not touched, while 2fa gives
// _2fa.
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
// the root directory "." and example.com/app/pb/archive/document;document
// for archive/document, the package named as gg check names a model package
// (see modelinfo.ModelPackageName).
func goPackageOption(modulePath, dir string) string {
	if dir == "." {
		return path.Join(modulePath, dirPB) + ";" + dirPB
	}
	return path.Join(modulePath, dirPB, dir) + ";" + modelinfo.ModelPackageName(path.Base(dir))
}

// dirPB is the directory the definitions go to, beside the model directory.
const dirPB = "pb"

// rpcName names the rpc of an action on a route: the action name (Create,
// DeleteMany), or the role name of an action declaring Filename (Merge for
// Filename("merge")), then the model name, then the suffix rpcSuffix derives
// from the route: CreateRecord, MergeItem, ListDocumentByBox. The rpc name
// carries the model so that the message names messageName derives from it
// read as the AIP and Buf conventions want, GetRecordRequest for GetRecord.
func rpcName(m *modelinfo.Model, route string, action *dsl.Action) string {
	return rpcBase(action) + m.ModelName + rpcSuffix(m, route)
}

// rpcBase is the name of the action itself, Filename aside (see rpcName).
func rpcBase(action *dsl.Action) string {
	if action.Filename != "" {
		return action.RoleName()
	}
	return action.Phase.MethodName()
}

// rpcSuffix distinguishes the rpcs of one action declared on several routes
// by the path parameters a route adds to the model's own, those of its
// endpoint and its item parameter (see modelinfo.ItemParam): "" for a route
// adding none, such as items/:id/seal on a model whose endpoint is
// records/:record/items, ByBox for archive/boxes/:box/documents on a model
// whose own route is archive/documents/:document, and ByBoxAndShelf for a
// route adding box and shelf.
func rpcSuffix(m *modelinfo.Model, route string) string {
	own := append(routeParams(m.Design.Endpoint), strings.TrimPrefix(modelinfo.ItemParam(m.Design), ":"))
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

// routeParams returns the names of the path parameters of route, written
// :name or {name}, in order: box and shelf for
// archive/boxes/:box/shelves/{shelf}/documents.
func routeParams(route string) []string {
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

// messageName names the request or response message of an rpc: the rpc name
// followed by kind, Request or Response, as in CreateRecordRequest,
// ListDocumentByBoxRequest and MergeEntryResponse. Every rpc owns its two
// messages, however alike another rpc's, so that one of them can grow without
// touching the other.
func messageName(m *modelinfo.Model, route string, action *dsl.Action, kind string) string {
	return rpcName(m, route, action) + kind
}

// requestParam is a route parameter as the request message of an rpc carries
// it: param as the route writes it, name the string field holding it, and
// the comment of the field.
type requestParam struct {
	param   string
	name    string
	comment string
}

// requestParams lists the parameters of the route the router registers the
// action under (see modelinfo.RouterTargetForAction), in route order, as the
// leading fields of the rpc's request message: gRPC has no path to read them
// from. The model's own item parameter, :id when the design declares no
// Param, becomes the field id, "the id of the Item", "the id of the Item to
// delete" for its Delete action; every other parameter becomes a field named
// after itself, record for :record and box_id for :box-id, "the :record
// parameter of records/:record/items". The Get action of Item on
// records/:record/items, registered on records/:record/items/:id, carries
// record and id; its Create carries record alone; a Delete of Document,
// registered on archive/documents/:document for Param("document"), carries
// id; DeleteMany, registered on archive/documents/batch, carries nothing.
func requestParams(m *modelinfo.Model, route string, action *dsl.Action) []requestParam {
	registered, _ := modelinfo.RouterTargetForAction(route, m.Design, action)
	own := strings.TrimPrefix(modelinfo.ItemParam(m.Design), ":")
	var params []requestParam
	for _, param := range routeParams(registered) {
		if param == own {
			params = append(params, requestParam{param: param, name: "id", comment: "the id of the " + m.ModelName + ownParamPurpose(action.Phase)})
			continue
		}
		params = append(params, requestParam{param: param, name: protoIdentifier(param), comment: "the :" + param + " parameter of " + registered})
	}
	return params
}

// ownParamPurpose is what the item actions do to the record the id names,
// appended to the comment of the id field: " to delete", " to replace",
// " to patch", and nothing for the others.
func ownParamPurpose(phase consts.Phase) string {
	switch phase {
	case consts.PHASE_DELETE:
		return " to delete"
	case consts.PHASE_UPDATE:
		return " to replace"
	case consts.PHASE_PATCH:
		return " to patch"
	}
	return ""
}

// modelFieldName names the field carrying the model in a standard message:
// the model name in snake case, record for Record and item_link for ItemLink.
func modelFieldName(m *modelinfo.Model) string {
	return strcase.SnakeCase(m.ModelName)
}
