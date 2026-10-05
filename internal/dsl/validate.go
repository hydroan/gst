package dsl

import (
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/goast"
)

// actionMethodPhases maps DSL action method names to their phases. It serves
// both as the action keyword set and as the phase lookup for generation
// facts derived from an action call, such as the service filename.
var actionMethodPhases = map[string]consts.Phase{
	consts.Create.Name():     consts.Create,
	consts.Delete.Name():     consts.Delete,
	consts.Update.Name():     consts.Update,
	consts.Patch.Name():      consts.Patch,
	consts.List.Name():       consts.List,
	consts.Get.Name():        consts.Get,
	consts.CreateMany.Name(): consts.CreateMany,
	consts.DeleteMany.Name(): consts.DeleteMany,
	consts.UpdateMany.Name(): consts.UpdateMany,
	consts.PatchMany.Name():  consts.PatchMany,
	consts.Import.Name():     consts.Import,
	consts.Export.Name():     consts.Export,
	consts.SSE.Name():        consts.SSE,
	consts.Stream.Name():     consts.Stream,
}

func isActionMethod(name string) bool {
	_, ok := actionMethodPhases[name]
	return ok
}

// routeIDActionMethodNames are actions whose built-in controllers read the
// resource id from the route parameter only. Exact() removes the default
// "/:id" suffix from the generated route, so these actions must delegate to a
// custom service method when Exact() is used (via Payload/Result, or Result
// alone for the GET-verb Get action); otherwise the generated route can never
// resolve a resource id.
var routeIDActionMethodNames = map[string]bool{
	consts.Delete.Name(): true,
	consts.Update.Name(): true,
	consts.Patch.Name():  true,
	consts.Get.Name():    true,
}

// getVerbActionMethodNames are actions whose generated routes handle HTTP GET
// requests. A GET request carries no request body, so these actions must not
// declare Payload; custom services read filters from ServiceContext.Query().
var getVerbActionMethodNames = map[string]bool{
	consts.List.Name(): true,
	consts.Get.Name():  true,
}

// fixedContractActionSignatures are actions whose service methods have fixed
// signatures that never bind Payload or Result types, mapped to those
// signatures for error reporting. The Import controller reads the uploaded
// multipart form file and responds with a bare status code; the Export
// controller handles an HTTP GET request, reads filters from query
// parameters, and writes the returned bytes as a file attachment; the SSE
// controller handles an HTTP GET request whose response is the event stream
// the service opens through ServiceContext.SSE.
var fixedContractActionSignatures = map[string]string{
	consts.Import.Name(): "Import(ctx, io.Reader) ([]M, error)",
	consts.Export.Name(): "Export(ctx, ...M) ([]byte, error)",
	consts.SSE.Name():    "SSE(ctx) error",
}

// serviceRequiredActionMethodNames are actions whose request cannot be
// answered without a custom service: their fixed-contract service method,
// or the stream method of a Stream, is the whole implementation, so
// declaring them without Service() is a wiring error caught at generation
// time.
var serviceRequiredActionMethodNames = map[string]bool{
	consts.Import.Name(): true,
	consts.Export.Name(): true,
	consts.SSE.Name():    true,
	consts.Stream.Name(): true,
}

var designOnlyMethodNames = map[string]bool{
	"Endpoint": true,
	"Param":    true,
	"Migrate":  true,
	"GRPC":     true,
}

// httpOnlyActionMethodNames are actions gRPC cannot serve: Import reads a
// multipart upload, Export answers with a file attachment and SSE is an HTTP
// protocol of its own. A model declaring GRPC() needs at least one other
// action, or its gRPC service would have nothing to serve.
var httpOnlyActionMethodNames = map[string]bool{
	consts.Import.Name(): true,
	consts.Export.Name(): true,
	consts.SSE.Name():    true,
}

// HTTPOnlyAction reports whether the action named name is one gRPC cannot
// serve (see httpOnlyActionMethodNames): true for Import, Export and SSE,
// false for Create or List. The generator leaves these actions out of a
// model's gRPC service, and gg check leaves their service files alone.
func HTTPOnlyAction(name string) bool {
	return httpOnlyActionMethodNames[name]
}

