package gen

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/hydroan/gst/internal/codegen/constants"
	"github.com/hydroan/gst/types/consts"
)

// BuildModelFile generates a model.go file, the content like below:
/*
package model

import "github.com/hydroan/gst/model"

func init() {
	model.Register[*Group]()
	model.Register[*User]()
}
*/
func BuildModelFile(pkgName string, modelImports []string, stmts ...ast.Stmt) (string, error) {
	// Create init function body
	body := make([]ast.Stmt, 0, len(stmts))
	body = append(body, stmts...)

	// Create Init function declaration
	initDecl := &ast.FuncDecl{
		Name: ast.NewIdent(constants.FuncInit),
		Type: &ast.FuncType{
			TypeParams: nil,
			Params:     nil,
		},
		Body: &ast.BlockStmt{
			List: body,
		},
	}

	// Create import declaration
	importDecl := &ast.GenDecl{
		Tok: token.IMPORT,
		Specs: []ast.Spec{
			&ast.ImportSpec{
				Path: &ast.BasicLit{
					Kind:  token.STRING,
					Value: fmt.Sprintf(`"%s"`, constants.ImportPathModel),
				},
			},
		},
	}
	for _, modelImport := range modelImports {
		importDecl.Specs = append(importDecl.Specs, &ast.ImportSpec{
			Path: &ast.BasicLit{
				Kind:  token.STRING,
				Value: fmt.Sprintf(`"%s"`, modelImport),
			},
		})
	}

	// Create file AST
	f := &ast.File{
		Name:  ast.NewIdent(pkgName),
		Decls: []ast.Decl{
			// NOTE: imports must appear before other declarations
		},
	}

	// Create generated code comment at the top of the file
	generatedComment := &ast.CommentGroup{
		List: []*ast.Comment{
			{
				Text:  consts.CodeGeneratedComment(),
				Slash: token.Pos(1),
			},
		},
	}
	f.Comments = []*ast.CommentGroup{generatedComment}
	// Set package name position to ensure comment appears before it
	f.Name.NamePos = token.Pos(2)

	// If the caller does not pass stmts or stmts is empty, then the Init function body is empty,
	// So we should not imports any external package.
	if len(stmts) != 0 {
		// Add imports
		f.Decls = append(f.Decls, importDecl)
	}
	// Add init function
	f.Decls = append(f.Decls, initDecl)

	return FormatNodeExtra(f, false)
}

// BuildServiceFile generates a service.go file, the content like below:
/*
package service

import "github.com/hydroan/gst/service"

func Init() error {
	service.Register[*group]()
	service.Register[*user]()
	return nil
}
*/
// FIXME: process imports automatically problem.
func BuildServiceFile(pkgName string, modelImports []string, stmts ...ast.Stmt) (string, error) {
	// Handle import conflicts when modelImports contain packages with same base name
	// For example: ["myproject/service/pkg1/user", "myproject/service/pkg2/user"]
	// Should be renamed to:
	// import (
	//     pkg1_user "myproject/service/pkg1/user"
	//     pkg2_user "myproject/service/pkg2/user"
	// )
	importAliases := ResolveImportConflicts(modelImports)

	body := make([]ast.Stmt, 0, len(stmts))
	body = append(body, stmts...)

	initDecl := &ast.FuncDecl{
		Name: ast.NewIdent(constants.FuncInit),
		Type: &ast.FuncType{
			TypeParams: nil,
			Params:     nil,
		},
		Body: &ast.BlockStmt{
			List: body,
		},
	}

	// imports service
	imports := &ast.GenDecl{
		Tok: token.IMPORT,
		Specs: []ast.Spec{
			&ast.ImportSpec{
				Path: &ast.BasicLit{
					Kind:  token.STRING,
					Value: fmt.Sprintf(`"%s"`, constants.ImportPathService),
				},
			},
			&ast.ImportSpec{
				Path: &ast.BasicLit{
					Kind:  token.STRING,
					Value: fmt.Sprintf(`"%s"`, constants.ImportPathConsts),
				},
			},
		},
	}
	// imports, such like: "helloworld/model"
	// Use aliases to resolve import conflicts
	for _, importPath := range modelImports {
		alias := importAliases[importPath]
		importSpec := &ast.ImportSpec{
			Path: &ast.BasicLit{
				Kind:  token.STRING,
				Value: fmt.Sprintf("%q", importPath),
			},
		}
		// Add alias if needed to resolve conflicts
		if alias != "" {
			importSpec.Name = ast.NewIdent(alias)
		}
		imports.Specs = append(imports.Specs, importSpec)
	}

	f := &ast.File{
		Name:  ast.NewIdent(pkgName),
		Decls: []ast.Decl{
			// NOTE: imports must appear before other declarations
		},
	}

	// Create generated code comment at the top of the file
	generatedComment := &ast.CommentGroup{
		List: []*ast.Comment{
			{
				Text:  consts.CodeGeneratedComment(),
				Slash: token.Pos(1),
			},
		},
	}
	f.Comments = []*ast.CommentGroup{generatedComment}
	// Set package name position to ensure comment appears before it
	f.Name.NamePos = token.Pos(2)

	// If the caller does not pass stmts or stmts is empty, then the Init function body is empty,
	// So we should not imports any external package.
	if len(stmts) != 0 {
		// imports
		f.Decls = append(f.Decls, imports)
	}
	// Init() declarations.
	f.Decls = append(f.Decls, initDecl)

	return FormatNodeExtra(f, false)
}

