package ggmodule

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"golang.org/x/tools/go/ast/astutil"
)

const middlewareRegistrationFilename = "middleware.go"

// moduleCopyMiddleware connects one manifest-declared framework middleware file
// to the project-owned file and registration call that module copy will create.
// Unlike action service files, a middleware file is not merged onto a generated
// shell. The whole source file is normalized into the project middleware
// package, which rewrites only the package clause and the copied model/service
// imports.
type moduleCopyMiddleware struct {
	SourcePath string
	TargetPath string
	Scope      moduleCopyMiddlewareScope
	Handler    string
}

func (p *CopyPlan) resolveMiddleware(manifest []moduleCopyMiddlewareManifest) ([]moduleCopyMiddleware, error) {
	middleware := make([]moduleCopyMiddleware, 0, len(manifest))
	for _, item := range manifest {
		// The manifest stores framework-root relative paths so module.json
		// remains stable no matter whether copy runs from gst itself or from a
		// consumer project with internal/gst symlinked in.
		sourcePath := filepath.Join(p.FrameworkRoot, filepath.FromSlash(item.SourceFile))
		targetPath := filepath.Join(p.TargetMiddlewareDir, filepath.Base(item.SourceFile))
		if err := requireMiddlewareSourceFile(sourcePath, item.Handler); err != nil {
			return nil, err
		}
		middleware = append(middleware, moduleCopyMiddleware{
			SourcePath: sourcePath,
			TargetPath: targetPath,
			Scope:      item.Scope,
			Handler:    item.Handler,
		})
	}
	sort.Slice(middleware, func(i, j int) bool {
		return middleware[i].TargetPath < middleware[j].TargetPath
	})
	return middleware, nil
}

func requireMiddlewareSourceFile(sourcePath string, handler string) error {
	if _, err := os.Stat(sourcePath); err != nil {
		return fmt.Errorf("source middleware file not found for %s: %w", filepath.Base(sourcePath), err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, sourcePath, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Recv == nil && fn.Name != nil && fn.Name.Name == handler {
			return nil
		}
	}
	return fmt.Errorf("source middleware file %s does not declare handler %s", sourcePath, handler)
}

func (p *CopyPlan) addMiddlewareFiles() error {
	for _, middleware := range p.Middleware {
		src, err := os.ReadFile(middleware.SourcePath)
		if err != nil {
			return err
		}
		content, err := normalizeModuleMiddlewareSource(middleware.SourcePath, src, p.rewriteConfig(middleware.TargetPath))
		if err != nil {
			return err
		}
		p.Files = append(p.Files, moduleCopyFile{
			Kind:        moduleCopyFileMiddleware,
			TargetPath:  middleware.TargetPath,
			Content:     content,
			Preexisting: fileExists(middleware.TargetPath),
		})
	}
	return nil
}

func (e *CopyExecution) registerMiddleware() (status moduleCopyWriteStatus, path string, err error) {
	targetDir := e.Plan.TargetMiddlewareDir
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
		return moduleCopyWriteSkip, targetPath, nil
	}

	if err := writeGoFile(targetPath, fset, file); err != nil {
		return "", "", err
	}
	e.WrittenFiles = append(e.WrittenFiles, targetPath)
	if preexisting {
		return moduleCopyWriteUpdate, targetPath, nil
	}
	return moduleCopyWriteCreate, targetPath, nil
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
	return ensureInitCall(file,
		func(initFn *ast.FuncDecl) bool { return middlewareRegisterCallExists(initFn, importAlias, middleware) },
		func(pos token.Pos) ast.Stmt { return middlewareRegisterCallStmt(importAlias, middleware, pos) })
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

// middlewareRegisterCallStmt anchors every generated node at pos for the reason
// spelled out on registerCallStmt: go/printer merges comments by token
// position, so a node at token.NoPos can be printed around existing init
// comments.
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