// grpcOnlyActionMethodNames are actions HTTP cannot serve: a Stream carries
// a stream of messages on one side of the call or both, which only gRPC
// does. A model declaring one needs GRPC(), or the action would be served
// nowhere.
var grpcOnlyActionMethodNames = map[string]bool{
	consts.Stream.Name(): true,
}

// GRPCOnlyAction reports whether the action named name is one HTTP cannot
// serve (see grpcOnlyActionMethodNames): true for Stream, false for Create
// or SSE. The generator registers no route, generates no service and
// declares no TypeScript type for these actions, and the route ignore rules
// of gst.yaml, written as HTTP methods and paths, never match them.
func GRPCOnlyAction(name string) bool {
	return grpcOnlyActionMethodNames[name]
}

// PayloadKeyword reports whether name is the keyword declaring the request
// side of an action's types: true for Payload and StreamingPayload, false
// for Result or Service. ResultKeyword reports the response side, Result
// and StreamingResult, and StreamingKeyword the sides a Stream action
// streams, StreamingPayload and StreamingResult. The parser, the validator
// and gg check read the typed keywords through these alone.
func PayloadKeyword(name string) bool {
	return name == "Payload" || name == "StreamingPayload"
}

// ResultKeyword reports whether name declares the response side of an
// action's types (see PayloadKeyword): true for Result and StreamingResult.
func ResultKeyword(name string) bool {
	return name == "Result" || name == "StreamingResult"
}

// StreamingKeyword reports whether name declares a side a Stream action
// streams (see PayloadKeyword): true for StreamingPayload and
// StreamingResult.
func StreamingKeyword(name string) bool {
	return name == "StreamingPayload" || name == "StreamingResult"
}

// serviceNamePattern is what Service("name") accepts: a bare name of
// letters, digits and underscores, starting with a letter, which names the
// service file, the service type and, for a model declaring GRPC(), the rpc.
var serviceNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

var actionOnlyMethodNames = map[string]bool{
	"Service":          true,
	"Public":           true,
	"Exact":            true,
	"Payload":          true,
	"Result":           true,
	"Flatten":          true,
	"StreamingPayload": true,
	"StreamingResult":  true,
}

// Validate checks DSL keyword placement and generation semantics for one model file.
// It intentionally validates only model Design() methods for structs embedding
// model.Base, model.AutoBase or model.Empty, matching Parse's model discovery
// scope, and rejects the embeddings that scope cannot see: those base types
// embedded through a pointer (see validatePointerEmbeddings).
// Do not duplicate Go compiler or type-checker diagnostics here, such as wrong
// argument counts or incompatible argument types. Keep this validator focused on
// DSL structure, keyword placement, and generation-specific semantics.
func Validate(file *ast.File, modelDir string, filename string) []error {
	designBase, designEmpty := parse(file)
	for name := range designBase {
		delete(designEmpty, name)
	}

	errs := validatePointerEmbeddings(file, filename)
	records := make([]serviceActionRecord, 0)
	rootModelFile := isRootModelFile(file, modelDir, filename)
	for _, name := range slices.Sorted(maps.Keys(designBase)) {
		recs, designErrs := validateDesignFunc(designBase[name], name, rootModelFile, false, filename)
		records = append(records, recs...)
		errs = append(errs, designErrs...)
	}
	// Models embedding model.Empty are virtual: routes exist but no table
	// backs them, so actions relying on built-in table access are rejected.
	for _, name := range slices.Sorted(maps.Keys(designEmpty)) {
		recs, designErrs := validateDesignFunc(designEmpty[name], name, rootModelFile, true, filename)
		records = append(records, recs...)
		errs = append(errs, designErrs...)
	}
	errs = append(errs, validateServiceCollisions(records, filename)...)
	return errs
}

