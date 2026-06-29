package dsl

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/hydroan/gst/types/consts"
)

var actionMethodNames = map[string]bool{
	consts.PHASE_CREATE.MethodName():      true,
	consts.PHASE_DELETE.MethodName():      true,
	consts.PHASE_UPDATE.MethodName():      true,
	consts.PHASE_PATCH.MethodName():       true,
	consts.PHASE_LIST.MethodName():        true,
	consts.PHASE_GET.MethodName():         true,
	consts.PHASE_CREATE_MANY.MethodName(): true,
	consts.PHASE_DELETE_MANY.MethodName(): true,
	consts.PHASE_UPDATE_MANY.MethodName(): true,
	consts.PHASE_PATCH_MANY.MethodName():  true,
	consts.PHASE_IMPORT.MethodName():      true,
	consts.PHASE_EXPORT.MethodName():      true,
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
	rootModelFile := isRootModelFile(file, modelDir, filename)
	for _, fn := range designBase {
		errs = append(errs, validateDesignFunc(fn, rootModelFile, filename)...)
	}
	for _, fn := range designEmpty {
		errs = append(errs, validateDesignFunc(fn, rootModelFile, filename)...)
	}
	return errs
}

func validateDesignFunc(fn *ast.FuncDecl, rootModelFile bool, filename string) []error {
	if fn == nil || fn.Body == nil {
		return nil
	}

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
		case actionMethodNames[name]:
			errs = append(errs, validateActionCall(call, name, rootModelFile, filename)...)
		case name == "Route":
			errs = append(errs, validateRouteCall(call, rootModelFile, filename)...)
		case name == "Enabled" || designOnlyMethodNames[name]:
			continue
		case actionOnlyMethodNames[name]:
			errs = append(errs, fmt.Errorf("%s: %s() can only be used inside an action block", filename, name))
		}
	}
	return errs
}

func validateRouteCall(call *ast.CallExpr, rootModelFile bool, filename string) []error {
	if len(call.Args) < 2 {
		return nil
	}
	flit, ok := call.Args[1].(*ast.FuncLit)
	if !ok || flit == nil || flit.Body == nil {
		return nil
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
		case actionMethodNames[name]:
			errs = append(errs, validateActionCall(child, name, rootModelFile, filename)...)
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
	return errs
}

func validateActionCall(call *ast.CallExpr, actionName string, rootModelFile bool, filename string) []error {
	if len(call.Args) == 0 {
		return nil
	}
	flit, ok := call.Args[0].(*ast.FuncLit)
	if !ok || flit == nil || flit.Body == nil {
		return nil
	}

	service := false
	filenameValue := ""
	flatten := false
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
			service = boolArgValue(child, service)
		case name == "Filename":
			filenameValue = stringArgValue(child, filenameValue)
		case name == "Flatten":
			flatten = true
		case name == "Enabled" || name == "Public" || name == "Exact" || name == "Payload" || name == "Result":
			continue
		case actionMethodNames[name]:
			errs = append(errs, fmt.Errorf("%s: %s action cannot contain nested %s action", filename, actionName, name))
		case name == "Route":
			errs = append(errs, fmt.Errorf("%s: Route() can only be used at Design() top level", filename))
		case designOnlyMethodNames[name]:
			errs = append(errs, fmt.Errorf("%s: %s() can only be used at Design() top level", filename, name))
		}
	}

	if flatten {
		if !service {
			errs = append(errs, fmt.Errorf("%s: %s action uses dsl.Flatten() but does not enable Service(true)", filename, actionName))
		}
		if filenameValue == "" {
			errs = append(errs, fmt.Errorf("%s: %s action uses dsl.Flatten() but is missing Filename(...)", filename, actionName))
		}
		if rootModelFile {
			errs = append(errs, fmt.Errorf("%s: dsl.Flatten() cannot be used by root model file %s; move the model under model/<package>/<file>.go or remove Flatten()", filename, filename))
		}
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

func boolArgValue(call *ast.CallExpr, current bool) bool {
	if len(call.Args) == 0 {
		return current
	}
	ident, ok := call.Args[0].(*ast.Ident)
	if !ok || ident == nil {
		return current
	}
	return ident.Name == "true"
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
