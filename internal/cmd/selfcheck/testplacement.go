package main

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	"golang.org/x/tools/go/packages"
)

// internalTestSuffix ends the name of every test file that joins the package
// it tests; testpackage's skip-regexp names the same suffix.
const internalTestSuffix = "_internal_test.go"

// checkTestPlacement reports the test files under root named *_internal_test.go
// that could be external tests: those that declare the external test package,
// and those that use nothing unexported of the package they test, which
// checkTestPlacement confirms by type-checking them as external tests. A test
// file whose move would not compile is never reported, and neither is one that
// another internal test file depends on. Test files of a main package are left
// alone, since nothing can import a main package.
func checkTestPlacement(root string, pkgs []*packages.Package) ([]violation, error) {
	var violations []violation
	for _, p := range pkgs {
		switch {
		case isExternalTest(p):
			for _, path := range p.CompiledGoFiles {
				if strings.HasSuffix(path, internalTestSuffix) {
					file := relative(root, path)
					violations = append(violations, violation{
						File:    file,
						Message: fmt.Sprintf("Test file '%s' is named as an internal test but declares package %s: drop _internal from its name", file, p.Name),
					})
				}
			}
		case isInternalTestVariant(p) && p.Name != "main":
			movable, err := confirmExternal(root, p, externalCandidates(analyzeTestFiles(p)))
			if err != nil {
				return nil, err
			}
			for _, f := range movable {
				file := relative(root, f.path)
				violations = append(violations, violation{
					File:    file,
					Message: fmt.Sprintf("Test file '%s' uses nothing unexported of package %s: declare package %s_test and drop _internal from its name", file, p.Name, p.Name),
				})
			}
		}
	}
	sort.Slice(violations, func(i, j int) bool { return violations[i].File < violations[j].File })
	return violations, nil
}

// isExternalTest reports whether p is an external test package, the one whose
// files declare package <name>_test.
func isExternalTest(p *packages.Package) bool {
	return strings.HasSuffix(p.PkgPath, "_test")
}

// The static pass: what a test file uses of its package, read from the syntax
// and types of the package's internal test variant.

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

// analyzeTestFiles runs the static pass over the test files of p, an internal
// test variant.
func analyzeTestFiles(p *packages.Package) map[string]*testFile {
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
		inspectTestFile(p, f)
	}
	return files
}

// inspectTestFile records in f what ties it to the package p it tests.
func inspectTestFile(p *packages.Package, f *testFile) {
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

// externalCandidates returns the files named as internal tests that the static
// pass finds movable: blocked by nothing, and moving together with every test
// file they use and every test file that uses them.
func externalCandidates(files map[string]*testFile) []*testFile {
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

// isTestFile reports whether path names a Go test file.
func isTestFile(path string) bool {
	return strings.HasSuffix(path, "_test.go")
}

// The confirmation: a candidate the static pass found is reported only once
// it type-checks as an external test, so a rule the pass got wrong sends a
// file to this slower step instead of to the report.

// confirmExternal type-checks the candidates of p rewritten as external tests
// and returns the ones that compile that way, so a candidate the static pass
// misjudged is never reported. The candidates move together first, since they
// may use each other; only when that fails is each one tried on its own.
func confirmExternal(root string, p *packages.Package, found []*testFile) ([]*testFile, error) {
	if len(found) == 0 {
		return nil, nil
	}
	sort.Slice(found, func(i, j int) bool { return found[i].path < found[j].path })

	ok, err := compilesExternally(root, p, found)
	if err != nil {
		return nil, err
	}
	if ok {
		return found, nil
	}
	var confirmed []*testFile
	for _, f := range found {
		ok, err := compilesExternally(root, p, []*testFile{f})
		if err != nil {
			return nil, err
		}
		if ok {
			confirmed = append(confirmed, f)
		}
	}
	return confirmed, nil
}

// compilesExternally reports whether p and its tests still type-check once
// files declare the external test package instead.
func compilesExternally(root string, p *packages.Package, files []*testFile) (bool, error) {
	overlay := make(map[string][]byte, len(files))
	for _, f := range files {
		src, err := os.ReadFile(f.path)
		if err != nil {
			return false, errors.Wrapf(err, "read %s", f.path)
		}
		overlay[f.path] = asExternal(src, f, p.Name, p.PkgPath)
	}
	pattern := "."
	if dir := relative(root, filepath.Dir(files[0].path)); dir != "." {
		pattern = "./" + dir
	}
	pkgs, err := load(root, overlay, pattern)
	if err != nil {
		return false, err
	}
	for _, q := range pkgs {
		if len(q.Errors) > 0 {
			return false, nil
		}
	}
	return true, nil
}

// asExternal rewrites the source of f as a file of the external test package
// of the package name at path: the package clause gains _test, the package
// is imported under its name on the same line, keeping every other line where
// it was, and each identifier naming the package's own declarations is
// qualified with it.
func asExternal(src []byte, f *testFile, name, path string) []byte {
	type insertion struct {
		offset int
		text   string
	}
	clause := "_test"
	if len(f.qualify) > 0 {
		clause += fmt.Sprintf("; import %s %q", name, path)
	}
	insertions := []insertion{{offset: f.nameEnd, text: clause}}
	for _, offset := range f.qualify {
		insertions = append(insertions, insertion{offset: offset, text: name + "."})
	}
	// Inserting from the end keeps the offsets still to be used valid.
	sort.Slice(insertions, func(i, j int) bool { return insertions[i].offset > insertions[j].offset })

	out := append([]byte(nil), src...)
	for _, in := range insertions {
		out = append(out[:in.offset], append([]byte(in.text), out[in.offset:]...)...)
	}
	return out
}