// validatePointerEmbeddings rejects every struct of the file that embeds one of
// the framework's base types, model.Base, model.AutoBase or model.Empty,
// through a pointer. The framework recognizes a base type embedded by value
// only: gg gen generates nothing for the pointer form, and at run time a
// database model's nil *model.Base has no id to set while a *model.Empty is
// taken for a database model without a table name. For
//
//	type Record struct {
//		*model.Base
//	}
//
// it reports "struct Record embeds *model.Base; embed model.Base by value: the
// framework recognizes model.Base only when it is embedded by value".
func validatePointerEmbeddings(file *ast.File, filename string) []error {
	names := goast.ImportedNames(file, ggconst.ImportPathModel, ggconst.PkgModel)
	errs := make([]error, 0)
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name == nil {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || structType.Fields == nil {
				continue
			}
			for _, field := range structType.Fields.List {
				star, ok := field.Type.(*ast.StarExpr)
				if !ok || len(field.Names) != 0 {
					continue
				}
				for _, baseType := range []string{ggconst.FieldBase, ggconst.FieldAutoBase, ggconst.FieldEmpty} {
					if names.Refers(star.X, baseType) {
						errs = append(errs, fmt.Errorf("%s: struct %s embeds *model.%s; embed model.%[3]s by value: the framework recognizes model.%[3]s only when it is embedded by value", filename, typeSpec.Name.Name, baseType))
					}
				}
			}
		}
	}
	return errs
}

func validateDesignFunc(fn *ast.FuncDecl, modelName string, rootModelFile, virtual bool, filename string) ([]serviceActionRecord, []error) {
	if fn == nil || fn.Body == nil {
		return nil, nil
	}

	records := make([]serviceActionRecord, 0)
	errs := make([]error, 0)
	seenActions := make(map[string]bool)
	grpc, grpcServable, grpcOnly := false, false, false
	for _, stmt := range fn.Body.List {
		call, name, ok := keywordCall(stmt)
		if !ok {
			errs = append(errs, fmt.Errorf("%s: Design() of %s reads DSL keywords alone; delete %s", filename, modelName, nodeSource(stmt)))
			continue
		}

		switch {
		case isActionMethod(name):
			if seenActions[name] {
				errs = append(errs, fmt.Errorf("%s: %s declares %s twice at Design() top level; declare an action once", filename, modelName, name))
			}
			seenActions[name] = true
			grpcServable = grpcServable || !httpOnlyActionMethodNames[name]
			grpcOnly = grpcOnly || grpcOnlyActionMethodNames[name]
			info, actionErrs := validateActionCall(call, name, rootModelFile, virtual, filename)
			if record, ok := newServiceActionRecord(info, name, modelName, ""); ok {
				records = append(records, record)
			}
			errs = append(errs, actionErrs...)
		case name == "Route":
			recs, servable, only, routeErrs := validateRouteCall(call, modelName, rootModelFile, virtual, filename)
			records = append(records, recs...)
			grpcServable = grpcServable || servable
			grpcOnly = grpcOnly || only
			errs = append(errs, routeErrs...)
		case name == "GRPC":
			grpc = true
		case designOnlyMethodNames[name]:
			continue
		case actionOnlyMethodNames[name]:
			errs = append(errs, fmt.Errorf("%s: %s() can only be used inside an action block", filename, name))
		}
	}
	if grpc && !grpcServable {
		errs = append(errs, fmt.Errorf("%s: %s declares GRPC() but no action gRPC can serve: Import, Export and SSE are HTTP only; declare another action or remove GRPC()", filename, modelName))
	}
	if grpcOnly && !grpc {
		errs = append(errs, fmt.Errorf("%s: %s declares a Stream action but no GRPC(); a stream is served over gRPC alone, declare GRPC() or remove the Stream action", filename, modelName))
	}
	return records, errs
}

// functionLiteralArg returns the function literal a block keyword takes as
// its argument at index i, or nil when the call passes anything else. The
// keywords are declared to take a func(), so Create(nil) and Create(block)
// compile, but only a literal is a block the parser can read; the
// validators reject the rest rather than let the call silently declare
// nothing.
func functionLiteralArg(call *ast.CallExpr, i int) *ast.FuncLit {
	if len(call.Args) <= i {
		return nil
	}
	flit, _ := call.Args[i].(*ast.FuncLit)
	return flit
}

