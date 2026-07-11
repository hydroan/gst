package ggmodule

import (
	"bytes"
	"fmt"
	"go/ast"
	goformat "go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	gofumpt "mvdan.cc/gofumpt/format"
)

// ChangeStatus describes whether a module command changed module/module.go.
type ChangeStatus string

const (
	ChangeCreated ChangeStatus = "created"
	ChangeRemoved ChangeStatus = "removed"
	ChangeSkipped ChangeStatus = "skipped"
)

// ChangeResult is returned by add/remove commands so the Cobra layer can decide
// how to present the operation without knowing AST details.
type ChangeResult struct {
	Module Module
	Status ChangeStatus
	Path   string
}

// moduleForRegistration applies the command-level constraints shared by add and
// remove. A module can be listed even when it is not addable, but automatic
// registration only works when the framework can be called as pkg.Register().
func moduleForRegistration(name string) (Module, error) {
	if err := validateModuleName(name); err != nil {
		return Module{}, err
	}
	module, err := moduleByName(name)
	if os.IsNotExist(err) {
		return Module{}, fmt.Errorf("module %q not found", name)
	}
	if err != nil {
		return Module{}, err
	}
	if !module.Addable {
		return Module{}, fmt.Errorf("module %q cannot be added automatically because Register requires arguments", name)
	}
	return module, nil
}

// validateModuleName keeps add/remove on catalog entries instead of arbitrary
// filesystem paths. This avoids ambiguous commands such as `gg module add
// module/copytest` and prevents path traversal from reaching outside the module
// catalog.
func validateModuleName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("module name is required")
	}
	if name != strings.TrimSpace(name) {
		return fmt.Errorf("module name %q must not contain surrounding whitespace", name)
	}
	if strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("module command accepts a module name, not a path: %s", name)
	}
	if filepath.Clean(name) != name || filepath.Base(name) != name {
		return fmt.Errorf("module command accepts a module name, not a path: %s", name)
	}
	return nil
}

func projectModuleFile(projectDir string) string {
	return filepath.Join(projectDir, "module", "module.go")
}

func parseGoFile(path string) (*token.FileSet, *ast.File, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	return fset, file, err
}

// existingModuleAlias returns the qualifier already used for this framework
// import, if the project has one. AddModule uses it for half-manual edits such
// as an import without a Register call, so the generated call still compiles.
func existingModuleAlias(file *ast.File, module Module) (string, bool) {
	for _, spec := range file.Imports {
		alias, ok := moduleImportAlias(spec, module)
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
		alias, ok := moduleImportAlias(spec, module)
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

func writeGoFile(path string, fset *token.FileSet, file *ast.File) error {
	var buf bytes.Buffer
	if err := goformat.Node(&buf, fset, file); err != nil {
		return err
	}
	formatted, err := gofumpt.Source(buf.Bytes(), gofumpt.Options{})
	if err != nil {
		return err
	}
	if err := ensureParentDir(path); err != nil {
		return err
	}
	return os.WriteFile(path, formatted, 0o600)
}

func identName(ident *ast.Ident) string {
	if ident == nil {
		return ""
	}
	return ident.Name
}
