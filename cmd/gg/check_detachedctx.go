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

// CheckDetachedContext checks that in service, dao, cronjob, leader and lock
// code, the context passed to a framework database entry point or to a
// function of the project's dao packages is never context.Background() or
// context.TODO(). The context handed down to that code carries the request's
// or the round's identity for every log line and statement, the transaction
// the work may already be in, and the lease behind cluster-once work; a
// detached context loses all three, so a transaction opened on it neither
// joins the enclosing one nor stops when the lease is lost. Startup seeding,
// which has no context to inherit, belongs to the module or router packages
// outside these directories and passes its context down from there.
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

// checkFileDetachedContexts reports the calls in one file that hand a
// detached context to a database entry point or a dao function.
func checkFileDetachedContexts(filePath, modulePath string) []string {
	contextAliases, _, hasContext := importNamesOf(filePath, "context", "context")
	if !hasContext {
		return nil
	}
	dbAliases, dbDot, _ := gstDatabaseImportNames(filePath)
	daoAliases := daoImportNames(filePath, modulePath)
	if len(dbAliases) == 0 && !dbDot && len(daoAliases) == 0 {
		return nil
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil
	}
	relPath := relativePath(filePath)

	var violations []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee, ok := entryPointName(call, dbAliases, dbDot, daoAliases)
		if !ok {
			return true
		}
		for _, arg := range call.Args {
			if detached, ok := detachedContextCall(arg, contextAliases); ok {
				pos := fset.Position(arg.Pos())
				violations = append(violations, fmt.Sprintf(
					"%s:%d: %s receives %s; pass the context handed down to this code — it carries the request's or the round's identity, the transaction and the lease — or take one as a parameter",
					relPath, pos.Line, callee, detached,
				))
			}
		}
		return true
	})
	return violations
}

// daoImportNames returns the local names under which filePath imports a
// package of the project's dao tree.
func daoImportNames(filePath, modulePath string) []string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.ImportsOnly)
	if err != nil {
		return nil
	}

	daoPath := modulePath + "/dao"
	var names []string
	for _, spec := range file.Imports {
		path := strings.Trim(spec.Path.Value, `"`)
		if path != daoPath && !strings.HasPrefix(path, daoPath+"/") {
			continue
		}
		name := path[strings.LastIndex(path, "/")+1:]
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name == "_" || name == "." {
			continue
		}
		names = append(names, name)
	}
	return names
}

// entryPointName reports whether call is a framework database entry point
// or a dao function, naming it for the violation. A generic instantiation
// such as database.Database[*model.Record](ctx) wraps the selector in an
// index expression.
func entryPointName(call *ast.CallExpr, dbAliases []string, dbDot bool, daoAliases []string) (string, bool) {
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
		if slices.Contains(dbAliases, ident.Name) {
			return "database." + f.Sel.Name, true
		}
		if slices.Contains(daoAliases, ident.Name) {
			return ident.Name + "." + f.Sel.Name, true
		}
	case *ast.Ident:
		if dbDot && slices.Contains(databaseEntryPoints, f.Name) {
			return "database." + f.Name, true
		}
	}
	return "", false
}

// detachedContextCall reports whether arg is context.Background() or
// context.TODO(), naming it.
func detachedContextCall(arg ast.Expr, contextAliases []string) (string, bool) {
	call, ok := arg.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return "", false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok || !slices.Contains(contextAliases, ident.Name) {
		return "", false
	}
	if sel.Sel.Name != "Background" && sel.Sel.Name != "TODO" {
		return "", false
	}
	return "context." + sel.Sel.Name + "()", true
}
