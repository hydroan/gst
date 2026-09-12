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
)

// columnConstructors are the types functions that mint a column reference
// from a column name. Project code reads its columns through the XxxCols
// variables gg gen writes from the model schema; a reference minted by hand
// names a column the schema is never asked about, so a typo or a renamed
// field surfaces only when the query runs.
var columnConstructors = map[string]bool{
	"NewColumn":        true,
	"NewNumericColumn": true,
	"NewTimeColumn":    true,
}

// CheckColumnReferenceMinting reports project code that mints column
// references through types.NewColumn, NewNumericColumn or NewTimeColumn
// instead of reading the XxxCols variables gg gen writes. The one exception is
// generic code naming its own type parameter as the model, see
// mintsForTypeParameter. Generated files carry the constructors by design and
// are skipped, as are model and service subtrees owned by copyable framework
// modules, whose code is owned by the framework repository. Test files are
// checked like any other file: a test that mints a reference by hand stops
// noticing a renamed column just as production code does.
func CheckColumnReferenceMinting(ignore gitignore.Matcher) []string {
	owned, err := copyableModuleOwners()
	if err != nil {
		return []string{fmt.Sprintf("listing copyable framework modules: %v", err)}
	}

	var violations []string
	walkErr := walkProjectDir(".", ignore, func(path string, info os.FileInfo) error {
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
			if moduleOwnedPath(owned, modelDir, path) || moduleOwnedPath(owned, serviceDir, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || isGeneratedFileName(path) {
			return nil
		}
		violations = append(violations, checkFileColumnReferenceMinting(path)...)
		return nil
	})
	if walkErr != nil {
		violations = append(violations, fmt.Sprintf("walking project directory: %v", walkErr))
	}
	return violations
}

// checkFileColumnReferenceMinting reports the column constructor calls in
// one file. A file that fails to parse is reported as a violation so broken
// code cannot slip past the check.
func checkFileColumnReferenceMinting(path string) []string {
	aliases, dotImport, found := importNamesOf(path, gstTypesImportPath, "types")
	if !found {
		return nil
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return []string{fmt.Sprintf("%s has parse error: %v", relativePath(path), err)}
	}
	relPath := relativePath(path)

	var violations []string
	for _, decl := range file.Decls {
		// Only a function body can name a type parameter: calls in any other
		// declaration have none in scope and are all flagged.
		var typeParams map[string]bool
		if funcDecl, isFunc := decl.(*ast.FuncDecl); isFunc {
			typeParams = modelTypeParameters(funcDecl)
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, minted := columnConstructorName(call, aliases, dotImport)
			if !minted || mintsForTypeParameter(call, typeParams) {
				return true
			}
			pos := fset.Position(call.Pos())
			violations = append(violations, fmt.Sprintf(
				"%s:%d: mints a column reference through types.%s; read the column through the XxxCols variable gg gen writes for its model",
				relPath, pos.Line, name,
			))
			return true
		})
	}
	return violations
}

// mintsForTypeParameter reports whether a constructor call names, as its
// model, a type parameter of the function it sits in: the one minting the rule
// allows.
//
// Generic code has no concrete model in reach and so no Cols var to read.
// Passing its own type parameter as the model is the only way it keeps a typed
// reference; the alternative, a plain column name, also gives up the
// compile-time value check the reference's value type provides. Nothing about
// the table is restated: every instantiation binds the type parameter to a
// concrete model, whose TableName supplies the table exactly as it does for
// the generated references.
//
// The exception stops where a concrete model is in reach. A concrete first
// type argument stays flagged even inside a generic function, since that model
// has a Cols var to read. The type parameter must be named directly, as the
// function or its receiver declares it: an alias of it is not followed and is
// flagged like any other named type.
func mintsForTypeParameter(call *ast.CallExpr, typeParams map[string]bool) bool {
	var model ast.Expr
	switch fun := call.Fun.(type) {
	case *ast.IndexExpr:
		model = fun.Index
	case *ast.IndexListExpr:
		model = fun.Indices[0]
	}
	ident, ok := model.(*ast.Ident)
	return ok && typeParams[ident.Name]
}

// modelTypeParameters returns the type parameters code inside decl can pass as
// a model: the ones the function declares and the ones its receiver declares,
// which are the only type parameters a function body, function literals inside
// it included, can name. A name that a type declaration anywhere in the body
// reuses is dropped for the whole body. Telling apart the blocks where such a
// declaration takes the name over would need a scope analysis this syntactic
// check does not do, and dropping the name can only flag a reference, never
// let a concrete model through.
func modelTypeParameters(decl *ast.FuncDecl) map[string]bool {
	names := make(map[string]bool)
	if decl.Type.TypeParams != nil {
		for _, field := range decl.Type.TypeParams.List {
			for _, name := range field.Names {
				names[name.Name] = true
			}
		}
	}
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		receiver := decl.Recv.List[0].Type
		if pointer, isPointer := receiver.(*ast.StarExpr); isPointer {
			receiver = pointer.X
		}
		var params []ast.Expr
		switch generic := receiver.(type) {
		case *ast.IndexExpr:
			params = []ast.Expr{generic.Index}
		case *ast.IndexListExpr:
			params = generic.Indices
		}
		for _, param := range params {
			if ident, isIdent := param.(*ast.Ident); isIdent {
				names[ident.Name] = true
			}
		}
	}
	if decl.Body != nil {
		ast.Inspect(decl.Body, func(n ast.Node) bool {
			if spec, isType := n.(*ast.TypeSpec); isType {
				delete(names, spec.Name.Name)
			}
			return true
		})
	}
	return names
}

// columnConstructorName returns the column constructor a call invokes, with
// or without explicit type arguments, when the callee is one of the types
// package's column constructors.
func columnConstructorName(call *ast.CallExpr, aliases []string, dotImport bool) (string, bool) {
	fun := call.Fun
	switch f := fun.(type) {
	case *ast.IndexExpr:
		fun = f.X
	case *ast.IndexListExpr:
		fun = f.X
	}
	switch x := fun.(type) {
	case *ast.SelectorExpr:
		ident, ok := x.X.(*ast.Ident)
		if !ok || x.Sel == nil || !columnConstructors[x.Sel.Name] || !slices.Contains(aliases, ident.Name) {
			return "", false
		}
		return x.Sel.Name, true
	case *ast.Ident:
		if dotImport && columnConstructors[x.Name] {
			return x.Name, true
		}
	}
	return "", false
}

// importNamesOf reports the names a file refers to the package at importPath
// by: the aliases it imports the package under, defaultName standing for a
// plain import, and whether it dot-imports the package. found is false when
// the file does not import the package at all.
func importNamesOf(filePath, importPath, defaultName string) (aliases []string, dotImport bool, found bool) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.ImportsOnly)
	if err != nil {
		return nil, false, false
	}

	for _, imp := range file.Imports {
		if imp.Path == nil || imp.Path.Value != `"`+importPath+`"` {
			continue
		}
		found = true
		switch {
		case imp.Name == nil:
			aliases = append(aliases, defaultName)
		case imp.Name.Name == ".":
			dotImport = true
		case imp.Name.Name != "_":
			aliases = append(aliases, imp.Name.Name)
		}
	}
	return aliases, dotImport, found
}
