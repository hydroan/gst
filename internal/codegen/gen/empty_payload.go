package gen

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/constants"
)

const (
	// gstModelPkgName is the package name of the gst model package.
	gstModelPkgName = "model"
	// gstModelPkgAlias is the import alias used when the plain "model"
	// qualifier would clash with another import in the generated file.
	gstModelPkgAlias = "gstmodel"
)

// RouterGstModelUse resolves how the generated router file references the
// gst model package. pkgName is the qualifier emitted for model.Empty: the
// plain package name by default, falling back to the gstmodel alias when a
// routed business model package is itself named "model" (two imports cannot
// take the same name in one file, so the plain qualifier would clash with
// that import). needed reports whether any routed action resolves either
// side to dsl.PayloadEmpty, i.e. whether the qualifier appears in the file at
// all. For example, a routed root model package with a List declaring only
// its result yields ("gstmodel", true), and routed models in package sample
// declaring both sides of every action yield ("model", false). Call it after
// the route/model ignore passes so disabled actions no longer count as routed.
func RouterGstModelUse(models []*ModelInfo) (pkgName string, needed bool) {
	pkgName = gstModelPkgName
	for _, m := range models {
		if m.Design == nil {
			continue
		}
		routed := false
		m.Design.Range(func(_ string, act *dsl.Action) {
			routed = true
			if isEmptyPayload(act.Payload) || isEmptyPayload(act.Result) {
				needed = true
			}
		})
		if routed && m.ModelPkgName == gstModelPkgName {
			pkgName = gstModelPkgAlias
		}
	}
	return pkgName, needed
}

// GstModelImportEntry returns the import entry ("path" or "alias path") that
// makes the given gst model package qualifier resolvable in a generated file:
// a service file takes it through imports(), the router file through
// BuildRouterFile. It returns "gstmodel github.com/hydroan/gst/model" for
// gstmodel and "github.com/hydroan/gst/model" for model.
func GstModelImportEntry(pkgName string) string {
	if pkgName == gstModelPkgAlias {
		return gstModelPkgAlias + " " + constants.ImportPathModel
	}
	return constants.ImportPathModel
}

// emptyReqPkgName returns the package qualifier a generated service file uses
// to reference model.Empty. When the file refers to the business model
// package as "model" (modelQualifier), as with the root model package, the gst
// model package is imported under the gstmodel alias to avoid the name clash:
// it returns gstmodel for model and model for sample.
func emptyReqPkgName(modelQualifier string) string {
	if modelQualifier == gstModelPkgName {
		return gstModelPkgAlias
	}
	return gstModelPkgName
}

// emptyReqImport returns the imports() entry ("path" or "alias path") that
// makes the emptyReqPkgName qualifier resolvable in a generated service file:
// "gstmodel github.com/hydroan/gst/model" for model, and
// "github.com/hydroan/gst/model" for any other modelQualifier.
func emptyReqImport(modelQualifier string) string {
	return GstModelImportEntry(emptyReqPkgName(modelQualifier))
}

// emptyReqExpr builds the *<pkgName>.Empty type expression that generated
// code uses as the type of a dsl.PayloadEmpty request or result: for
// gstmodel it builds *gstmodel.Empty.
func emptyReqExpr(pkgName string) ast.Expr {
	return &ast.StarExpr{
		X: &ast.SelectorExpr{
			X:   ast.NewIdent(pkgName),
			Sel: ast.NewIdent("Empty"),
		},
	}
}

// isEmptyPayload reports whether the action type name, of a request or a
// result, is the dsl.PayloadEmpty sentinel.
func isEmptyPayload(typeName string) bool { return typeName == dsl.PayloadEmpty }

// payloadTypeTarget resolves an action type name, of a request or a result,
// to the package qualifier and type name used when rewriting existing service
// code. modelPkg is the business model package qualifier of the file being
// rewritten. With modelPkg sample it returns ("model", "*Empty") for
// dsl.PayloadEmpty and ("sample", "*RecordReq") for *RecordReq.
func payloadTypeTarget(payload, modelPkg string) (targetPkg, actionType string) {
	if isEmptyPayload(payload) {
		return emptyReqPkgName(modelPkg), "*Empty"
	}
	return modelPkg, payload
}

// ensureEmptyReqImportSpec inserts the gst model import into a parsed service
// file so a rewritten *model.Empty request or result type resolves: when the
// file refers to the business model package as model (modelPkg), it inserts
//
//	gstmodel "github.com/hydroan/gst/model"
//
// and "github.com/hydroan/gst/model" otherwise. It reports whether the file
// was modified.
func ensureEmptyReqImportSpec(file *ast.File, modelPkg string) bool {
	if file == nil || findImportSpec(file, constants.ImportPathModel) != nil {
		return false
	}

	spec := &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: fmt.Sprintf("%q", constants.ImportPathModel),
		},
	}
	if emptyReqPkgName(modelPkg) == gstModelPkgAlias {
		spec.Name = ast.NewIdent(gstModelPkgAlias)
	}
	insertImportSpec(file, spec)
	return true
}

// pruneGstModelImportSpec removes the gst model import when the file no
// longer references its qualifier, so switching a request or result type back
// to a business type does not leave an unused import behind. Hand-written code
// that still references the package keeps the import. It reports whether the
// file was modified.
func pruneGstModelImportSpec(file *ast.File) bool {
	spec := findImportSpec(file, constants.ImportPathModel)
	if spec == nil {
		return false
	}

	localName := gstModelPkgName
	if spec.Name != nil {
		localName = spec.Name.Name
	}

	referenced := false
	ast.Inspect(file, func(node ast.Node) bool {
		if referenced {
			return false
		}
		if sel, ok := node.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == localName {
				referenced = true
				return false
			}
		}
		return true
	})
	if referenced {
		return false
	}

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.IMPORT {
			continue
		}
		for i, s := range genDecl.Specs {
			if s == spec {
				genDecl.Specs = append(genDecl.Specs[:i], genDecl.Specs[i+1:]...)
				return true
			}
		}
	}
	return false
}

// findImportSpec returns the import spec for the given import path, or nil.
// It looks through file.Imports, the imports the file was parsed with, so it
// misses an import inserted into the AST since.
func findImportSpec(file *ast.File, importPath string) *ast.ImportSpec {
	for _, imp := range file.Imports {
		if imp.Path != nil && strings.Trim(imp.Path.Value, `"`) == importPath {
			return imp
		}
	}
	return nil
}
