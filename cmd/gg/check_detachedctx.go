package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	gitignore "github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/goast"
)

// detachedContextDirs are the project directories whose code runs under a
// context handed down to it — a request's, a round's, a tenure's, a lock's,
// the process's for a component, the start's for a routes-ready hook — and
// must pass that context on to the database.
var detachedContextDirs = []string{"service", "dao", "cronjob", "leader", "lock", "component", "router"}

// databaseEntryPoints are the framework database functions that take a
// context, for the file that dot-imports the package and calls them without
// a qualifier; with a qualifier every function of the package counts. The
// test of this rule pins the list to the package.
var databaseEntryPoints = []string{
	"AfterCommit",
	"Cleanup", "CleanupOn",
	"Database", "DatabaseOn",
	"Health", "HealthOn",
	"Select", "SelectOn",
	"Transaction", "TransactionOn",
	"UnionAll", "UnionAllOn",
}

// contextDerivations are the context functions that derive a context from
// their first argument: a context derived from a detached one is detached.
var contextDerivations = []string{
	"WithCancel", "WithCancelCause",
	"WithDeadline", "WithDeadlineCause",
	"WithTimeout", "WithTimeoutCause",
	"WithValue", "WithoutCancel",
}

// CheckDetachedContext checks that in service, dao, cronjob, leader, lock,
// component and router code, the context passed to a framework database entry point
// or to a function of the project's dao packages is never
// context.Background() or context.TODO() — written at the call, held in a
// local variable first, or wrapped in a context derivation such as
// context.WithTimeout. The context handed down to that code carries the
// request's or the round's identity for every log line and statement, the
// transaction the work may already be in, and the lease behind cluster-once
// work; a detached context loses all three, so a transaction opened on it
// neither joins the enclosing one nor stops when the lease is lost. Startup
// seeding runs in the router package's routes-ready hooks, on the context
// the hook receives, and is checked like the rest.
//
// The check is syntactic. Variables are resolved by function scope: a name
// declared in a closure is the closure's, a parameter is the function's,
// and the last value a variable receives, in source order, is the one that
// counts — a variable that received a detached context and then a real one
// is left alone, a parameter overwritten with a detached context is not. A
// detached context that reaches a call through a function parameter, a
// struct field or a package variable is beyond it, as is a shadowing
// declaration inside an if, for or switch block. A dao package imported
// under a dot is reported as such: its calls cannot be told apart.
//
// Code of copyable framework modules under the service directory is skipped:
// it is checked inside the framework.
func CheckDetachedContext(ignore gitignore.Matcher) []string {
	modulePath, err := gen.GetModulePath()
	if err != nil {
		return []string{fmt.Sprintf("reading the module path: %v", err)}
	}
	owned, err := copyableModuleOwners()
	if err != nil {
		return []string{fmt.Sprintf("listing copyable modules: %v", err)}
	}

	var violations []string
	for _, dir := range detachedContextDirs {
		if _, statErr := os.Stat(dir); statErr != nil {
			continue
		}
		walkErr := walkProjectDir(dir, ignore, func(path string, info os.FileInfo) error {
			if info.IsDir() {
				if path == dir {
					return nil
				}
				base := filepath.Base(path)
				if strings.HasPrefix(base, ".") || base == "vendor" || base == "testdata" {
					return filepath.SkipDir
				}
				// Nested Go modules belong to other projects.
				if _, statErr := os.Stat(filepath.Join(path, "go.mod")); statErr == nil {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || isGeneratedFileName(path) {
				return nil
			}
			if dir == serviceDir && moduleOwnedPath(owned, serviceDir, path) {
				return nil
			}
			violations = append(violations, checkFileDetachedContexts(path, modulePath)...)
			return nil
		})
		if walkErr != nil {
			violations = append(violations, fmt.Sprintf("walking %s: %v", dir, walkErr))
		}
	}
	return violations
}

// contextImports are the names one file knows the context package, the
// framework database package and the project's dao packages by.
type contextImports struct {
	context  goast.PackageNames
	database goast.PackageNames
	// dao holds the qualifiers of every dao package the file imports.
	dao []string
	// daoDot lists the dao packages imported under a dot, whose calls the
	// check cannot tell apart.
	daoDot []*ast.ImportSpec
}

// checkFileDetachedContexts reports the calls in one file that hand a
// detached context to a database entry point or a dao function.
func checkFileDetachedContexts(filePath, modulePath string) []string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil
	}
	imports := contextImportsOf(file, modulePath)
	relPath := relativePath(filePath)
	var violations []string
	for _, spec := range imports.daoDot {
		pos := fset.Position(spec.Pos())
		violations = append(violations, fmt.Sprintf(
			"%s:%d: dot-imports %s; the detached context check cannot tell its calls apart, import it under a name",
			relPath, pos.Line, spec.Path.Value,
		))
	}
	if len(imports.context.Qualifiers) == 0 && !imports.context.DotImported {
		return violations
	}
	if len(imports.database.Qualifiers) == 0 && !imports.database.DotImported && len(imports.dao) == 0 {
		return violations
	}

	report := func(callee string, origin string, arg ast.Expr) {
		pos := fset.Position(arg.Pos())
		violations = append(violations, fmt.Sprintf(
			"%s:%d: %s receives %s; pass the context handed down to this code — it carries the request's or the round's identity, the transaction and the lease — or take one as a parameter",
			relPath, pos.Line, callee, origin,
		))
	}
	for _, decl := range file.Decls {
		// Each declaration is resolved on its own: the variables a function
		// declares are known to that function's calls and to the closures
		// inside it, and to nothing else.
		scope := newContextScope(nil)
		var body ast.Node = decl
		if fn, ok := decl.(*ast.FuncDecl); ok {
			if fn.Body == nil {
				continue
			}
			body = fn.Body
			scope.declareParams(fn.Recv, fn.Type)
		}
		scopes := make(map[*ast.FuncLit]*contextScope)
		collectContextNames(body, scope, scopes, imports)
		checkDetachedCalls(body, scope, scopes, imports, report)
	}
	return violations
}

