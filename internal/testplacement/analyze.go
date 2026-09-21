package testplacement

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// testFile is a test file of a package's internal test variant, with what the
// static pass learned about it.
type testFile struct {
	path   string
	syntax *ast.File

	// blocked is set when the file uses something only the package itself
	// can: an unexported name declared outside the test files, a method
	// declared on one of the package's types, or an unkeyed literal of a
	// struct with unexported fields.
	blocked bool
	// deps holds the other test files of the package whose declarations
	// the file uses.
	deps map[string]bool
	// nameEnd is the offset just past the package name in the package clause.
	nameEnd int
	// qualify holds the offsets of the identifiers naming the package's own
	// declarations, which an external test reaches through the package name.
	qualify []int
}

// analyze runs the static pass over the test files of p, an internal test
// variant.
func analyze(p *packages.Package) map[string]*testFile {
	files := make(map[string]*testFile)
	for i, syntax := range p.Syntax {
		path := p.CompiledGoFiles[i]
		if isTestFile(path) {
			files[path] = &testFile{
				path:    path,
				syntax:  syntax,
				deps:    make(map[string]bool),
				nameEnd: p.Fset.Position(syntax.Name.End()).Offset,
			}
		}
	}
	for _, f := range files {
		inspect(p, f)
	}
	return files
}

// inspect records in f what ties it to the package p it tests.
func inspect(p *packages.Package, f *testFile) {
	declaredIn := func(obj types.Object) string { return p.Fset.Position(obj.Pos()).Filename }

	for _, decl := range f.syntax.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil {
			continue
		}
		method, ok := p.TypesInfo.Defs[fn.Name].(*types.Func)
		if !ok {
			continue
		}
		named, ok := derefNamed(method.Signature().Recv().Type())
		if !ok {
			continue
		}
		switch file := declaredIn(named.Obj()); {
		case file == f.path:
		case isTestFile(file):
			f.deps[file] = true
		default:
			// Only the package that declares a type can declare its methods.
			f.blocked = true
		}
	}

	ast.Inspect(f.syntax, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Ident:
			obj := p.TypesInfo.Uses[n]
			if obj == nil || obj.Pkg() != p.Types {
				return true
			}
			if _, ok := obj.(*types.PkgName); ok {
				return true
			}
			switch file := declaredIn(obj); {
			case file == f.path:
			case isTestFile(file):
				f.deps[file] = true
			case !obj.Exported():
				f.blocked = true
			case obj.Parent() == p.Types.Scope():
				f.qualify = append(f.qualify, p.Fset.Position(n.Pos()).Offset)
			}
		case *ast.CompositeLit:
			if unkeyedWithUnexportedFields(p, n) {
				f.blocked = true
			}
		}
		return true
	})
}

// unkeyedWithUnexportedFields reports whether lit is an unkeyed literal of a
// struct type of p with an unexported field, which only p itself can write.
func unkeyedWithUnexportedFields(p *packages.Package, lit *ast.CompositeLit) bool {
	if len(lit.Elts) == 0 {
		return false
	}
	if _, keyed := lit.Elts[0].(*ast.KeyValueExpr); keyed {
		return false
	}
	tv, ok := p.TypesInfo.Types[lit]
	if !ok {
		return false
	}
	named, ok := derefNamed(tv.Type)
	if !ok || named.Obj().Pkg() != p.Types || isTestFile(p.Fset.Position(named.Obj().Pos()).Filename) {
		return false
	}
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for field := range st.Fields() {
		if !field.Exported() {
			return true
		}
	}
	return false
}

// candidates returns the files named as internal tests that the static pass
// finds movable: blocked by nothing, and moving together with every test file
// they use and every test file that uses them.
func candidates(files map[string]*testFile) []*testFile {
	users := make(map[string]map[string]bool)
	movable := make(map[string]bool)
	for path, f := range files {
		for dep := range f.deps {
			if users[dep] == nil {
				users[dep] = make(map[string]bool)
			}
			users[dep][path] = true
		}
		if !f.blocked {
			movable[path] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for path := range movable {
			if anyOutside(files[path].deps, movable) || anyOutside(users[path], movable) {
				delete(movable, path)
				changed = true
			}
		}
	}

	var found []*testFile
	for path := range movable {
		if strings.HasSuffix(path, internalTestSuffix) {
			found = append(found, files[path])
		}
	}
	return found
}

// anyOutside reports whether a path of set is missing from movable.
func anyOutside(set, movable map[string]bool) bool {
	for path := range set {
		if !movable[path] {
			return true
		}
	}
	return false
}

// derefNamed returns the named type t is, or points to.
func derefNamed(t types.Type) (*types.Named, bool) {
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	named, ok := t.(*types.Named)
	return named, ok
}

// isTestFile reports whether path names a Go test file.
func isTestFile(path string) bool {
	return strings.HasSuffix(path, "_test.go")
}