// validateRouteCall validates one Route block and reports, beside the service
// records and errors of its actions, whether any of them is an action gRPC
// can serve (see httpOnlyActionMethodNames) and whether any is one only
// gRPC can serve (see grpcOnlyActionMethodNames). A parameter of the route
// is written :name, the one form the router reads; one written {name},
// which the router would serve as that literal segment, is refused with
// "the Route("archive/boxes/{box}/documents") of Record writes the
// parameter box as {box}; write :box, the form the router reads".
func validateRouteCall(call *ast.CallExpr, modelName string, rootModelFile, virtual bool, filename string) (records []serviceActionRecord, grpcServable, grpcOnly bool, errs []error) {
	flit := functionLiteralArg(call, 1)
	if flit == nil {
		return nil, false, false, []error{fmt.Errorf("%s: Route takes a function literal, Route(\"path\", func() {...}); a call passing anything else, nil included, declares no route: delete it or write the block", filename)}
	}

	route := stringArgValue(call, "")
	records = make([]serviceActionRecord, 0)
	errs = make([]error, 0)
	for part := range strings.SplitSeq(route, "/") {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			name := strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
			errs = append(errs, fmt.Errorf("%s: the Route(%q) of %s writes the parameter %s as {%[4]s}; write :%[4]s, the form the router reads", filename, route, modelName, name))
		}
	}
	// An action is declared once per route and form: an Exact() action
	// serves the route path itself, the other the item under it, so a
	// Delete of each is two endpoints, while two of one form would register
	// the same route twice.
	type form struct {
		name  string
		exact bool
	}
	seenForms := make(map[form]bool)
	for _, stmt := range flit.Body.List {
		child, name, ok := keywordCall(stmt)
		if !ok {
			errs = append(errs, fmt.Errorf("%s: the Route(%q) block of %s reads DSL keywords alone; delete %s", filename, route, modelName, nodeSource(stmt)))
			continue
		}

		switch {
		case isActionMethod(name):
			grpcServable = grpcServable || !httpOnlyActionMethodNames[name]
			grpcOnly = grpcOnly || grpcOnlyActionMethodNames[name]
			info, actionErrs := validateActionCall(child, name, rootModelFile, virtual, filename)
			if seenForms[form{name, info.exact}] {
				errs = append(errs, fmt.Errorf("%s: the Route(%q) block of %s declares %s twice; declare an action once per route, an Exact() one and one without at most", filename, route, modelName, name))
			}
			seenForms[form{name, info.exact}] = true
			if record, ok := newServiceActionRecord(info, name, modelName, route); ok {
				records = append(records, record)
			}
			errs = append(errs, actionErrs...)
		case name == "Route":
			errs = append(errs, fmt.Errorf("%s: Route() can only be used at Design() top level", filename))
		case actionOnlyMethodNames[name]:
			errs = append(errs, fmt.Errorf("%s: %s() can only be used inside an action block", filename, name))
		case designOnlyMethodNames[name]:
			errs = append(errs, fmt.Errorf("%s: %s() can only be used at Design() top level", filename, name))
		}
	}
	return records, grpcServable, grpcOnly, errs
}

// actionCallInfo carries the generation-relevant keywords collected from one
// action block, so callers can derive facts such as the service filename
// without re-walking the block.
type actionCallInfo struct {
	service          bool
	serviceName      string
	flatten          bool
	exact            bool
	payload          bool
	result           bool
	streamingPayload bool
	streamingResult  bool
}

