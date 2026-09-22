package gen

import (
	"fmt"
	"go/ast"
	"go/token"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gertd/go-pluralize"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/ggconst"
)

var pluralizeCli = pluralize.NewClient()

// serviceScaffoldImports lists the framework packages a generated service file
// of an action with phase imports besides the model package: the gst service
// package its service struct embeds Base from, the gst package its methods
// take the ServiceContext from, and io for the Reader an Import method reads.
// The gst model package a model.Empty request or result needs is imported
// separately (see emptyReqImport).
func serviceScaffoldImports(phase consts.Phase) []string {
	importPaths := []string{ggconst.ImportPathService, ggconst.ImportPathGst}
	if phase == consts.PHASE_IMPORT {
		importPaths = append(importPaths, ggconst.ImportPathIO)
	}
	return importPaths
}

// serviceModelQualifier returns the name a generated service file of an
// action with phase refers to the model package by: its package name, or the
// alias ResolveImportConflicts picks when a package of serviceScaffoldImports
// takes that name. For example, the Create service file of a model in
// package service at "helloworld/model/service" imports
//
//	model_service "helloworld/model/service"
//	"github.com/hydroan/gst/service"
//	"github.com/hydroan/gst"
//
// and declares
//
//	type Creator struct {
//		service.Base[*model_service.Item, *model_service.Item, *model_service.Item]
//	}
//
// A model in package io is aliased model_io only in the service file of an
// Import action, the one that imports io.
func serviceModelQualifier(info *ModelInfo, phase consts.Phase) string {
	importPath := info.ImportPath()
	aliases := ResolveImportConflicts(map[string]string{importPath: info.ModelPkgName}, importNames(serviceScaffoldImports(phase))...)
	if alias := aliases[importPath]; alias != "" {
		return alias
	}
	return info.ModelPkgName
}

// modelImportSpec builds the import of the model package at importPath that a
// service file refers to by modelQualifier, named after the qualifier when
// the qualifier is not the last segment of the path:
//
//	"helloworld/model/sample"                     // qualifier sample
//	recorditem "helloworld/model/record_item"     // qualifier recorditem, the package name
//	model_service "helloworld/model/service"      // qualifier model_service, an alias
func modelImportSpec(importPath, modelQualifier string) *ast.ImportSpec {
	spec := &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(importPath)}}
	if path.Base(importPath) != modelQualifier {
		spec.Name = ast.NewIdent(modelQualifier)
	}
	return spec
}

// imports builds the imports of a generated service file of an action with
// phase, in this order: the model package under modelQualifier (see
// modelImportSpec), the packages of serviceScaffoldImports, then every otherPkg
// entry. For example, an Import action on a model in package io at
// "helloworld/model/io" builds
//
//	import (
//		model_io "helloworld/model/io"
//		"github.com/hydroan/gst/service"
//		"github.com/hydroan/gst"
//		"io"
//	)
func imports(modulePath, modelFileDir, modelQualifier string, phase consts.Phase, otherPkg ...string) *ast.GenDecl {
	genDecl := &ast.GenDecl{
		Tok:   token.IMPORT,
		Specs: []ast.Spec{modelImportSpec(filepath.Join(modulePath, modelFileDir), modelQualifier)},
	}
	for _, importPath := range serviceScaffoldImports(phase) {
		genDecl.Specs = append(genDecl.Specs, &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(importPath)}})
	}

	for _, pkg := range otherPkg {
		if len(pkg) == 0 {
			continue
		}
		// An entry in "alias path" form imports the package under the alias,
		// e.g. `gstmodel "github.com/hydroan/gst/model"`.
		value := fmt.Sprintf("%q", pkg)
		if alias, importPath, ok := strings.Cut(pkg, " "); ok {
			value = fmt.Sprintf("%s %q", alias, importPath)
		}
		genDecl.Specs = append(genDecl.Specs, &ast.ImportSpec{
			Path: &ast.BasicLit{
				Kind:  token.STRING,
				Value: value,
			},
		})
	}

	return genDecl
}

// actionTypeExpr builds the type expression of one explicit action type,
// transcribing the declared form: a leading '*' yields the pointer form and a
// bare name stays a value type, so for the package sample it builds
// *sample.RecordReq from *RecordReq and sample.RecordRsp from RecordRsp. The
// form itself is enforced by gg checks (struct types are pointers, slice and
// map types are values), so the generator emits exactly what the DSL
// declares.
func actionTypeExpr(pkgName, typeName string) ast.Expr {
	sel := &ast.SelectorExpr{
		X:   ast.NewIdent(pkgName),
		Sel: ast.NewIdent(strings.TrimPrefix(typeName, "*")),
	}
	if !strings.HasPrefix(typeName, "*") {
		return sel
	}
	return &ast.StarExpr{X: sel}
}