// contextImportsOf reads the names file knows the context package, the
// framework database package and the project's dao packages by. A dao
// package imported without an alias goes by its package clause.
func contextImportsOf(file *ast.File, modulePath string) contextImports {
	imports := contextImports{
		context:  goast.ImportedNames(file, "context", "context"),
		database: goast.ImportedNames(file, gstDatabaseImportPath, "database"),
	}
	daoPath := modulePath + "/dao"
	resolved := make(map[string]bool)
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || (path != daoPath && !strings.HasPrefix(path, daoPath+"/")) {
			continue
		}
		if spec.Name != nil && spec.Name.Name == "." {
			imports.daoDot = append(imports.daoDot, spec)
		}
		if resolved[path] {
			continue
		}
		resolved[path] = true
		names := goast.ImportedNames(file, path, packageNameOf(strings.TrimPrefix(path, modulePath+"/")))
		imports.dao = append(imports.dao, names.Qualifiers...)
	}
	return imports
}

// packageNameOf reads the package clause of the project directory dir — the
// name an import without an alias is used under — and falls back to the
// directory's name when no source is there to read.
func packageNameOf(dir string) string {
	sources, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), source, nil, parser.PackageClauseOnly)
		if err != nil || file.Name == nil {
			continue
		}
		return file.Name.Name
	}
	return filepath.Base(dir)
}

// contextScope is what one function — a declaration or a closure — knows
// about the variables declared in it: which hold a detached context by
// their last assignment, and which hold something else. A name is looked up
// from the innermost scope outward, the way Go resolves it, so a closure's
// ctx is not its enclosing function's.
type contextScope struct {
	parent   *contextScope
	detached map[string]bool
	other    map[string]bool
}

func newContextScope(parent *contextScope) *contextScope {
	return &contextScope{parent: parent, detached: make(map[string]bool), other: make(map[string]bool)}
}

// declareParams records a function's receiver and parameters as variables
// holding something other than a detached context.
func (s *contextScope) declareParams(recv *ast.FieldList, typ *ast.FuncType) {
	for _, list := range []*ast.FieldList{recv, typ.Params, typ.Results} {
		if list == nil {
			continue
		}
		for _, field := range list.List {
			for _, name := range field.Names {
				s.declare(name.Name, false)
			}
		}
	}
}

// declare records a variable declared in this scope and whether the value it
// is declared with is a detached context; a later declaration or
// assignment of the same name replaces what the earlier one recorded, so
// the last value in source order is the one that counts.
func (s *contextScope) declare(name string, detached bool) {
	if name == "_" {
		return
	}
	if detached {
		s.detached[name] = true
		delete(s.other, name)
	} else {
		s.other[name] = true
		delete(s.detached, name)
	}
}

// assign records a value assigned to an existing variable, in the scope that
// declares it; an assignment to a name no scope declares — a package
// variable, a field — is out of reach and ignored.
func (s *contextScope) assign(name string, detached bool) {
	for scope := s; scope != nil; scope = scope.parent {
		if scope.detached[name] || scope.other[name] {
			scope.declare(name, detached)
			return
		}
	}
}

// isDetached reports whether name, resolved from this scope outward, holds
// a detached context by its last assignment.
func (s *contextScope) isDetached(name string) bool {
	for scope := s; scope != nil; scope = scope.parent {
		if scope.detached[name] || scope.other[name] {
			return scope.detached[name]
		}
	}
	return false
}

