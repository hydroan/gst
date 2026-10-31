package gggen

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/goast"
	"github.com/hydroan/gst/internal/modelinfo"
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
func RouterGstModelUse(models []*modelinfo.Model) (pkgName string, needed bool) {
	pkgName = gstModelPkgName
	for _, m := range models {
		if m.Design == nil {
			continue
		}
		routed := false
		m.Design.Range(func(_ string, act *dsl.Action) {
			// A Stream action registers no route: the router file names
			// none of its types.
			if dsl.GRPCOnlyAction(act.Phase.Name()) {
				return
			}
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
		return gstModelPkgAlias + " " + ggconst.ImportPathModel
	}
	return ggconst.ImportPathModel
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

// gstModelQualifier returns the qualifier a service file being rewritten
// refers to the gst model package by, for a dsl.PayloadEmpty request or
// result: the name the file's import of the package binds, model or an
// alias, when it imports the package; otherwise model (see emptyReqPkgName),
// unless the business model package is referred to as model (modelPkg) or
// another import of the file takes that name, the root model package of a
// project imported beside a nested one, in which case the gstmodel alias. A
// file of package item importing "helloworld/model" and
// "helloworld/model/record" gets gstmodel, one importing
// gstmodel "github.com/hydroan/gst/model" keeps gstmodel, and one importing
// "helloworld/model/box" alone gets model.
func gstModelQualifier(file *ast.File, modelPkg string) string {
	if spec := goast.FindImportSpec(file, ggconst.ImportPathModel); spec != nil {
		if spec.Name != nil {
			return spec.Name.Name
		}
		return gstModelPkgName
	}
	if modelPkg == gstModelPkgName {
		return gstModelPkgAlias
	}
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		if importedAs(spec, importPath) == gstModelPkgName {
			return gstModelPkgAlias
		}
	}
	return gstModelPkgName
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
// rewritten and gstModelPkg the qualifier it refers to the gst model package
// by (see gstModelQualifier). With modelPkg sample and gstModelPkg model it
// returns ("model", "*Empty") for dsl.PayloadEmpty and ("sample",
// "*RecordReq") for *RecordReq.
func payloadTypeTarget(payload, modelPkg, gstModelPkg string) (targetPkg, actionType string) {
	if isEmptyPayload(payload) {
		return gstModelPkg, "*Empty"
	}
	return modelPkg, payload
}

// ensureEmptyReqImportSpec inserts the gst model import into a parsed service
// file so a rewritten *model.Empty request or result type resolves, under
// gstModelPkg, the qualifier the file refers to the package by (see
// gstModelQualifier): for gstmodel it inserts
//
//	gstmodel "github.com/hydroan/gst/model"
//
// and "github.com/hydroan/gst/model" for model. It reports whether the file
// was modified.
func ensureEmptyReqImportSpec(file *ast.File, gstModelPkg string) bool {
	if file == nil || goast.FindImportSpec(file, ggconst.ImportPathModel) != nil {
		return false
	}

	spec := &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: fmt.Sprintf("%q", ggconst.ImportPathModel),
		},
	}
	if gstModelPkg != gstModelPkgName {
		spec.Name = ast.NewIdent(gstModelPkg)
	}
	goast.InsertImportSpec(file, spec)
	return true
}

// pruneGstModelImportSpec removes the gst model import when the file no
// longer references its qualifier, so switching a request or result type back
// to a business type does not leave an unused import behind. Hand-written code
// that still references the package keeps the import. It reports whether the
// file was modified.
func pruneGstModelImportSpec(file *ast.File) bool {
	spec := goast.FindImportSpec(file, ggconst.ImportPathModel)
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