// actionTypeOrEmptyExpr builds the type expression of one action type,
// resolving the dsl.PayloadEmpty sentinel to *model.Empty from the gst model
// package (aliased when the file refers to the business model package as
// "model") and qualifying any other type by modelQualifier.
func actionTypeOrEmptyExpr(modelQualifier, typeName string) ast.Expr {
	if isEmptyPayload(typeName) {
		return emptyReqExpr(emptyReqPkgName(modelQualifier))
	}
	return actionTypeExpr(modelQualifier, typeName)
}

// types builds the declaration of the service struct named roleName, which
// embeds service.Base over the model and the action's request and response
// types, referring to the model package by modelQualifier (see
// serviceModelQualifier):
//
//	type Updater struct {
//		service.Base[*model.User, *model.UserReq, *model.UserRsp]
//	}
//
// or, with modelQualifier model_service,
//
//	type Updater struct {
//		service.Base[*model_service.User, *model_service.UserReq, *model_service.UserRsp]
//	}
func types(modelQualifier, modelName, reqName, rspName, roleName string) *ast.GenDecl {
	// The dsl.PayloadEmpty sentinel resolves to *model.Empty from the gst
	// model package on either side; any other action type is emitted in its
	// declared form.
	reqExpr := actionTypeOrEmptyExpr(modelQualifier, reqName)
	rspExpr := actionTypeOrEmptyExpr(modelQualifier, rspName)

	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{
			&ast.TypeSpec{
				// eg: Creator, Updater, Deleter.
				Name: ast.NewIdent(roleName),
				Type: &ast.StructType{
					Fields: &ast.FieldList{
						List: []*ast.Field{
							{
								Type: &ast.IndexListExpr{
									X: &ast.SelectorExpr{
										X:   ast.NewIdent("service"),
										Sel: ast.NewIdent("Base"),
									},
									Indices: []ast.Expr{
										&ast.StarExpr{
											X: &ast.SelectorExpr{
												X:   ast.NewIdent(modelQualifier),
												Sel: ast.NewIdent(modelName),
											},
										},
										reqExpr,
										rspExpr,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// serviceMethod1 builds the declaration of a hook taking one model, with the
// given body. For example:
//
//	func (u *Creator) CreateBefore(ctx *gst.ServiceContext, user *model.User) error {
//	}
//
//	func (g *Updater) UpdateAfter(ctx *gst.ServiceContext, group *model_auth.Group) error {
//	}
func serviceMethod1(recvName, modelName, modelQualifier string, phase consts.Phase, roleName string, body ...ast.Stmt) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: &ast.FieldList{
			List: []*ast.Field{
				{
					Names: []*ast.Ident{ast.NewIdent(recvName)},
					Type: &ast.StarExpr{
						X: ast.NewIdent(roleName),
					},
				},
			},
		},
		Name: ast.NewIdent(phase.MethodName()),
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					{
						Names: []*ast.Ident{ast.NewIdent("ctx")},
						Type: &ast.StarExpr{
							X: &ast.SelectorExpr{
								X:   ast.NewIdent("gst"),
								Sel: ast.NewIdent("ServiceContext"),
							},
						},
					},
					{
						Names: []*ast.Ident{ast.NewIdent(strings.ToLower(modelName))},
						Type: &ast.StarExpr{
							X: &ast.SelectorExpr{
								X:   ast.NewIdent(modelQualifier),
								Sel: ast.NewIdent(modelName),
							},
						},
					},
				},
			},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{
						Type: ast.NewIdent("error"),
					},
				},
			},
		},
		Body: &ast.BlockStmt{
			List: body,
		},
	}
}

// serviceMethod2 builds the declaration of a hook taking a pointer to a list
// of models, with the given body. For example:
//
//	func (u *Lister) ListBefore(ctx *gst.ServiceContext, users *[]*model.User) error {
//	}
func serviceMethod2(recvName, modelName, modelQualifier string, phase consts.Phase, roleName string, body ...ast.Stmt) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: &ast.FieldList{
			List: []*ast.Field{
				{
					Names: []*ast.Ident{ast.NewIdent(recvName)},
					Type: &ast.StarExpr{
						X: ast.NewIdent(roleName),
					},
				},
			},
		},
		Name: ast.NewIdent(phase.MethodName()),
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					{
						Names: []*ast.Ident{ast.NewIdent("ctx")},
						Type: &ast.StarExpr{
							X: &ast.SelectorExpr{
								X:   ast.NewIdent("gst"),
								Sel: ast.NewIdent("ServiceContext"),
							},
						},
					},
					{
						Names: []*ast.Ident{ast.NewIdent(pluralizeCli.Plural(strings.ToLower(modelName)))},
						Type: &ast.StarExpr{
							X: &ast.ArrayType{
								Elt: &ast.StarExpr{
									X: &ast.SelectorExpr{
										X:   ast.NewIdent(modelQualifier),
										Sel: ast.NewIdent(modelName),
									},
								},
							},
						},
					},
				},
			},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{
						Type: ast.NewIdent("error"),
					},
				},
			},
		},
		Body: &ast.BlockStmt{
			List: body,
		},
	}
}

