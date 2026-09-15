package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"

	gitignore "github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/hydroan/gst/internal/codegen/gen"
)

// detachedContextDirs are the project directories whose code runs under a
// context handed down to it — a request's, a round's, a tenure's, a lock's —
// and must pass that context on to the database.
var detachedContextDirs = []string{"service", "dao", "cronjob", "leader", "lock"}

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

// CheckDetachedContext checks that in service, dao, cronjob, leader and lock
// code, the context passed to a framework database entry point or to a
// function of the project's dao packages is never context.Background() or
// context.TODO() — written at the call, held in a local variable first, or
// wrapped in a context derivation such as context.WithTimeout. The context
// handed down to that code carries the request's or the round's identity
// for every log line and statement, the transaction the work may already be
// in, and the lease behind cluster-once work; a detached context loses all
// three, so a transaction opened on it neither joins the enclosing one nor
// stops when the lease is lost. Startup seeding, which has no context to
// inherit, belongs to the module or router packages outside these
// directories and passes its context down from there.
//
// The check is syntactic: a detached context that reaches a call through a
// function parameter, a struct field or a package variable is beyond it.
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
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
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

// contextImports are the local names one file knows the context package,
// the framework database package and the project's dao packages by.
type contextImports struct {
	context    []string
	contextDot bool
	database   []string
	dbDot      bool
	dao        []string
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
	if len(imports.context) == 0 && !imports.contextDot {
		return nil
	}
	if len(imports.database) == 0 && !imports.dbDot && len(imports.dao) == 0 {
		return nil
	}
	relPath := relativePath(filePath)

	var violations []string
	for _, decl := range file.Decls {
		// Each function body is inspected on its own, so that a context a
		// function holds in a local variable is known to the calls of that
		// function alone; the closures inside it see the same variables.
		var body ast.Node = decl
		if fn, ok := decl.(*ast.FuncDecl); ok {
			if fn.Body == nil {
				continue
			}
			body = fn.Body
		}
		detached := detachedNames(body, imports)
		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee, ok := entryPointName(call, imports)
			if !ok {
				return true
			}
			for _, arg := range call.Args {
				if origin, ok := detachedContext(arg, imports, detached); ok {
					pos := fset.Position(arg.Pos())
					violations = append(violations, fmt.Sprintf(
						"%s:%d: %s receives %s; pass the context handed down to this code — it carries the request's or the round's identity, the transaction and the lease — or take one as a parameter",
						relPath, pos.Line, callee, origin,
					))
				}
			}
			return true
		})
	}
	return violations
}

// contextImportsOf reads the local names file imports the context package,
// the framework database package and the project's dao packages under.
func contextImportsOf(file *ast.File, modulePath string) contextImports {
	var imports contextImports
	daoPath := modulePath + "/dao"
	for _, spec := range file.Imports {
		path := strings.Trim(spec.Path.Value, `"`)
		alias := ""
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		if alias == "_" {
			continue
		}
		switch {
		case path == "context":
			if alias == "." {
				imports.contextDot = true
			} else {
				imports.context = append(imports.context, localName(alias, "context"))
			}
		case path == gstDatabaseImportPath:
			if alias == "." {
				imports.dbDot = true
			} else {
				imports.database = append(imports.database, localName(alias, "database"))
			}
		case path == daoPath || strings.HasPrefix(path, daoPath+"/"):
			if alias == "." {
				continue
			}
			imports.dao = append(imports.dao, localName(alias, packageNameOf(strings.TrimPrefix(path, modulePath+"/"))))
		}
	}
	return imports
}

// localName returns the name an import is used under: its alias, or the
// package's own name.
func localName(alias, packageName string) string {
	if alias != "" {
		return alias
	}
	return packageName
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

// detachedNames collects the local variables a body assigns a detached
// context to — ctx := context.Background(), var ctx = context.TODO(), or a
// derivation of either — and never anything else: a variable that also
// receives a context from elsewhere is left alone, because the check reads
// the syntax, not the flow.
func detachedNames(body ast.Node, imports contextImports) map[string]bool {
	detached := make(map[string]bool)
	other := make(map[string]bool)
	record := func(target ast.Expr, value ast.Expr) {
		ident, ok := target.(*ast.Ident)
		if !ok || ident.Name == "_" {
			return
		}
		if _, ok := detachedContext(value, imports, nil); ok {
			detached[ident.Name] = true
		} else {
			other[ident.Name] = true
		}
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			switch {
			case len(stmt.Lhs) == len(stmt.Rhs):
				for i, rhs := range stmt.Rhs {
					record(stmt.Lhs[i], rhs)
				}
			case len(stmt.Rhs) == 1:
				// ctx, cancel := context.WithTimeout(...): the context is
				// the first value.
				record(stmt.Lhs[0], stmt.Rhs[0])
			}
		case *ast.ValueSpec:
			switch {
			case len(stmt.Names) == len(stmt.Values):
				for i, value := range stmt.Values {
					record(stmt.Names[i], value)
				}
			case len(stmt.Values) == 1:
				record(stmt.Names[0], stmt.Values[0])
			}
		}
		return true
	})
	for name := range other {
		delete(detached, name)
	}
	return detached
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
		if slices.Contains(imports.database, ident.Name) {
			return "database." + f.Sel.Name, true
		}
		if slices.Contains(imports.dao, ident.Name) {
			return ident.Name + "." + f.Sel.Name, true
		}
	case *ast.Ident:
		if imports.dbDot && slices.Contains(databaseEntryPoints, f.Name) {
			return "database." + f.Name, true
		}
	}
	return "", false
}

// detachedContext reports whether expr is a detached context — a call to
// context.Background() or context.TODO(), a derivation of one through the
// context package, or a variable named in detached — and names its origin.
func detachedContext(expr ast.Expr, imports contextImports, detached map[string]bool) (string, bool) {
	switch e := expr.(type) {
	case *ast.Ident:
		if detached[e.Name] {
			return "a context held in " + e.Name, true
		}
	case *ast.ParenExpr:
		return detachedContext(e.X, imports, detached)
	case *ast.CallExpr:
		name, ok := contextFunctionName(e.Fun, imports)
		if !ok {
			return "", false
		}
		switch {
		case (name == "Background" || name == "TODO") && len(e.Args) == 0:
			return "context." + name + "()", true
		case slices.Contains(contextDerivations, name) && len(e.Args) > 0:
			if origin, ok := detachedContext(e.Args[0], imports, detached); ok {
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
		if !ok || f.Sel == nil || !slices.Contains(imports.context, ident.Name) {
			return "", false
		}
		return f.Sel.Name, true
	case *ast.Ident:
		if imports.contextDot {
			return f.Name, true
		}
	}
	return "", false
}
