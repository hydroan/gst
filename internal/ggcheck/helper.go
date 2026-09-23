// The helpers the checks share: walking the project by its Git ignore rules,
// the files gg generates and owns, the subtrees gg module copy writes, the
// explicit DSL Payload and Result calls, and what the checks know about the
// framework's own packages. No check lives here; every check has a file of
// its own.

package ggcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/ggmodule"
	"github.com/hydroan/gst/internal/goast"
)

// isGeneratedFileName reports whether a path is a file gg generates and owns.
func isGeneratedFileName(path string) bool {
	return strings.HasSuffix(path, ggconst.SuffixGenGo)
}

// copyableModuleOwners returns the first path segments under the model and
// service directories owned by copyable framework modules: gg module copy
// writes module code to model/<module>/... and service/<module>/... subtrees.
func copyableModuleOwners() (map[string]bool, error) {
	names, err := ggmodule.CopyableModuleNames()
	if err != nil {
		return nil, err
	}
	owned := make(map[string]bool, len(names))
	for _, name := range names {
		owned[name] = true
	}
	return owned, nil
}

// moduleOwnedPath reports whether a path below root falls under a subtree
// owned by a copyable framework module.
func moduleOwnedPath(owned map[string]bool, root, path string) bool {
	if len(owned) == 0 {
		return false
	}
	rel, err := filepath.Rel(root, filepath.Clean(path))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	first, _, _ := strings.Cut(rel, string(filepath.Separator))
	return owned[first]
}

// excludedDir reports whether a check walking the project from root leaves
// out the directory at path and everything below it: a hidden directory, a
// vendor or testdata directory, or a directory holding a go.mod of its own,
// whose code belongs to another module. The root itself is never left out.
// Walking from ".", "." and "service" stay in, while ".git",
// "service/testdata" and "tools", holding tools/go.mod, are left out.
func excludedDir(root, path string) bool {
	if path == root {
		return false
	}
	base := filepath.Base(path)
	if strings.HasPrefix(base, ".") || base == ggconst.DirVendor || base == ggconst.DirTestData {
		return true
	}
	_, err := os.Stat(filepath.Join(path, "go.mod"))
	return err == nil
}

// localActionTypeName resolves a DSL type argument to a type name declared in
// the same package. Pointer forms are unwrapped; qualified names from other
// packages are not resolved.
func localActionTypeName(expr ast.Expr) (string, bool) {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name, true
	case *ast.StarExpr:
		return localActionTypeName(x.X)
	}
	return "", false
}

// dslActionTypeCall returns the kind and type argument for DSL Payload/Result calls.
func dslActionTypeCall(expr ast.Expr) (string, ast.Expr, bool) {
	switch x := expr.(type) {
	case *ast.IndexExpr:
		if kind, ok := dslActionTypeName(x.X); ok {
			return kind, x.Index, true
		}
	case *ast.IndexListExpr:
		if len(x.Indices) == 1 {
			if kind, ok := dslActionTypeName(x.X); ok {
				return kind, x.Indices[0], true
			}
		}
	}
	return "", nil, false
}

// dslActionTypeName returns the DSL function name for Payload or Result.
func dslActionTypeName(expr ast.Expr) (string, bool) {
	switch x := expr.(type) {
	case *ast.Ident:
		if x.Name == "Payload" || x.Name == "Result" {
			return x.Name, true
		}
	case *ast.SelectorExpr:
		if x.Sel != nil && (x.Sel.Name == "Payload" || x.Sel.Name == "Result") {
			return x.Sel.Name, true
		}
	}
	return "", false
}

// actionTypeBaseName extracts the named type from supported DSL type arguments.
func actionTypeBaseName(expr ast.Expr) (string, bool) {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name, true
	case *ast.StarExpr:
		return actionTypeBaseName(x.X)
	case *ast.SelectorExpr:
		if x.Sel != nil {
			return x.Sel.Name, true
		}
	}
	return "", false
}

// importedNamesOf parses only the imports of filePath and reports how the
// file refers to the package at importPath (see goast.ImportedNames), and
// whether it imports the package at all: a file that does not import it
// needs no full parse.
func importedNamesOf(filePath, importPath, defaultName string) (goast.PackageNames, bool) {
	file, err := parser.ParseFile(token.NewFileSet(), filePath, nil, parser.ImportsOnly)
	if err != nil {
		return goast.PackageNames{}, false
	}
	return goast.ImportedNames(file, importPath, defaultName), goast.FindImportSpec(file, importPath) != nil
}

// gstDatabaseImportPath is the framework package whose Database function
// starts a model-scoped operation chain.
const gstDatabaseImportPath = "github.com/hydroan/gst/database"

// gstDatabaseImportNames returns how filePath refers to the framework database
// package, and whether it imports the package at all. It parses imports only,
// so files that do not use the package stay cheap to scan.
func gstDatabaseImportNames(filePath string) (goast.PackageNames, bool) {
	return importedNamesOf(filePath, gstDatabaseImportPath, "database")
}

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

// gstImportPath is the framework package declaring ServiceContext, whose
// SSE method is a sanctioned error exit: its errors are framework-governed —
// a setup failure carries a framework-built message, and an error after the
// stream opened never reaches the response envelope at all.
const gstImportPath = "github.com/hydroan/gst"