// serviceMethod3 builds the declaration of a hook taking models as variadic
// arguments, with the given body. For example:
//
//	func (u *ManyCreator) CreateManyBefore(ctx *gst.ServiceContext, users ...*model.User) error {
//	}
func serviceMethod3(recvName, modelName, modelQualifier string, phase consts.Phase, roleName string, body ...ast.Stmt) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: &ast.FieldList{
			List: []*ast.Field{
				{
					Names: []*ast.Ident{ast.NewIdent(recvName)},
					Type: &ast.StarExpr{
						X: ast.NewIdent(roleName),
					},
				},
			},
		},
		Name: ast.NewIdent(phase.MethodName()),
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					{
						Names: []*ast.Ident{ast.NewIdent("ctx")},
						Type: &ast.StarExpr{
							X: &ast.SelectorExpr{
								X:   ast.NewIdent("gst"),
								Sel: ast.NewIdent("ServiceContext"),
							},
						},
					},
					{
						Names: []*ast.Ident{ast.NewIdent(pluralizeCli.Plural(strings.ToLower(modelName)))},
						Type: &ast.Ellipsis{
							Elt: &ast.StarExpr{
								X: &ast.SelectorExpr{
									X:   ast.NewIdent(modelQualifier),
									Sel: ast.NewIdent(modelName),
								},
							},
						},
					},
				},
			},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{
						Type: ast.NewIdent("error"),
					},
				},
			},
		},
		Body: &ast.BlockStmt{
			List: body,
		},
	}
}

// serviceMethod4 builds the declaration of an action method taking the
// request and returning the result, with the given body. For example:
//
//	func (u *Creator) Create(ctx *gst.ServiceContext, req *model.User) (rsp *model.User, err error) {
//	}
func serviceMethod4(recvName, modelQualifier, reqName, rspName string, phase consts.Phase, roleName string, body ...ast.Stmt) *ast.FuncDecl {
	// The dsl.PayloadEmpty sentinel resolves to *model.Empty from the gst
	// model package on either side; any other action type is emitted in its
	// declared form.
	reqExpr := actionTypeOrEmptyExpr(modelQualifier, reqName)
	rspExpr := actionTypeOrEmptyExpr(modelQualifier, rspName)

	return &ast.FuncDecl{
		Recv: &ast.FieldList{
			List: []*ast.Field{
				{
					Names: []*ast.Ident{ast.NewIdent(recvName)},
					Type: &ast.StarExpr{
						X: ast.NewIdent(roleName),
					},
				},
			},
		},
		Name: ast.NewIdent(phase.MethodName()),
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					{
						Names: []*ast.Ident{ast.NewIdent("ctx")},
						Type: &ast.StarExpr{
							X: &ast.SelectorExpr{
								X:   ast.NewIdent("gst"),
								Sel: ast.NewIdent("ServiceContext"),
							},
						},
					},
					{
						Names: []*ast.Ident{ast.NewIdent("req")},
						Type:  reqExpr,
					},
				},
			},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{
						Names: []*ast.Ident{ast.NewIdent("rsp")},
						Type:  rspExpr,
					},
					{
						Names: []*ast.Ident{ast.NewIdent("err")},
						Type:  ast.NewIdent("error"),
					},
				},
			},
		},
		Body: &ast.BlockStmt{
			List: body,
		},
	}
}

