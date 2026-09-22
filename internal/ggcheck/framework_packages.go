// What the checks know about the framework's own packages: their import
// paths, the names a project file refers to them by, and the database
// functions and context derivations the context checks follow.

package ggcheck

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/hydroan/gst/internal/goast"
)

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