func validateActionCall(call *ast.CallExpr, actionName string, rootModelFile, virtual bool, filename string) (actionCallInfo, []error) {
	info := actionCallInfo{}
	flit := functionLiteralArg(call, 0)
	if flit == nil {
		return info, []error{fmt.Errorf("%s: %s takes a function literal, %s(func() {...}); a call passing anything else, nil included, declares no action: delete it or write the block", filename, actionName, actionName)}
	}

	errs := make([]error, 0)
	for _, stmt := range flit.Body.List {
		child, name, ok := keywordCall(stmt)
		if !ok {
			errs = append(errs, fmt.Errorf("%s: the %s block reads DSL keywords alone; delete %s", filename, actionName, nodeSource(stmt)))
			continue
		}

		switch {
		case name == "Service":
			info.service = true
			errs = append(errs, validateServiceName(child, actionName, filename, &info)...)
		case name == "Flatten":
			info.flatten = true
		case name == "Exact":
			info.exact = true
		case PayloadKeyword(name), ResultKeyword(name):
			switch {
			case StreamingKeyword(name) && PayloadKeyword(name):
				info.streamingPayload = true
			case StreamingKeyword(name):
				info.streamingResult = true
			case PayloadKeyword(name):
				info.payload = true
			default:
				info.result = true
			}
			if arg, ok := actionTypeArgument(child); ok && !modelPackageType(arg) {
				errs = append(errs, fmt.Errorf("%s: %s action declares %s[%s]; Payload, Result, StreamingPayload and StreamingResult name a type of the model package, T or *T", filename, actionName, name, nodeSource(arg)))
			}
		case name == "Public":
			continue
		case isActionMethod(name):
			errs = append(errs, fmt.Errorf("%s: %s action cannot contain nested %s action", filename, actionName, name))
		case name == "Route":
			errs = append(errs, fmt.Errorf("%s: Route() can only be used at Design() top level", filename))
		case designOnlyMethodNames[name]:
			errs = append(errs, fmt.Errorf("%s: %s() can only be used at Design() top level", filename, name))
		}
	}

	if info.flatten {
		if !info.service {
			errs = append(errs, fmt.Errorf("%s: %s action uses dsl.Flatten() but does not enable Service()", filename, actionName))
		}
		if info.serviceName == "" {
			errs = append(errs, fmt.Errorf("%s: %s action uses dsl.Flatten() but names no service; write Service(\"name\")", filename, actionName))
		}
		if rootModelFile {
			errs = append(errs, fmt.Errorf("%s: dsl.Flatten() cannot be used by root model file %s; move the model under model/<package>/<file>.go or remove Flatten()", filename, filename))
		}
	}
	if info.payload && getVerbActionMethodNames[actionName] {
		errs = append(errs, fmt.Errorf("%s: %s action handles an HTTP GET request and cannot declare Payload; declare Result for a custom service method and read query parameters from ServiceContext.Query()", filename, actionName))
	}
	if sig, ok := fixedContractActionSignatures[actionName]; ok {
		if info.payload {
			errs = append(errs, fmt.Errorf("%s: %s action delegates to the fixed service method %s and cannot declare Payload", filename, actionName, sig))
		}
		if info.result {
			errs = append(errs, fmt.Errorf("%s: %s action delegates to the fixed service method %s and cannot declare Result", filename, actionName, sig))
		}
	}
	if info.exact && routeIDActionMethodNames[actionName] {
		if getVerbActionMethodNames[actionName] {
			if !info.result {
				errs = append(errs, fmt.Errorf("%s: %s action uses dsl.Exact() but relies on the built-in controller which reads the resource id from the route parameter only; declare Result with a custom service method or remove Exact()", filename, actionName))
			}
		} else if !info.payload && !info.result {
			errs = append(errs, fmt.Errorf("%s: %s action uses dsl.Exact() but relies on the built-in controller which reads the resource id from the route parameter only; declare Payload/Result with a custom service method or remove Exact()", filename, actionName))
		}
	}
	// A List without a custom Result runs the built-in list controller, which
	// selects from the model table; a virtual model has none, so the request
	// could only fail at runtime. The Export action is exempt: its controller
	// skips the table phases for virtual models and delegates to the service.
	if virtual && actionName == consts.List.Name() && !info.result {
		errs = append(errs, fmt.Errorf("%s: %s action on a virtual model relies on the built-in list controller, but a virtual model has no table to list from; declare Result with a custom service method", filename, actionName))
	}
	// These actions have no built-in implementation able to answer a request
	// on its own: SSE's stream, Import's parsing, and Export's rendering all
	// live in the custom service, so a block without Service() registers a
	// route that can only answer "not implemented".
	if serviceRequiredActionMethodNames[actionName] && !info.service {
		errs = append(errs, fmt.Errorf("%s: %s action has no built-in implementation and must declare Service()", filename, actionName))
	}
	// A Stream streams one side of the call or both, and each side is
	// either one message or a stream of them; its rpc is named after the
	// name Service gives it, there being no default role name for it, and
	// it has no HTTP route for Exact to shape. No other action streams.
	if !grpcOnlyActionMethodNames[actionName] {
		if info.streamingPayload {
			errs = append(errs, fmt.Errorf("%s: %s action cannot declare StreamingPayload; only a Stream action streams", filename, actionName))
		}
		if info.streamingResult {
			errs = append(errs, fmt.Errorf("%s: %s action cannot declare StreamingResult; only a Stream action streams", filename, actionName))
		}
		return info, errs
	}
	if !info.streamingPayload && !info.streamingResult {
		errs = append(errs, fmt.Errorf("%s: %s action must declare StreamingPayload or StreamingResult; a call streaming neither side is a plain action, declare it with Create", filename, actionName))
	}
	if info.payload && info.streamingPayload {
		errs = append(errs, fmt.Errorf("%s: %s action declares both Payload and StreamingPayload; the request is either one message or a stream of them", filename, actionName))
	}
	if info.result && info.streamingResult {
		errs = append(errs, fmt.Errorf("%s: %s action declares both Result and StreamingResult; the response is either one message or a stream of them", filename, actionName))
	}
	if info.serviceName == "" {
		errs = append(errs, fmt.Errorf("%s: %s action must name its service, Service(\"name\"), which names its rpc", filename, actionName))
	}
	if info.exact {
		errs = append(errs, fmt.Errorf("%s: %s action has no HTTP route for dsl.Exact() to shape; remove Exact()", filename, actionName))
	}

	return info, errs
}