// BuildRouterFile generates a router.go file, the content like below:
/*
package router

import (
	"helloworld/model"

	"github.com/hydroan/gst/router"
)

func Init() error {
	router.Register[*model.Group, *model.Group, *model.Group](router.Auth(), "group")
	router.Register[*model.User, *model.User, *model.User](router.Pub(), "user")
	return nil
}
*/
// FIXME: process imports automatically problem.
func BuildRouterFile(pkgName string, modelImports []string, stmts ...ast.Stmt) (string, error) {
	body := make([]ast.Stmt, 0, len(stmts)+1)
	body = append(body, stmts...)
	body = append(body, &ast.ReturnStmt{
		Results: []ast.Expr{
			ast.NewIdent("nil"),
		},
	})

	initDecl := &ast.FuncDecl{
		Name: ast.NewIdent(constants.FuncInit2),
		Type: &ast.FuncType{
			TypeParams: nil,
			Params:     nil,
			Results: &ast.FieldList{
				List: []*ast.Field{
					{Type: ast.NewIdent("error")},
				},
			},
		},
		Body: &ast.BlockStmt{
			List: body,
		},
	}

	importDecl := &ast.GenDecl{
		Tok: token.IMPORT,
		Specs: []ast.Spec{
			&ast.ImportSpec{
				Path: &ast.BasicLit{
					Kind:  token.STRING,
					Value: fmt.Sprintf(`"%s"`, constants.ImportPathRouter),
				},
			},
			&ast.ImportSpec{
				Path: &ast.BasicLit{
					Kind:  token.STRING,
					Value: fmt.Sprintf(`"%s"`, constants.ImportPathConsts),
				},
			},
		},
	}
	for _, imp := range modelImports {
		importDecl.Specs = append(importDecl.Specs, &ast.ImportSpec{
			Path: &ast.BasicLit{
				Kind:  token.STRING,
				Value: fmt.Sprintf("%q", imp),
			},
		})
	}

	f := &ast.File{
		Name:  ast.NewIdent(pkgName),
		Decls: []ast.Decl{
			// NOTE: imports must appear before other declarations
		},
	}

	// Create generated code comment at the top of the file
	generatedComment := &ast.CommentGroup{
		List: []*ast.Comment{
			{
				Text:  consts.CodeGeneratedComment(),
				Slash: token.Pos(1),
			},
		},
	}
	f.Comments = []*ast.CommentGroup{generatedComment}
	// Set package name position to ensure comment appears before it
	f.Name.NamePos = token.Pos(2)

	// If the caller does not pass stmts or stmts is empty, then the Init function body is empty,
	// So we should not imports any external package.
	if len(stmts) != 0 {
		// imports
		f.Decls = append(f.Decls, importDecl)
	}
	// Init() declarations.
	f.Decls = append(f.Decls, initDecl)

	return FormatNodeExtra(f, false)
}

