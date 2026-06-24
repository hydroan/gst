package ggmodule

import (
	"bytes"
	"go/ast"
	goformat "go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/tools/go/ast/astutil"
	gofumpt "mvdan.cc/gofumpt/format"
)

const middlewareRegistrationFilename = "middleware.go"

func (p *CopyPlan) targetMiddlewareDir() string {
	if p.TargetMiddlewareDir == "" {
		return defaultMiddlewareDir
	}
	return p.TargetMiddlewareDir
}

func (e *CopyExecution) registerMiddleware() (status string, path string, err error) {
	targetDir := e.Plan.targetMiddlewareDir()
	targetPath := filepath.Join(targetDir, middlewareRegistrationFilename)
	fset, file, preexisting, err := parseOrCreateMiddlewareRegistrationFile(targetPath)
	if err != nil {
		return "", "", err
	}

	// Registration is intentionally AST-based. The middleware template often
	// contains explanatory comments, grouped imports, or existing init work; AST
	// editing preserves those structures while adding only the import and calls
	// owned by module copy.
	importAlias := frameworkMiddlewareImportAlias(file)
	changed := false
	if importAlias == "" {
		changed = astutil.AddImport(fset, file, frameworkModulePath+"/middleware")
		importAlias = "middleware"
	}
	for _, item := range e.Plan.Middleware {
		if ensureMiddlewareRegisterCall(file, importAlias, item) {
			changed = true
		}
	}
	if !changed {
		return "SKIP", targetPath, nil
	}

	formatted, err := formatGoFile(fset, file)
	if err != nil {
		return "", "", err
	}
	if err := ensureParentDir(targetPath); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(targetPath, formatted, 0o600); err != nil {
		return "", "", err
	}
	e.WrittenFiles = append(e.WrittenFiles, targetPath)
	if preexisting {
		return "UPDATE", targetPath, nil
	}
	return "CREATE", targetPath, nil
}

func parseOrCreateMiddlewareRegistrationFile(path string) (*token.FileSet, *ast.File, bool, error) {
	fset := token.NewFileSet()
	src, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Older or hand-written projects may not have the template file. Creating
		// a minimal package file lets the same AST path handle both new and
		// existing projects without special string-concatenation output.
		file, parseErr := parser.ParseFile(fset, path, []byte("package middleware\n"), parser.ParseComments)
		return fset, file, false, parseErr
	}
	if err != nil {
		return nil, nil, false, err
	}
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	return fset, file, true, err
}

func frameworkMiddlewareImportAlias(file *ast.File) string {
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != frameworkModulePath+"/middleware" {
			continue
		}
		// New registrations use the default middleware import. If a project
		// already chose an alias manually, reuse it so module copy stays
		// idempotent instead of adding a competing import for the same package.
		if spec.Name != nil && spec.Name.Name != "." && spec.Name.Name != "_" {
			return spec.Name.Name
		}
		return "middleware"
	}
	return ""
}

func ensureMiddlewareRegisterCall(file *ast.File, importAlias string, middleware moduleCopyMiddleware) bool {
	initFn := ensureInitFunc(file)
	if middlewareRegisterCallExists(initFn, importAlias, middleware) {
		return false
	}
	initFn.Body.List = append(initFn.Body.List, middlewareRegisterCallStmt(importAlias, middleware, initFn.Body.Rbrace))
	return true
}

func middlewareRegisterCallExists(fn *ast.FuncDecl, importAlias string, middleware moduleCopyMiddleware) bool {
	if fn == nil || fn.Body == nil {
		return false
	}
	method := middlewareRegisterMethod(middleware)
	for _, stmt := range fn.Body.List {
		call, ok := callExprFromStmt(stmt)
		if !ok {
			continue
		}
		if !isMiddlewareRegisterCall(call, importAlias, method, middleware.Handler) {
			continue
		}
		return true
	}
	return false
}

func isMiddlewareRegisterCall(call *ast.CallExpr, importAlias string, method string, handlerName string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != method || len(call.Args) != 1 {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != importAlias {
		return false
	}

	handler, ok := call.Args[0].(*ast.CallExpr)
	if !ok || len(handler.Args) != 0 {
		return false
	}
	handlerIdent, ok := handler.Fun.(*ast.Ident)
	return ok && handlerIdent.Name == handlerName
}

func middlewareRegisterCallStmt(importAlias string, middleware moduleCopyMiddleware, pos token.Pos) ast.Stmt {
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   &ast.Ident{NamePos: pos, Name: importAlias},
			Sel: &ast.Ident{NamePos: pos, Name: middlewareRegisterMethod(middleware)},
		},
		Args: []ast.Expr{
			&ast.CallExpr{
				Fun:    &ast.Ident{NamePos: pos, Name: middleware.Handler},
				Lparen: pos,
				Rparen: pos,
			},
		},
		Lparen: pos,
		Rparen: pos,
	}}
}

func middlewareRegisterMethod(middleware moduleCopyMiddleware) string {
	if middleware.Scope == moduleCopyMiddlewareScopeAuth {
		return "RegisterAuth"
	}
	return "Register"
}

func formatGoFile(fset *token.FileSet, file *ast.File) ([]byte, error) {
	var buf bytes.Buffer
	if err := goformat.Node(&buf, fset, file); err != nil {
		return nil, err
	}
	return gofumpt.Source(buf.Bytes(), gofumpt.Options{})
}
