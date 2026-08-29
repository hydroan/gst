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
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

const middlewareRegistrationFilename = "middleware.go"

// middlewareMarkerPrefix opens the ownership marker line module copy writes at
// the top of every copied middleware file. The middleware directory is shared
// with project-owned handlers, so this marker is the only proof that a file
// belongs to a copied module and may be pruned with it.
const middlewareMarkerPrefix = "// Managed by gg module copy (module "

// moduleCopyMiddlewareMarker returns the ownership marker line for one module.
func moduleCopyMiddlewareMarker(moduleName string) string {
	return middlewareMarkerPrefix + moduleName + "). Removing the module removes this file."
}

// middlewareMarkerModule returns the module name a middleware file declares in
// its ownership marker, or "" for files module copy does not own. Only lines
// before the package clause count, mirroring where the copy writes the marker.
func middlewareMarkerModule(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(string(content), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if after, ok := strings.CutPrefix(line, middlewareMarkerPrefix); ok {
			if name, _, found := strings.Cut(after, ")"); found {
				return name, nil
			}
		}
		if strings.HasPrefix(strings.TrimSpace(line), "package ") {
			break
		}
	}
	return "", nil
}

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
		if !ok || fn.Recv != nil || fn.Name == nil || fn.Name.Name != handler {
			continue
		}
		// Registration is written into the project middleware package as
		// handler(), so a handler that takes arguments would emit code that
		// does not compile. Failing here names the manifest entry instead of
		// leaving the copied project broken.
		if fn.Type.Params != nil && len(fn.Type.Params.List) > 0 {
			return fmt.Errorf("middleware handler %s in %s must take no arguments", handler, sourcePath)
		}

		return nil
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
		// The ownership marker is part of the planned content, so idempotent
		// re-copies compare equal and marker-less files from older copies show
		// up as a --force overwrite that upgrades them into prune management.
		content = append([]byte(moduleCopyMiddlewareMarker(p.Name)+"\n\n"), content...)
		p.Files = append(p.Files, moduleCopyFile{
			Kind:        moduleCopyFileMiddleware,
			TargetPath:  middleware.TargetPath,
			Content:     content,
			Preexisting: fileExists(middleware.TargetPath),
		})
	}
	return nil
}

// collectStaleMiddlewareFiles records project middleware files this module's
// copy marker claims but the current manifest no longer declares. Ownership
// must be proven by the marker: the middleware directory is shared with
// project-owned handlers and other modules' copies, so an unmarked or
// foreign-marked file is never touched. The registration file is skipped no
// matter what it carries — it is project infrastructure, not a copy product.
func (p *CopyPlan) collectStaleMiddlewareFiles() error {
	info, err := os.Stat(p.TargetMiddlewareDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", p.TargetMiddlewareDir)
	}

	planned := make(map[string]bool)
	for _, target := range p.MiddlewareTargets() {
		planned[target] = true
	}
	files, err := goFilesInPackageDir(p.TargetMiddlewareDir)
	if err != nil {
		return err
	}
	stale := make([]string, 0)
	for _, path := range files {
		if filepath.Base(path) == middlewareRegistrationFilename || planned[path] {
			continue
		}
		owner, ownerErr := middlewareMarkerModule(path)
		if ownerErr != nil {
			return ownerErr
		}
		if owner != p.Name {
			continue
		}
		stale = append(stale, path)
	}
	p.StaleMiddlewareFiles = stale
	return nil
}

// middlewareHandlersOnDisk collects the top-level function names of the
// planned middleware targets as they currently exist on disk, before the copy
// overwrites them. A handler rename would otherwise leave its old register
// call behind: the new file content no longer proves the old name belonged to
// this module, so the proof must be taken while the old content is still
// there. Only files carrying this module's marker count — an unmarked
// preexisting file is not provably module-owned.
func (e *CopyExecution) middlewareHandlersOnDisk() (map[string]bool, error) {
	handlers := make(map[string]bool)
	for _, middleware := range e.Plan.Middleware {
		if !fileExists(middleware.TargetPath) {
			continue
		}
		owner, err := middlewareMarkerModule(middleware.TargetPath)
		if err != nil {
			return nil, err
		}
		if owner != e.Plan.Name {
			continue
		}
		names, err := topLevelFunctionNames(middleware.TargetPath)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			handlers[name] = true
		}
	}
	return handlers, nil
}