// serviceMethod5 builds the declaration of an Import method reading models,
// with the given body. For example:
//
//	func (a *Importer) Import(ctx *gst.ServiceContext, reader io.Reader) (samples []*model.Sample, err error) {
//	}
func serviceMethod5(recvName, modelName, modelQualifier, roleName string, body ...ast.Stmt) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: &ast.FieldList{
			List: []*ast.Field{
				{
					Names: []*ast.Ident{ast.NewIdent(recvName)},
					Type: &ast.StarExpr{
						X: ast.NewIdent(roleName),
					},
				},
			},
		},
		Name: ast.NewIdent("Import"),
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					{
						Names: []*ast.Ident{ast.NewIdent("ctx")},
						Type: &ast.StarExpr{
							X: &ast.SelectorExpr{
								X:   ast.NewIdent("gst"),
								Sel: ast.NewIdent("ServiceContext"),
							},
						},
					},
					{
						Names: []*ast.Ident{ast.NewIdent("reader")},
						Type: &ast.SelectorExpr{
							X:   ast.NewIdent("io"),
							Sel: ast.NewIdent("Reader"),
						},
					},
				},
			},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{
						Names: []*ast.Ident{ast.NewIdent(pluralizeCli.Plural(strings.ToLower(modelName)))},
						Type: &ast.ArrayType{
							Elt: &ast.StarExpr{
								X: &ast.SelectorExpr{
									X:   ast.NewIdent(modelQualifier),
									Sel: ast.NewIdent(modelName),
								},
							},
						},
					},
					{
						Names: []*ast.Ident{ast.NewIdent("err")},
						Type:  ast.NewIdent("error"),
					},
				},
			},
		},
		Body: &ast.BlockStmt{
			List: body,
		},
	}
}

// serviceMethod6 builds the declaration of an Export method writing models,
// with the given body. For example:
//
//	func (a *Exporter) Export(ctx *gst.ServiceContext, samples ...*model.Sample) (data []byte, err error) {
//	}
func serviceMethod6(recvName, modelName, modelQualifier, roleName string, body ...ast.Stmt) *ast.FuncDecl {
	paramName := pluralizeCli.Plural(strings.ToLower(modelName))

	return &ast.FuncDecl{
		Recv: &ast.FieldList{
			List: []*ast.Field{
				{
					Names: []*ast.Ident{ast.NewIdent(recvName)},
					Type: &ast.StarExpr{
						X: ast.NewIdent(roleName),
					},
				},
			},
		},
		Name: ast.NewIdent("Export"),
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					{
						Names: []*ast.Ident{ast.NewIdent("ctx")},
						Type: &ast.StarExpr{
							X: &ast.SelectorExpr{
								X:   ast.NewIdent("gst"),
								Sel: ast.NewIdent("ServiceContext"),
							},
						},
					},
					{
						Names: []*ast.Ident{ast.NewIdent(paramName)},
						Type: &ast.Ellipsis{
							Elt: &ast.StarExpr{
								X: &ast.SelectorExpr{
									X:   ast.NewIdent(modelQualifier),
									Sel: ast.NewIdent(modelName),
								},
							},
						},
					},
				},
			},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{
						Names: []*ast.Ident{ast.NewIdent("data")},
						Type: &ast.ArrayType{
							Elt: ast.NewIdent("byte"),
						},
					},
					{
						Names: []*ast.Ident{ast.NewIdent("err")},
						Type:  ast.NewIdent("error"),
					},
				},
			},
		},
		Body: &ast.BlockStmt{
			List: body,
		},
	}
}

// serviceMethod7 builds the declaration of an SSE method, with the given
// body. For example:
//
//	func (a *Streamer) SSE(ctx *gst.ServiceContext) (err error) {
//	}
func serviceMethod7(recvName, roleName string, body ...ast.Stmt) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: &ast.FieldList{
			List: []*ast.Field{
				{
					Names: []*ast.Ident{ast.NewIdent(recvName)},
					Type: &ast.StarExpr{
						X: ast.NewIdent(roleName),
					},
				},
			},
		},
		Name: ast.NewIdent("SSE"),
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					{
						Names: []*ast.Ident{ast.NewIdent("ctx")},
						Type: &ast.StarExpr{
							X: &ast.SelectorExpr{
								X:   ast.NewIdent("gst"),
								Sel: ast.NewIdent("ServiceContext"),
							},
						},
					},
				},
			},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{
						Names: []*ast.Ident{ast.NewIdent("err")},
						Type:  ast.NewIdent("error"),
					},
				},
			},
		},
		Body: &ast.BlockStmt{
			List: body,
		},
	}
}