// serviceActionRecord captures one Service-enabled action and the service
// file it generates, for collision checks across the actions of a model file.
type serviceActionRecord struct {
	model    string // model struct name declaring the Design
	route    string // Route path owning the action; empty for Design top-level actions
	action   string // action method name, e.g. "Get"
	flatten  bool   // Flatten writes into the package service dir instead of the model file dir
	filename string // generated service filename, e.g. "list.go"
	role     string // generated service type name, e.g. "Lister"
}

// newServiceActionRecord builds the generation record of one action call. It
// reports false when the action does not generate a service file.
func newServiceActionRecord(info actionCallInfo, actionName, modelName, route string) (serviceActionRecord, bool) {
	if !info.service {
		return serviceActionRecord{}, false
	}
	action := Action{ServiceName: info.serviceName, Phase: actionMethodPhases[actionName]}
	return serviceActionRecord{
		model:    modelName,
		route:    route,
		action:   actionName,
		flatten:  info.flatten,
		filename: action.ServiceFilename(),
		role:     action.RoleName(),
	}, true
}

// validateServiceCollisions rejects two Service actions generating the same
// service file, or the same service type in one service directory. gg gen
// derives both from the service name, so colliding actions fight over one
// struct: the first action creates it and every later action force-syncs
// the service.Base type parameters to its own Payload/Result, leaving a
// hybrid declaration that satisfies neither service registration, or, in
// two files, a type declared twice that does not compile. Two files collide
// when their names lower-case alike, Service("Archive") and
// Service("archive"); two types when their names camel-case alike,
// Service("item_archive") and Service("itemArchive"), or Service() on Create
// and Service("creator") on another action, whose files differ. All models
// in one file share one service dir, so records are grouped per file;
// Flatten actions write into the package service dir instead and therefore
// only collide with other Flatten actions.
func validateServiceCollisions(records []serviceActionRecord, filename string) []error {
	type key struct {
		flatten bool
		name    string
	}
	describe := func(group []serviceActionRecord) string {
		descs := make([]string, 0, len(group))
		for _, record := range group {
			desc := fmt.Sprintf("%s on %s", record.action, record.model)
			if record.route != "" {
				desc = fmt.Sprintf("%s (route %q)", desc, record.route)
			}
			descs = append(descs, desc)
		}
		return strings.Join(descs, ", ")
	}
	collisions := func(keyOf func(serviceActionRecord) key) ([]key, map[key][]serviceActionRecord) {
		groups := make(map[key][]serviceActionRecord)
		keys := make([]key, 0)
		for _, record := range records {
			k := keyOf(record)
			if _, ok := groups[k]; !ok {
				keys = append(keys, k)
			}
			groups[k] = append(groups[k], record)
		}
		return keys, groups
	}

	errs := make([]error, 0)
	keys, files := collisions(func(r serviceActionRecord) key { return key{flatten: r.flatten, name: r.filename} })
	for _, k := range keys {
		if group := files[k]; len(group) > 1 {
			errs = append(errs, fmt.Errorf("%s: service file %q is generated by multiple actions: %s; give each Service action a distinct name, Service(\"name\")", filename, k.name, describe(group)))
		}
	}
	keys, roles := collisions(func(r serviceActionRecord) key { return key{flatten: r.flatten, name: r.role} })
	for _, k := range keys {
		group := roles[k]
		if len(group) < 2 {
			continue
		}
		// One file: the file collision above reports it.
		oneFile := true
		for _, r := range group[1:] {
			if r.filename != group[0].filename {
				oneFile = false
				break
			}
		}
		if oneFile {
			continue
		}
		errs = append(errs, fmt.Errorf("%s: service type %q is generated by multiple actions: %s; give each Service action a distinct name, Service(\"name\")", filename, k.name, describe(group)))
	}
	return errs
}

