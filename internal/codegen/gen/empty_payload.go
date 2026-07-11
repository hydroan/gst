package gen

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"github.com/hydroan/gst/dsl"
)

// GstModelImportPath is the import path of the gst model package that
// defines model.Empty, the request type generated for List and Get actions
// declaring Result (dsl.PayloadEmpty).
const GstModelImportPath = "github.com/hydroan/gst/model"

const (
	// gstModelPkgName is the package name of the gst model package.
	gstModelPkgName = "model"
	// gstModelPkgAlias is the import alias used when the plain "model"
	// qualifier would clash with another import in the generated file.
	gstModelPkgAlias = "gstmodel"
)

// GstModelRouterImport is the BuildRouterFile modelImports entry that makes
// the gstmodel qualifier emitted by StmtRouterRegister resolvable. The router
// file aggregates imports from many model packages, so the gst model package
// is always imported under the gstmodel alias there.
const GstModelRouterImport = gstModelPkgAlias + " " + GstModelImportPath

// emptyReqPkgName returns the package qualifier a generated service file uses
// to reference model.Empty. When the business model package itself is named
// "model" (root model files), the gst model package is imported under the
// gstmodel alias to avoid the name clash.
func emptyReqPkgName(modelPkgName string) string {
	if modelPkgName == gstModelPkgName {
		return gstModelPkgAlias
	}
	return gstModelPkgName
}

// emptyReqImport returns the imports() entry ("path" or "alias path") that
// makes the emptyReqPkgName qualifier resolvable in a generated service file.
func emptyReqImport(modelPkgName string) string {
	if emptyReqPkgName(modelPkgName) == gstModelPkgAlias {
		return gstModelPkgAlias + " " + GstModelImportPath
	}
	return GstModelImportPath
}

// emptyReqExpr builds the *<pkgName>.Empty type expression that generated
// code uses as the request type for dsl.PayloadEmpty.
func emptyReqExpr(pkgName string) ast.Expr {
	return &ast.StarExpr{
		X: &ast.SelectorExpr{
			X:   ast.NewIdent(pkgName),
			Sel: ast.NewIdent("Empty"),
		},
	}
}

// isEmptyPayload reports whether the request type name is the
// dsl.PayloadEmpty sentinel.
func isEmptyPayload(reqName string) bool { return reqName == dsl.PayloadEmpty }

// payloadTypeTarget resolves an action payload name to the package qualifier
// and type name used when rewriting existing service code. modelPkg is the
// business model package qualifier of the file being rewritten.
func payloadTypeTarget(payload, modelPkg string) (targetPkg, actionType string) {
	if isEmptyPayload(payload) {
		return emptyReqPkgName(modelPkg), "*Empty"
	}
	return modelPkg, payload
}

// ensureEmptyReqImportSpec inserts the gst model import into a parsed service
// file so a rewritten *model.Empty request type resolves. The import is
// aliased to gstmodel when the business model package is itself named
// "model". It reports whether the file was modified.
func ensureEmptyReqImportSpec(file *ast.File, modelPkg string) bool {
	if file == nil || findImportSpec(file, GstModelImportPath) != nil {
		return false
	}

	spec := &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: fmt.Sprintf("%q", GstModelImportPath),
		},
	}
	if emptyReqPkgName(modelPkg) == gstModelPkgAlias {
		spec.Name = ast.NewIdent(gstModelPkgAlias)
	}

	for _, decl := range file.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.IMPORT {
			genDecl.Specs = append(genDecl.Specs, spec)
			return true
		}
	}
	file.Decls = append([]ast.Decl{&ast.GenDecl{Tok: token.IMPORT, Specs: []ast.Spec{spec}}}, file.Decls...)
	return true
}

// pruneGstModelImportSpec removes the gst model import when the file no
// longer references its qualifier, so switching a request type back to a
// business type does not leave an unused import behind. Hand-written code
// that still references the package keeps the import. It reports whether the
// file was modified.
func pruneGstModelImportSpec(file *ast.File) bool {
	spec := findImportSpec(file, GstModelImportPath)
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
func findImportSpec(file *ast.File, importPath string) *ast.ImportSpec {
	for _, imp := range file.Imports {
		if imp.Path != nil && strings.Trim(imp.Path.Value, `"`) == importPath {
			return imp
		}
	}
	return nil
}