// collectContextNames walks node, recording in scope every variable it
// declares or assigns and what it receives, and opening a scope of its own
// for every closure, remembered in scopes for the check that follows.
func collectContextNames(node ast.Node, scope *contextScope, scopes map[*ast.FuncLit]*contextScope, imports contextImports) {
	ast.Inspect(node, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.FuncLit:
			inner := newContextScope(scope)
			inner.declareParams(nil, stmt.Type)
			scopes[stmt] = inner
			collectContextNames(stmt.Body, inner, scopes, imports)
			return false
		case *ast.AssignStmt:
			record := func(target ast.Expr, value ast.Expr) {
				ident, ok := target.(*ast.Ident)
				if !ok {
					return
				}
				_, detached := detachedContext(value, imports, scope)
				if stmt.Tok == token.DEFINE {
					scope.declare(ident.Name, detached)
				} else {
					scope.assign(ident.Name, detached)
				}
			}
			switch {
			case len(stmt.Lhs) == len(stmt.Rhs):
				for i, rhs := range stmt.Rhs {
					record(stmt.Lhs[i], rhs)
				}
			case len(stmt.Rhs) == 1:
				// ctx, cancel := context.WithTimeout(...): the context is
				// the first value; the others are not contexts.
				record(stmt.Lhs[0], stmt.Rhs[0])
				for _, target := range stmt.Lhs[1:] {
					if ident, ok := target.(*ast.Ident); ok && stmt.Tok == token.DEFINE {
						scope.declare(ident.Name, false)
					}
				}
			}
		case *ast.ValueSpec:
			// var ctx = context.Background() declares a detached context; a
			// declaration without a value, or of the second name of a
			// multi-value call, declares something else.
			for i, name := range stmt.Names {
				detached := false
				if i < len(stmt.Values) {
					_, detached = detachedContext(stmt.Values[i], imports, scope)
				}
				scope.declare(name.Name, detached)
			}
		case *ast.RangeStmt:
			if stmt.Tok == token.DEFINE {
				for _, target := range []ast.Expr{stmt.Key, stmt.Value} {
					if ident, ok := target.(*ast.Ident); ok {
						scope.declare(ident.Name, false)
					}
				}
			}
		}
		return true
	})
}

// checkDetachedCalls walks node with the scopes collectContextNames built
// and reports every entry point call whose argument is a detached context.
func checkDetachedCalls(node ast.Node, scope *contextScope, scopes map[*ast.FuncLit]*contextScope, imports contextImports, report func(callee, origin string, arg ast.Expr)) {
	ast.Inspect(node, func(n ast.Node) bool {
		switch expr := n.(type) {
		case *ast.FuncLit:
			checkDetachedCalls(expr.Body, scopes[expr], scopes, imports, report)
			return false
		case *ast.CallExpr:
			callee, ok := entryPointName(expr, imports)
			if !ok {
				return true
			}
			for _, arg := range expr.Args {
				if origin, ok := detachedContext(arg, imports, scope); ok {
					report(callee, origin, arg)
				}
			}
		}
		return true
	})
}

// entryPointName reports whether call is a framework database entry point
// or a dao function, naming it for the violation. A generic instantiation
// such as database.Database[*model.Record](ctx) wraps the selector in an
// index expression.
func entryPointName(call *ast.CallExpr, imports contextImports) (string, bool) {
	fun := call.Fun
	switch generic := fun.(type) {
	case *ast.IndexExpr:
		fun = generic.X
	case *ast.IndexListExpr:
		fun = generic.X
	}

	switch f := fun.(type) {
	case *ast.SelectorExpr:
		ident, ok := f.X.(*ast.Ident)
		if !ok || f.Sel == nil {
			return "", false
		}
		if slices.Contains(imports.database.Qualifiers, ident.Name) {
			return "database." + f.Sel.Name, true
		}
		if slices.Contains(imports.dao, ident.Name) {
			return ident.Name + "." + f.Sel.Name, true
		}
	case *ast.Ident:
		if imports.database.Refers(f, databaseEntryPoints...) {
			return "database." + f.Name, true
		}
	}
	return "", false
}

// detachedContext reports whether expr is a detached context — a call to
// context.Background() or context.TODO(), a derivation of one through the
// context package, or a variable scope resolves to one — and names its
// origin. A nil scope resolves no variable.
func detachedContext(expr ast.Expr, imports contextImports, scope *contextScope) (string, bool) {
	switch e := expr.(type) {
	case *ast.Ident:
		if scope != nil && scope.isDetached(e.Name) {
			return "a context held in " + e.Name, true
		}
	case *ast.ParenExpr:
		return detachedContext(e.X, imports, scope)
	case *ast.CallExpr:
		name, ok := contextFunctionName(e.Fun, imports)
		if !ok {
			return "", false
		}
		switch {
		case (name == "Background" || name == "TODO") && len(e.Args) == 0:
			return "context." + name + "()", true
		case slices.Contains(contextDerivations, name) && len(e.Args) > 0:
			if origin, ok := detachedContext(e.Args[0], imports, scope); ok {
				return origin + " through context." + name, true
			}
		}
	}
	return "", false
}

// contextFunctionName returns the name of the context package function fun
// refers to, under a qualifier or a dot import.
func contextFunctionName(fun ast.Expr, imports contextImports) (string, bool) {
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		ident, ok := f.X.(*ast.Ident)
		if !ok || f.Sel == nil || !slices.Contains(imports.context.Qualifiers, ident.Name) {
			return "", false
		}
		return f.Sel.Name, true
	case *ast.Ident:
		if imports.context.DotImported {
			return f.Name, true
		}
	}
	return "", false
}