// keywordCall returns the call a statement makes and the DSL keyword it
// names; ok is false for any other statement, a call of something that is
// not a keyword included. A Design() and the Route and action blocks in it
// hold keyword calls alone: nothing runs them, so any other statement is
// dead code that reads like a declaration and is reported instead.
func keywordCall(stmt ast.Stmt) (call *ast.CallExpr, name string, ok bool) {
	expr, isExpr := stmt.(*ast.ExprStmt)
	if !isExpr || expr == nil {
		return nil, "", false
	}
	call, isCall := expr.X.(*ast.CallExpr)
	if !isCall || call == nil || call.Fun == nil {
		return nil, "", false
	}
	name, ok = funcName(call.Fun)
	return call, name, ok && is(name)
}

// nodeSource spells a node the way the source does, on one line, for a
// report that names what it found: println("x") or _ = 1 for a statement,
// *shared.Event for a type.
func nodeSource(node ast.Node) string {
	var buf strings.Builder
	if err := printer.Fprint(&buf, token.NewFileSet(), node); err != nil {
		return "the statement"
	}
	return strings.Join(strings.Fields(buf.String()), " ")
}

// funcName returns the name a call names: the identifier, the selector's
// name for dsl.Create, or the name under the index for an instantiated
// generic such as Payload[*T].
func funcName(expr ast.Expr) (string, bool) {
	switch fun := expr.(type) {
	case *ast.Ident:
		if fun == nil || fun.Name == "" {
			return "", false
		}
		return fun.Name, true
	case *ast.SelectorExpr:
		if fun == nil || fun.Sel == nil || fun.Sel.Name == "" {
			return "", false
		}
		return fun.Sel.Name, true
	case *ast.IndexExpr:
		return funcName(fun.X)
	case *ast.IndexListExpr:
		return funcName(fun.X)
	default:
		return "", false
	}
}

