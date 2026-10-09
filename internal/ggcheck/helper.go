// The helpers the checks share: the files gg generates and owns, the subtrees
// gg module copy writes, the explicit DSL Payload and Result calls and the
// names type expressions end in, and what the checks know about imports and
// packages, the framework's own and the project's. No check lives here; every
// check has a file of its own.

package ggcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/ggmodule"
	"github.com/hydroan/gst/internal/goast"
	"github.com/hydroan/gst/internal/modelinfo"
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

// dslActionTypeName returns the DSL function name of an action type
// keyword, one taking an action type argument (see dsl.PayloadKeyword and
// dsl.ResultKeyword): Payload, Result, StreamingPayload or StreamingResult.
func dslActionTypeName(expr ast.Expr) (string, bool) {
	var name string
	switch x := expr.(type) {
	case *ast.Ident:
		name = x.Name
	case *ast.SelectorExpr:
		if x.Sel != nil {
			name = x.Sel.Name
		}
	}
	return name, dsl.PayloadKeyword(name) || dsl.ResultKeyword(name)
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

// actionTypeSide is one side of an action that declares a type: the DSL
// keyword declaring it, Payload, Result, StreamingPayload or StreamingResult,
// and the type as the declaration wrote it, *SampleRsp.
type actionTypeSide struct {
	kind string
	raw  string
}

// actionTypeSides returns the sides of action that declare a type, a Stream
// action's streaming side as StreamingPayload or StreamingResult; a side the
// framework fills in, *model.Empty for the payload of a List or Get (see
// dsl.PayloadEmpty), is left out.
func actionTypeSides(action *dsl.Action) []actionTypeSide {
	payloadKind, resultKind := "Payload", "Result"
	if action.StreamingPayload {
		payloadKind = "StreamingPayload"
	}
	if action.StreamingResult {
		resultKind = "StreamingResult"
	}
	sides := make([]actionTypeSide, 0, 2)
	for _, side := range []actionTypeSide{{kind: payloadKind, raw: action.Payload}, {kind: resultKind, raw: action.Result}} {
		if side.raw == "" || side.raw == dsl.PayloadEmpty {
			continue
		}
		sides = append(sides, side)
	}
	return sides
}

// typeBaseName returns the name a type expression ends in, the way a
// receiver, an embedded field or a DSL type argument names its type: Record
// for Record, *Record and model.Record. Any other form, such as a slice or a
// generic instantiation, names none.
func typeBaseName(expr ast.Expr) (string, bool) {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name, true
	case *ast.StarExpr:
		return typeBaseName(x.X)
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

// gstImportPath is the framework's root package. It declares the column
// constructors project code must not mint references with, and
// ServiceContext, whose SSE method is a sanctioned error exit: its errors are
// framework-governed — a setup failure carries a framework-built message, and
// an error after the stream opened never reaches the response envelope at
// all.
const gstImportPath = "github.com/hydroan/gst"

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

// errModelsRefused marks the error of a scan the model files stopped (see
// scanModels): the DSL design rules report such a fault file by file, so
// the checks reading the models say nothing of it.
var errModelsRefused = errors.New("the model files were refused")

// scanModels reads the models of the project the way gg gen reads them
// (see modelinfo.ScanModels), for the checks that read the models: gst.yaml
// for the ignore rules, go.mod for the module path, then the scan. An error
// names what stopped the scan, marked errModelsRefused when the model files
// did; the DSL design rules report it, once, and the other checks reading
// the models return nothing for it.
func scanModels(ignore gghelper.ProjectIgnore) (modelinfo.ScannedModels, error) {
	cfg, err := ggconfig.Load(".")
	if err != nil {
		return modelinfo.ScannedModels{}, errors.Wrap(err, "loading gst.yaml")
	}
	modulePath, err := gghelper.ModulePath()
	if err != nil {
		return modelinfo.ScannedModels{}, errors.Wrap(err, "reading the module path")
	}
	scanned, err := modelinfo.ScanModels(modulePath, ggconst.DirModel, ignore, cfg)
	if err != nil {
		return modelinfo.ScannedModels{}, errors.Mark(errors.Wrap(err, "scanning model designs"), errModelsRefused)
	}
	return scanned, nil
}

// varObj is the parser-resolved declaration object of a local variable.
// ast.Object is deprecated because syntactic resolution is ambiguous
// without type information (composite literal keys, selector fields); the
// checks resolve only plain local variables in assignments, returns and
// method calls, a subset the parser's lexical scoping gets right, and they
// must stay off go/types to keep gg gen fast. Every use of the deprecated
// API is confined to this alias and the two accessors below.
//
//nolint:staticcheck // SA1019: sound for the local-variable subset, see above.
type varObj = *ast.Object

// declObj returns the declaration object of an identifier, the single
// accessor for the deprecated field.
func declObj(ident *ast.Ident) varObj {
	return ident.Obj
}

// declNode returns the node declaring obj: a Field for a receiver or
// parameter, a ValueSpec for a var declaration, an AssignStmt for a short
// variable declaration, a FuncDecl for a function, and so on.
func declNode(obj varObj) any {
	return obj.Decl
}

// serviceContextParams returns the declaration objects of the function's
// parameters declared as *gst.ServiceContext under the names gstNames
// resolves. Objects rather than names tell the parameter apart from a local
// variable that reuses its name.
func serviceContextParams(decl *ast.FuncDecl, gstNames goast.PackageNames) map[varObj]bool {
	params := map[varObj]bool{}
	if decl.Type == nil || decl.Type.Params == nil {
		return params
	}
	for _, field := range decl.Type.Params.List {
		star, ok := field.Type.(*ast.StarExpr)
		if !ok || !gstNames.Refers(star.X, "ServiceContext") {
			continue
		}
		for _, name := range field.Names {
			if obj := declObj(name); obj != nil {
				params[obj] = true
			}
		}
	}
	return params
}
