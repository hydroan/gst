package pb

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen"
	"github.com/hydroan/gst/internal/codegen/gen"
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
// (see gen.ModelPackageName).
func goPackageOption(modulePath, dir string) string {
	if dir == "." {
		return path.Join(modulePath, dirPB) + ";" + dirPB
	}
	return path.Join(modulePath, dirPB, dir) + ";" + gen.ModelPackageName(path.Base(dir))
}

// dirPB is the directory the definitions go to, beside the model directory.
const dirPB = "pb"

// rpcName names the rpc of an action on a route: the action name (Create,
// DeleteMany), or the role name of an action declaring Filename (Merge for
// Filename("merge")), followed by the suffix rpcSuffix derives from the route.
func rpcName(m *gen.ModelInfo, route string, action *dsl.Action) string {
	return rpcBase(action) + rpcSuffix(m, route)
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
// endpoint and its item parameter (see codegen.ItemParam): "" for a route
// adding none, such as items/:id/seal on a model whose endpoint is
// records/:record/items, ByBox for archive/boxes/:box/documents on a model
// whose own route is archive/documents/:document, and ByBoxAndShelf for a
// route adding box and shelf.
func rpcSuffix(m *gen.ModelInfo, route string) string {
	own := append(routeParams(m.Design.Endpoint), strings.TrimPrefix(codegen.ItemParam(m.Design), ":"))
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

// standardMessageName names the request or response message of the rpc of an
// action: the action name, the model name, the route suffix, then Request or
// Response, as in CreateRecordRequest, ListDocumentByBoxRequest and
// MergeEntryResponse.
func standardMessageName(m *gen.ModelInfo, route string, action *dsl.Action, kind string) string {
	return rpcBase(action) + m.ModelName + rpcSuffix(m, route) + kind
}

// modelFieldName names the field carrying the model in a standard message:
// the model name in snake case, record for Record and item_link for ItemLink.
func modelFieldName(m *gen.ModelInfo) string {
	return strcase.SnakeCase(m.ModelName)
}