// validateServiceName reads the name a Service call gives the action into
// info and reports a call the generator cannot take a name from: more than
// one argument, an argument that is not a string literal, an empty name,
// pointed at Service() for the service named after the action, or a name
// serviceNameRefusal refuses; for a name written like a file whose base name
// it takes, archive.go or sample/archive.go, the report suggests that base
// name, archive.
func validateServiceName(call *ast.CallExpr, actionName, filename string, info *actionCallInfo) []error {
	switch {
	case len(call.Args) == 0:
		return nil
	case len(call.Args) > 1:
		return []error{fmt.Errorf("%s: %s action calls Service with %d arguments; Service takes one at most, the name of the service", filename, actionName, len(call.Args))}
	}
	value, ok := stringLiteral(call.Args[0])
	if !ok {
		return []error{fmt.Errorf("%s: %s action names its service with something other than a string literal; write Service(\"name\")", filename, actionName)}
	}
	if value == "" {
		return []error{fmt.Errorf("%s: %s action names its service \"\"; write Service() to name the service after the action, or Service(\"name\") to name it", filename, actionName)}
	}
	if refusal := serviceNameRefusal(value); refusal != "" {
		if base := strings.TrimSuffix(filepath.Base(value), filepath.Ext(value)); serviceNameRefusal(base) == "" {
			refusal += fmt.Sprintf(": Service(%q)", base)
		}
		return []error{fmt.Errorf("%s: %s action names its service %q; %s", filename, actionName, value, refusal)}
	}
	info.serviceName = value
	return nil
}

// serviceNameRefusal returns why Service refuses name, or "" when it takes
// it: a name outside serviceNamePattern, or one the go command reads as the
// suffix of the file it names, _test making that a test file and _windows or
// _amd64 building it for that platform alone.
func serviceNameRefusal(name string) string {
	if !serviceNamePattern.MatchString(name) {
		return "a service name is letters, digits and underscores, starting with a letter, naming the service file, its type and its rpc"
	}
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, "_test") {
		return "a name ending in _test names a test file, choose another name"
	}
	if i := strings.LastIndex(lower, "_"); i >= 0 {
		if suffix := lower[i+1:]; knownOS[suffix] || knownArch[suffix] {
			return fmt.Sprintf("a name ending in _%s names a file built for that platform alone, choose another name", suffix)
		}
	}
	return ""
}

// The operating systems and architectures the go command reads as the
// suffix of a file name, _windows.go or _amd64.go, which then builds for
// that platform alone; the go command keeps the lists in an internal
// package, so serviceNameRefusal repeats them.
var (
	knownOS = map[string]bool{
		"aix": true, "android": true, "darwin": true, "dragonfly": true, "freebsd": true, "hurd": true, "illumos": true, "ios": true, "js": true,
		"linux": true, "nacl": true, "netbsd": true, "openbsd": true, "plan9": true, "solaris": true, "wasip1": true, "windows": true, "zos": true,
	}
	knownArch = map[string]bool{
		"386": true, "amd64": true, "amd64p32": true, "arm": true, "armbe": true, "arm64": true, "arm64be": true, "loong64": true,
		"mips": true, "mipsle": true, "mips64": true, "mips64le": true, "mips64p32": true, "mips64p32le": true, "ppc": true, "ppc64": true, "ppc64le": true,
		"riscv": true, "riscv64": true, "s390": true, "s390x": true, "sparc": true, "sparc64": true, "wasm": true,
	}
)

// actionTypeArgument returns the type argument of an instantiated action
// type keyword, Payload[*T]() or Result[T](), and false for a call carrying
// none.
func actionTypeArgument(call *ast.CallExpr) (ast.Expr, bool) {
	switch fun := call.Fun.(type) {
	case *ast.IndexExpr:
		return fun.Index, true
	case *ast.IndexListExpr:
		if len(fun.Indices) == 1 {
			return fun.Indices[0], true
		}
	}
	return nil, false
}

// modelPackageType reports whether expr names a type of the model package,
// T or *T: the parser reads those alone (see parse), so any other form, a
// type of another package or a slice, would declare nothing.
func modelPackageType(expr ast.Expr) bool {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	_, ok := expr.(*ast.Ident)
	return ok
}

func stringArgValue(call *ast.CallExpr, current string) string {
	if len(call.Args) == 0 {
		return current
	}
	if value, ok := stringLiteral(call.Args[0]); ok {
		return value
	}
	return current
}

func isRootModelFile(file *ast.File, modelDir string, filename string) bool {
	if file == nil || file.Name == nil || file.Name.Name != "model" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(modelDir), filepath.Clean(filename))
	if err != nil {
		return false
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return false
	}
	return filepath.Dir(rel) == "."
}
