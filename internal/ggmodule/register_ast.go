package ggmodule

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
)

// existingModuleAlias returns the qualifier already used for this framework
// import, if the project has one. AddModule uses it for half-manual edits such
// as an import without a Register call, so the generated call still compiles.
func existingModuleAlias(file *ast.File, module Module) (string, bool) {
	for _, spec := range file.Imports {
		alias, ok := moduleAliasFromImportSpec(spec, module)
		if ok {
			return alias, true
		}
	}
	return "", false
}

// registeredModuleAlias requires both sides of registration to be present: an
// import of the framework module and a matching alias.Register() call in an init
// function. This makes add idempotent and lets remove fail loudly when a project
// is only partially edited instead of guessing which stray import or call the
// user intended to manage.
func registeredModuleAlias(file *ast.File, module Module) (string, bool) {
	aliases := make(map[string]bool)
	for _, spec := range file.Imports {
		alias, ok := moduleAliasFromImportSpec(spec, module)
		if ok {
			aliases[alias] = true
		}
	}
	if len(aliases) == 0 {
		return "", false
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "init" || fn.Body == nil {
			continue
		}
		for _, stmt := range fn.Body.List {
			call, ok := callExprFromStmt(stmt)
			if !ok {
				continue
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Register" {
				continue
			}
			ident, ok := sel.X.(*ast.Ident)
			if ok && aliases[ident.Name] {
				return ident.Name, true
			}
		}
	}
	return "", false
}

func checkModuleNotRegistered(name string) error {
	// Framework-module registration and local-source copy are mutually exclusive:
	// running both would register the same module's model/service/router paths twice.
	moduleFile := filepath.Join("module", "module.go")
	src, err := os.ReadFile(moduleFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, moduleFile, src, parser.ParseComments)
	if err != nil {
		return err
	}

	aliases := make(map[string]bool)
	importPath := filepath.Join(frameworkModulePath, "module", name)
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != importPath {
			continue
		}
		if spec.Name != nil {
			if spec.Name.Name == "." {
				return fmt.Errorf("framework module %s is already imported in %s", name, moduleFile)
			}
			if spec.Name.Name != "_" {
				aliases[spec.Name.Name] = true
			}
			continue
		}
		aliases[importPathBase(path)] = true
	}
	if len(aliases) == 0 {
		return nil
	}

	registered := false
	ast.Inspect(file, func(node ast.Node) bool {
		if registered {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Register" {
			return true
		}
		if ident, ok := sel.X.(*ast.Ident); ok && aliases[ident.Name] {
			registered = true
			return false
		}
		return true
	})
	if registered {
		return fmt.Errorf("framework module %s is already registered; remove it before copying local source", name)
	}
	return nil
}

func importPathBase(path string) string {
	return filepath.Base(filepath.ToSlash(path))
}

// ensureRegisterCall appends the framework registration to an existing init
// function or creates one when module/module.go does not have an init yet.
// Register calls are intentionally appended so existing framework/project setup
// keeps its order.
func ensureRegisterCall(file *ast.File, alias string) bool {
	initFn := ensureInitFunc(file)
	if registerCallExists(initFn, alias) {
		return false
	}
	initFn.Body.List = append(initFn.Body.List, registerCallStmt(alias, initFn.Body.Rbrace))
	return true
}

func ensureInitFunc(file *ast.File) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Recv == nil && fn.Name != nil && fn.Name.Name == "init" {
			if fn.Body == nil {
				fn.Body = &ast.BlockStmt{}
			}
			return fn
		}
	}
	fn := &ast.FuncDecl{
		Name: ast.NewIdent("init"),
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: &ast.BlockStmt{},
	}
	file.Decls = append(file.Decls, fn)
	return fn
}

func registerCallExists(fn *ast.FuncDecl, alias string) bool {
	if fn == nil || fn.Body == nil {
		return false
	}
	for _, stmt := range fn.Body.List {
		call, ok := callExprFromStmt(stmt)
		if !ok {
			continue
		}
		if isRegisterCall(call, alias) {
			return true
		}
	}
	return false
}

func registerCallStmt(alias string, pos token.Pos) ast.Stmt {
	// The position is not cosmetic. go/printer merges comments into the output by
	// token position; a newly-created AST node with token.NoPos can be printed
	// around existing init comments in surprising ways, including splitting
	// `copytest.Register()` into `copytest.` + comment + `Register()`. Anchoring the
	// generated call at the init block's closing brace makes the call a normal
	// statement after any placeholder comments while preserving those comments.
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   &ast.Ident{NamePos: pos, Name: alias},
			Sel: &ast.Ident{NamePos: pos, Name: "Register"},
		},
		Lparen: pos,
		Rparen: pos,
	}}
}

// removeRegisterCall filters only the exact alias.Register() statement that gg
// manages. Other init statements remain untouched, including manual setup for
// the same package that takes arguments or calls different functions.
func removeRegisterCall(file *ast.File, alias string) bool {
	var changed bool
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "init" || fn.Body == nil {
			continue
		}
		var removedFromInit bool
		stmts := fn.Body.List[:0]
		for _, stmt := range fn.Body.List {
			call, ok := callExprFromStmt(stmt)
			if ok && isRegisterCall(call, alias) {
				changed = true
				removedFromInit = true
				continue
			}
			stmts = append(stmts, stmt)
		}
		fn.Body.List = stmts
		if removedFromInit && len(fn.Body.List) == 0 {
			compactEmptyInitBody(file, fn.Body)
		}
	}
	return changed
}

func compactEmptyInitBody(file *ast.File, body *ast.BlockStmt) {
	var lastCommentEnd token.Pos
	for _, group := range file.Comments {
		if group.Pos() <= body.Lbrace || group.End() >= body.Rbrace {
			continue
		}
		if group.End() > lastCommentEnd {
			lastCommentEnd = group.End()
		}
	}
	if lastCommentEnd == token.NoPos {
		return
	}

	// Removing the only statement leaves the block's right brace at the old
	// statement line. gofmt preserves that position as a blank line after the
	// placeholder comments, so collapse the brace back to the final in-body
	// comment. This keeps user comments while making add/remove a clean round
	// trip for the default module template.
	body.Rbrace = lastCommentEnd
}

func callExprFromStmt(stmt ast.Stmt) (*ast.CallExpr, bool) {
	exprStmt, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return nil, false
	}
	call, ok := exprStmt.X.(*ast.CallExpr)
	return call, ok
}

func isRegisterCall(call *ast.CallExpr, alias string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Register" || len(call.Args) != 0 {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == alias
}
