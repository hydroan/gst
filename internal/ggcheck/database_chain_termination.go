package ggcheck

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
	"github.com/hydroan/gst/internal/goast"
)

// databaseTerminalMethods lists the gst.Database methods that finish an
// operation chain. TestDatabaseChainMethodSetsMatchDatabaseInterfaces guards this
// set against drifting from the interface declaration.
var databaseTerminalMethods = map[string]bool{
	"Create":     true,
	"Delete":     true,
	"Update":     true,
	"Upsert":     true,
	"UpdateByID": true,
	"List":       true,
	"Get":        true,
	"First":      true,
	"Last":       true,
	"Take":       true,
	"Count":      true,
}

// databaseChainMethods lists the gst.DatabaseOption methods that keep the
// chain open. TestDatabaseChainMethodSetsMatchDatabaseInterfaces guards this set
// against drifting from the interface declaration.
var databaseChainMethods = map[string]bool{
	"WithQuery":      true,
	"WithCursor":     true,
	"WithSelect":     true,
	"WithLock":       true,
	"WithBatchSize":  true,
	"WithPagination": true,
	"WithLimit":      true,
	"WithOrder":      true,
	"WithExpand":     true,
	"WithPurge":      true,
	"WithDeleted":    true,
	"WithReplica":    true,
	"WithDryRun":     true,
	"WithoutHook":    true,
}

// DatabaseChainTermination requires every database.Database chain to end in a
// terminal operation.
var DatabaseChainTermination = Check{
	Name: "Database chain termination",
	Rule: "database.Database operation chains must end with a terminal operation inline or be passed directly as a call argument",
	run:  checkDatabaseChainTermination,
}

// checkDatabaseChainTermination checks that every database.Database operation
// chain either ends with a terminal operation inline or is handed directly to
// a helper as a call argument. Database implementations share an underlying
// GORM session, so storing the chain value in a variable and running
// operations later is incorrect usage; each independent operation must start
// with its own database.Database call.
func checkDatabaseChainTermination(ignore gitignore.Matcher) []string {
	var violations []string

	err := walkProjectDir(".", ignore, func(path string, info os.FileInfo) error {
		if info.IsDir() {
			if path == "." {
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

		violations = append(violations, checkFileDatabaseChains(path)...)
		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking project directory: %v", err))
	}

	return violations
}

// checkFileDatabaseChains reports database.Database chains in one file that do
// not end with a terminal operation inline.
func checkFileDatabaseChains(filePath string) []string {
	dbNames, ok := gstDatabaseImportNames(filePath)
	if !ok {
		return nil
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil
	}

	parents := nodeParents(file)
	relPath := relativePath(filePath)

	var violations []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isDatabaseChainStart(call, dbNames) {
			return true
		}
		if message, bad := databaseChainViolation(call, parents); bad {
			pos := fset.Position(call.Pos())
			violations = append(violations, fmt.Sprintf("%s:%d: %s", relPath, pos.Line, message))
		}
		return true
	})

	return violations
}

// isDatabaseChainStart reports whether call is a generic database.Database
// instantiation call that starts an operation chain, under the names dbNames
// resolves.
func isDatabaseChainStart(call *ast.CallExpr, dbNames goast.PackageNames) bool {
	switch fun := call.Fun.(type) {
	case *ast.IndexExpr:
		return dbNames.Refers(fun.X, "Database")
	case *ast.IndexListExpr:
		return dbNames.Refers(fun.X, "Database")
	}
	return false
}

// databaseChainViolation follows the method chain outward from a
// database.Database call and reports how the chain violates the
// terminate-inline rule, if it does.
func databaseChainViolation(anchor *ast.CallExpr, parents map[ast.Node]ast.Node) (string, bool) {
	node := ast.Node(anchor)
	for {
		parent := parents[node]
		if paren, ok := parent.(*ast.ParenExpr); ok {
			node = paren
			continue
		}

		sel, ok := parent.(*ast.SelectorExpr)
		if !ok || sel.X != node {
			break
		}
		call, ok := parents[sel].(*ast.CallExpr)
		if !ok || call.Fun != sel {
			// The method value escapes, e.g. consume(db.List).
			break
		}

		if databaseChainMethods[sel.Sel.Name] {
			node = call
			continue
		}
		// A terminal method finishes the chain. Methods unknown to both sets
		// are treated as terminal so interface additions cannot produce false
		// positives before the method sets catch up.
		return "", false
	}

	switch parent := parents[node].(type) {
	case *ast.ExprStmt:
		return "database.Database chain never calls a terminal operation such as List, Get, or Create", true
	case *ast.CallExpr:
		// Handing the chain directly to a helper as a call argument is
		// allowed; the helper is responsible for running one terminal
		// operation on it, e.g. dao.ModelExists(database.Database[M](ctx), id, dest).
		if expr, ok := node.(ast.Expr); ok && slices.Contains(parent.Args, expr) {
			return "", false
		}
	}
	return "database.Database chain value must not be stored or returned; end the chain with a terminal operation inline or pass it directly as a call argument, and call database.Database again for each independent operation", true
}

// nodeParents maps every AST node in file to its parent node.
func nodeParents(file *ast.File) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var stack []ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) > 0 {
			parents[n] = stack[len(stack)-1]
		}
		stack = append(stack, n)
		return true
	})
	return parents
}
