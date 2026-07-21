package dsl

import (
	"fmt"
	"go/ast"
	"go/token"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hydroan/gst/types/consts"
)

// actionMethodPhases maps DSL action method names to their phases. It serves
// both as the action keyword set and as the phase lookup for generation
// facts derived from an action call, such as the service filename.
var actionMethodPhases = map[string]consts.Phase{
	consts.PHASE_CREATE.MethodName():      consts.PHASE_CREATE,
	consts.PHASE_DELETE.MethodName():      consts.PHASE_DELETE,
	consts.PHASE_UPDATE.MethodName():      consts.PHASE_UPDATE,
	consts.PHASE_PATCH.MethodName():       consts.PHASE_PATCH,
	consts.PHASE_LIST.MethodName():        consts.PHASE_LIST,
	consts.PHASE_GET.MethodName():         consts.PHASE_GET,
	consts.PHASE_CREATE_MANY.MethodName(): consts.PHASE_CREATE_MANY,
	consts.PHASE_DELETE_MANY.MethodName(): consts.PHASE_DELETE_MANY,
	consts.PHASE_UPDATE_MANY.MethodName(): consts.PHASE_UPDATE_MANY,
	consts.PHASE_PATCH_MANY.MethodName():  consts.PHASE_PATCH_MANY,
	consts.PHASE_IMPORT.MethodName():      consts.PHASE_IMPORT,
	consts.PHASE_EXPORT.MethodName():      consts.PHASE_EXPORT,
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
	consts.PHASE_DELETE.MethodName(): true,
	consts.PHASE_UPDATE.MethodName(): true,
	consts.PHASE_PATCH.MethodName():  true,
	consts.PHASE_GET.MethodName():    true,
}

// getVerbActionMethodNames are actions whose generated routes handle HTTP GET
// requests. A GET request carries no request body, so these actions must not
// declare Payload; custom services read filters from ServiceContext.Query().
var getVerbActionMethodNames = map[string]bool{
	consts.PHASE_LIST.MethodName(): true,
	consts.PHASE_GET.MethodName():  true,
}

// fixedContractActionSignatures are actions whose service methods have fixed
// signatures that never bind Payload or Result types, mapped to those
// signatures for error reporting. The Import controller reads the uploaded
// multipart form file and responds with a bare status code; the Export
// controller handles an HTTP GET request, reads filters from query
// parameters, and writes the returned bytes as a file attachment.
var fixedContractActionSignatures = map[string]string{
	consts.PHASE_IMPORT.MethodName(): "Import(ctx, io.Reader) ([]M, error)",
	consts.PHASE_EXPORT.MethodName(): "Export(ctx, ...M) ([]byte, error)",
}

var designOnlyMethodNames = map[string]bool{
	"Endpoint": true,
	"Param":    true,
	"Migrate":  true,
}

var actionOnlyMethodNames = map[string]bool{
	"Service":  true,
	"Public":   true,
	"Exact":    true,
	"Filename": true,
	"Payload":  true,
	"Result":   true,
	"Flatten":  true,
}

// Validate checks DSL keyword placement and generation semantics for one model file.
// It intentionally validates only model Design() methods for structs embedding
// model.Base or model.Empty, matching Parse's model discovery scope.
// Do not duplicate Go compiler or type-checker diagnostics here, such as wrong
// argument counts or incompatible argument types. Keep this validator focused on
// DSL structure, keyword placement, and generation-specific semantics.
func Validate(file *ast.File, modelDir string, filename string) []error {
	designBase, designEmpty := parse(file)
	for name := range designBase {
		delete(designEmpty, name)
	}

	errs := make([]error, 0)
	records := make([]serviceActionRecord, 0)
	rootModelFile := isRootModelFile(file, modelDir, filename)
	for _, name := range slices.Sorted(maps.Keys(designBase)) {
		recs, designErrs := validateDesignFunc(designBase[name], name, rootModelFile, filename)
		records = append(records, recs...)
		errs = append(errs, designErrs...)
	}
	for _, name := range slices.Sorted(maps.Keys(designEmpty)) {
		recs, designErrs := validateDesignFunc(designEmpty[name], name, rootModelFile, filename)
		records = append(records, recs...)
		errs = append(errs, designErrs...)
	}
	errs = append(errs, validateServiceFilenameCollisions(records, filename)...)
	return errs
}

func validateDesignFunc(fn *ast.FuncDecl, modelName string, rootModelFile bool, filename string) ([]serviceActionRecord, []error) {
	if fn == nil || fn.Body == nil {
		return nil, nil
	}

	records := make([]serviceActionRecord, 0)
	errs := make([]error, 0)
	for _, stmt := range fn.Body.List {
		call := exprStmtCall(stmt)
		if call == nil {
			continue
		}
		name, ok := callName(call)
		if !ok || !is(name) {
			continue
		}

		switch {
		case isActionMethod(name):
			info, actionErrs := validateActionCall(call, name, rootModelFile, filename)
			if record, ok := newServiceActionRecord(info, name, modelName, ""); ok {
				records = append(records, record)
			}
			errs = append(errs, actionErrs...)
		case name == "Route":
			recs, routeErrs := validateRouteCall(call, modelName, rootModelFile, filename)
			records = append(records, recs...)
			errs = append(errs, routeErrs...)
		case name == "Enabled" || designOnlyMethodNames[name]:
			continue
		case actionOnlyMethodNames[name]:
			errs = append(errs, fmt.Errorf("%s: %s() can only be used inside an action block", filename, name))
		}
	}
	return records, errs
}