// BuildMainFile generates a main.go file, the content like below:
/*
package main

import (
	- "helloworld/configx"
	_ "helloworld/cronjob"
	_ "helloworld/middleware"
	_ "helloworld/model"
	_ "helloworld/module"
	"helloworld/router"
	_ "helloworld/service"

	"github.com/hydroan/gst/bootstrap"
	. "github.com/hydroan/gst/util"
)

func main() {
	RunOrDie(bootstrap.Bootstrap)
	RunOrDie(configx.Init)
	RunOrDie(cronjob.Init)
	RunOrDie(service.Init)
	RunOrDie(router.Init)
	RunOrDie(bootstrap.Run)
}
*/
func BuildMainFile(projectName string) (string, error) {
	f := &ast.File{
		Name: ast.NewIdent(constants.PkgMain),
		Decls: []ast.Decl{
			&ast.GenDecl{
				Tok: token.IMPORT,
				Specs: []ast.Spec{
					&ast.ImportSpec{Path: &ast.BasicLit{Value: fmt.Sprintf("%q", projectName+"/"+constants.SubDirConfigx)}, Name: ast.NewIdent("_")},
					&ast.ImportSpec{Path: &ast.BasicLit{Value: fmt.Sprintf("%q", projectName+"/"+constants.SubDirCronjob)}, Name: ast.NewIdent("_")},
					&ast.ImportSpec{Path: &ast.BasicLit{Value: fmt.Sprintf("%q", projectName+"/"+constants.SubDirMiddleware)}, Name: ast.NewIdent("_")},
					&ast.ImportSpec{Path: &ast.BasicLit{Value: fmt.Sprintf("%q", projectName+"/"+constants.SubDirModel)}, Name: ast.NewIdent("_")},
					&ast.ImportSpec{Path: &ast.BasicLit{Value: fmt.Sprintf("%q", projectName+"/"+constants.SubDirService)}, Name: ast.NewIdent("_")},
					&ast.ImportSpec{Path: &ast.BasicLit{Value: fmt.Sprintf("%q", projectName+"/"+constants.SubDirModule)}, Name: ast.NewIdent("_")},
					&ast.ImportSpec{Path: &ast.BasicLit{Value: fmt.Sprintf("%q", projectName+"/"+constants.SubDirRouter)}},
					&ast.ImportSpec{Path: &ast.BasicLit{Value: fmt.Sprintf("%q", constants.ImportPathBootstrap)}},
					&ast.ImportSpec{
						Path: &ast.BasicLit{Value: fmt.Sprintf("%q", constants.ImportPathUtil)},
						Name: ast.NewIdent("."),
					},
				},
			},
			&ast.FuncDecl{
				Name: ast.NewIdent(constants.FuncMain),
				Type: &ast.FuncType{},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.ExprStmt{
							X: &ast.CallExpr{
								Fun: ast.NewIdent(constants.FuncRunOrDie),
								Args: []ast.Expr{
									&ast.SelectorExpr{
										X:   ast.NewIdent(constants.PkgBootstrap),
										Sel: ast.NewIdent(constants.BootstrapBootstrap),
									},
								},
							},
						},
						&ast.ExprStmt{
							X: &ast.CallExpr{
								Fun: ast.NewIdent(constants.FuncRunOrDie),
								Args: []ast.Expr{
									&ast.SelectorExpr{
										X:   ast.NewIdent(constants.PkgRouter),
										Sel: ast.NewIdent(constants.RouterInit),
									},
								},
							},
						},
						&ast.ExprStmt{
							X: &ast.CallExpr{
								Fun: ast.NewIdent(constants.FuncRunOrDie),
								Args: []ast.Expr{
									&ast.SelectorExpr{
										X:   ast.NewIdent(constants.PkgBootstrap),
										Sel: ast.NewIdent(constants.BootstrapRun),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Create generated code comment at the top of the file
	generatedComment := &ast.CommentGroup{
		List: []*ast.Comment{
			{
				Text:  consts.CodeGeneratedComment(),
				Slash: token.Pos(1),
			},
		},
	}
	f.Comments = []*ast.CommentGroup{generatedComment}
	// Set package name position to ensure comment appears before it
	f.Name.NamePos = token.Pos(2)

	return FormatNodeExtra(f, false)
}