// reconcileMiddlewareRegistrations makes middleware/middleware.go agree with
// the manifest for every handler this module owns: a register call whose
// handler is module-owned but no longer matches a declared (handler, scope)
// pair is dropped — that is how a scope change or a handler rename retires its
// old call — and every declared pair is ensured. The owned set is the union of
// the pre-overwrite disk content (obsoleteHandlers) and the freshly written
// targets, so both old and new handler names are attributable. Handlers the
// module does not own are never touched.
//
// The edit is intentionally AST-based. The middleware template often contains
// explanatory comments, grouped imports, or existing init work; AST editing
// preserves those structures while touching only the import and calls owned
// by module copy.
func (e *CopyExecution) reconcileMiddlewareRegistrations(obsoleteHandlers map[string]bool) (status moduleCopyWriteStatus, path string, err error) {
	targetDir := e.Plan.TargetMiddlewareDir
	targetPath := filepath.Join(targetDir, middlewareRegistrationFilename)
	fset, file, preexisting, err := parseOrCreateMiddlewareRegistrationFile(targetPath)
	if err != nil {
		return "", "", err
	}

	ownedHandlers := make(map[string]bool, len(obsoleteHandlers))
	for name := range obsoleteHandlers {
		ownedHandlers[name] = true
	}
	expected := make(map[string]bool, len(e.Plan.Middleware))
	for _, middleware := range e.Plan.Middleware {
		names, namesErr := topLevelFunctionNames(middleware.TargetPath)
		if namesErr != nil {
			return "", "", namesErr
		}
		for _, name := range names {
			ownedHandlers[name] = true
		}
		expected[middlewareRegisterMethod(middleware)+"/"+middleware.Handler] = true
	}

	changed := false
	importAlias := frameworkMiddlewareImportAlias(file)
	if importAlias != "" {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name == nil || fn.Name.Name != "init" || fn.Body == nil {
				continue
			}
			kept := make([]ast.Stmt, 0, len(fn.Body.List))
			for _, stmt := range fn.Body.List {
				if call, isCall := callExprFromStmt(stmt); isCall {
					if method, handler, isRegister := parseMiddlewareRegisterCall(call, importAlias); isRegister && ownedHandlers[handler] && !expected[method+"/"+handler] {
						changed = true
						continue
					}
				}
				kept = append(kept, stmt)
			}
			fn.Body.List = kept
		}
	}
	if importAlias == "" {
		changed = astutil.AddImport(fset, file, frameworkModulePath+"/middleware") || changed
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

// topLevelFunctionNames returns the names of the top-level functions a Go
// file declares. A file that is already gone contributes nothing: its
// registration calls, if any, are beyond what a prune can still attribute.
func topLevelFunctionNames(path string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	names := make([]string, 0)
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name != nil {
			names = append(names, fn.Name.Name)
		}
	}
	return names, nil
}

// removeMiddlewareRegistrations drops the middleware.Register and
// middleware.RegisterAuth calls whose zero-argument handler constructors are
// named in handlerNames. It edits only init functions, mirrors the shape
// matching of ensureMiddlewareRegisterCall, and leaves the registration file
// untouched when nothing matches.
func (e *CopyExecution) removeMiddlewareRegistrations(handlerNames map[string]bool) error {
	if len(handlerNames) == 0 {
		return nil
	}
	targetPath := filepath.Join(e.Plan.TargetMiddlewareDir, middlewareRegistrationFilename)
	src, err := os.ReadFile(targetPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, targetPath, src, parser.ParseComments)
	if err != nil {
		return err
	}
	importAlias := frameworkMiddlewareImportAlias(file)
	if importAlias == "" {
		return nil
	}

	changed := false
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name == nil || fn.Name.Name != "init" || fn.Body == nil {
			continue
		}
		kept := make([]ast.Stmt, 0, len(fn.Body.List))
		for _, stmt := range fn.Body.List {
			if call, isCall := callExprFromStmt(stmt); isCall && isPrunedMiddlewareRegisterCall(call, importAlias, handlerNames) {
				changed = true
				continue
			}
			kept = append(kept, stmt)
		}
		fn.Body.List = kept
	}
	if !changed {
		return nil
	}
	safePath, err := requirePathUnderRoot(targetPath, e.Plan.TargetMiddlewareDir)
	if err != nil {
		return err
	}
	if err := writeGoFile(safePath, fset, file); err != nil { // #nosec G703 -- safePath validated under the middleware dir by requirePathUnderRoot
		return err
	}
	printModuleCopyStatus(moduleCopyWriteUpdate, safePath)
	e.WrittenFiles = append(e.WrittenFiles, safePath)
	return nil
}

// isPrunedMiddlewareRegisterCall matches middleware.Register(Handler()) and
// middleware.RegisterAuth(Handler()) calls whose handler name a pruned file
// declared.
func isPrunedMiddlewareRegisterCall(call *ast.CallExpr, importAlias string, handlerNames map[string]bool) bool {
	_, handler, ok := parseMiddlewareRegisterCall(call, importAlias)
	return ok && handlerNames[handler]
}

// parseMiddlewareRegisterCall recognizes the exact call shape module copy
// manages — middleware.Register(Handler()) or middleware.RegisterAuth(
// Handler()) with a zero-argument handler constructor — and returns its
// method and handler names.
func parseMiddlewareRegisterCall(call *ast.CallExpr, importAlias string) (method string, handler string, ok bool) {
	sel, selOK := call.Fun.(*ast.SelectorExpr)
	if !selOK || (sel.Sel.Name != "Register" && sel.Sel.Name != "RegisterAuth") || len(call.Args) != 1 {
		return "", "", false
	}
	ident, identOK := sel.X.(*ast.Ident)
	if !identOK || ident.Name != importAlias {
		return "", "", false
	}
	handlerCall, callOK := call.Args[0].(*ast.CallExpr)
	if !callOK || len(handlerCall.Args) != 0 {
		return "", "", false
	}
	handlerIdent, handlerOK := handlerCall.Fun.(*ast.Ident)
	if !handlerOK {
		return "", "", false
	}
	return sel.Sel.Name, handlerIdent.Name, true
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