func validateRouteCall(call *ast.CallExpr, modelName string, rootModelFile bool, filename string) ([]serviceActionRecord, []error) {
	if len(call.Args) < 2 {
		return nil, nil
	}
	flit, ok := call.Args[1].(*ast.FuncLit)
	if !ok || flit == nil || flit.Body == nil {
		return nil, nil
	}

	route := stringArgValue(call, "")
	records := make([]serviceActionRecord, 0)
	errs := make([]error, 0)
	for _, stmt := range flit.Body.List {
		child := exprStmtCall(stmt)
		if child == nil {
			continue
		}
		name, ok := callName(child)
		if !ok || !is(name) {
			continue
		}

		switch {
		case isActionMethod(name):
			info, actionErrs := validateActionCall(child, name, rootModelFile, filename)
			if record, ok := newServiceActionRecord(info, name, modelName, route); ok {
				records = append(records, record)
			}
			errs = append(errs, actionErrs...)
		case name == "Route":
			errs = append(errs, fmt.Errorf("%s: Route() can only be used at Design() top level", filename))
		case actionOnlyMethodNames[name]:
			errs = append(errs, fmt.Errorf("%s: %s() can only be used inside an action block", filename, name))
		case name == "Enabled":
			errs = append(errs, fmt.Errorf("%s: Enabled() can only be used at Design() top level or inside an action block", filename))
		case designOnlyMethodNames[name]:
			errs = append(errs, fmt.Errorf("%s: %s() can only be used at Design() top level", filename, name))
		}
	}
	return records, errs
}

// actionCallInfo carries the generation-relevant keywords collected from one
// action block, so callers can derive facts such as the service filename
// without re-walking the block.
type actionCallInfo struct {
	service  bool
	filename string
	flatten  bool
	exact    bool
	payload  bool
	result   bool
}

func validateActionCall(call *ast.CallExpr, actionName string, rootModelFile bool, filename string) (actionCallInfo, []error) {
	info := actionCallInfo{}
	if len(call.Args) == 0 {
		return info, nil
	}
	flit, ok := call.Args[0].(*ast.FuncLit)
	if !ok || flit == nil || flit.Body == nil {
		return info, nil
	}

	errs := make([]error, 0)
	for _, stmt := range flit.Body.List {
		child := exprStmtCall(stmt)
		if child == nil {
			continue
		}
		name, ok := callName(child)
		if !ok || !is(name) {
			continue
		}

		switch {
		case name == "Service":
			info.service = true
		case name == "Filename":
			info.filename = stringArgValue(child, info.filename)
		case name == "Flatten":
			info.flatten = true
		case name == "Exact":
			info.exact = true
		case name == "Payload":
			info.payload = true
		case name == "Result":
			info.result = true
		case name == "Enabled" || name == "Public":
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
		if info.filename == "" {
			errs = append(errs, fmt.Errorf("%s: %s action uses dsl.Flatten() but is missing Filename(...)", filename, actionName))
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
}

// newServiceActionRecord builds the generation record of one action call. It
// reports false when the action does not generate a service file.
func newServiceActionRecord(info actionCallInfo, actionName, modelName, route string) (serviceActionRecord, bool) {
	if !info.service {
		return serviceActionRecord{}, false
	}
	action := Action{Filename: info.filename, Phase: actionMethodPhases[actionName]}
	return serviceActionRecord{
		model:    modelName,
		route:    route,
		action:   actionName,
		flatten:  info.flatten,
		filename: action.ServiceFilename(),
	}, true
}

// validateServiceFilenameCollisions rejects two Service actions generating
// the same service file. gg gen derives both the target file and the service
// struct name from Filename, so colliding actions fight over one struct: the
// first action creates it and every later action force-syncs the
// service.Base type parameters to its own Payload/Result, leaving a hybrid
// declaration that satisfies neither service registration. All models in one
// file share one service dir, so records are grouped per file; Flatten
// actions write into the package service dir instead and therefore only
// collide with other Flatten actions.
func validateServiceFilenameCollisions(records []serviceActionRecord, filename string) []error {
	type fileKey struct {
		flatten bool
		name    string
	}
	groups := make(map[fileKey][]serviceActionRecord)
	keys := make([]fileKey, 0)
	for _, record := range records {
		key := fileKey{flatten: record.flatten, name: record.filename}
		if _, ok := groups[key]; !ok {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], record)
	}

	errs := make([]error, 0)
	for _, key := range keys {
		group := groups[key]
		if len(group) < 2 {
			continue
		}
		descs := make([]string, 0, len(group))
		for _, record := range group {
			desc := fmt.Sprintf("%s on %s", record.action, record.model)
			if record.route != "" {
				desc = fmt.Sprintf("%s (route %q)", desc, record.route)
			}
			descs = append(descs, desc)
		}
		errs = append(errs, fmt.Errorf("%s: service file %q is generated by multiple actions: %s; give each Service action a distinct Filename()", filename, key.name, strings.Join(descs, ", ")))
	}
	return errs
}

func exprStmtCall(stmt ast.Stmt) *ast.CallExpr {
	expr, ok := stmt.(*ast.ExprStmt)
	if !ok || expr == nil {
		return nil
	}
	call, ok := expr.X.(*ast.CallExpr)
	if !ok || call == nil || call.Fun == nil {
		return nil
	}
	return call
}

func callName(call *ast.CallExpr) (string, bool) {
	return funcName(call.Fun)
}

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

func stringArgValue(call *ast.CallExpr, current string) string {
	if len(call.Args) == 0 {
		return current
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit == nil || lit.Kind != token.STRING {
		return current
	}
	return trimQuote(lit.Value)
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
