package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

// TestRun runs every check over the fixture module of TestCheckTestPlacement
// and gets what make check prints for it: the two test files that check
// reports there.
func TestRun(t *testing.T) {
	violations, err := run("testdata/testplacement/module")
	require.NoError(t, err)
	require.Equal(t, []violation{
		{
			File:    "mislabeled/mislabeled_internal_test.go",
			Message: "Test file 'mislabeled/mislabeled_internal_test.go' is named as an internal test but declares package mislabeled_test: drop _internal from its name",
		},
		{
			File:    "movable/movable_internal_test.go",
			Message: "Test file 'movable/movable_internal_test.go' uses nothing unexported of package movable: declare package movable_test and drop _internal from its name",
		},
	}, violations)
}

func TestRunFailsOnAPackageThatDoesNotTypeCheck(t *testing.T) {
	_, err := run("testdata/broken")
	require.ErrorContains(t, err, "does not type-check")
}

// loadFixture loads the fixture module at dir the way run loads the tree it
// checks, and returns the module's absolute root with its packages.
func loadFixture(t *testing.T, dir string) (string, []*packages.Package) {
	t.Helper()
	root, err := filepath.Abs(dir)
	require.NoError(t, err)
	pkgs, err := load(root, nil, "./...")
	require.NoError(t, err)
	return root, pkgs
}

// TestEveryCheckIsDeclaredInAFileNamedAfterIt holds the layout the package
// documentation describes: the run function of every check in checks is
// declared in the file named after the check, and the package holds no other
// source file than main.go, helper.go and one file per check.
func TestEveryCheckIsDeclaredInAFileNamedAfterIt(t *testing.T) {
	fset := token.NewFileSet()
	sources, err := filepath.Glob("*.go")
	require.NoError(t, err)

	declaredIn := make(map[string]string)
	var files []string
	var runFuncs map[string]string
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		files = append(files, path)
		file, err := parser.ParseFile(fset, path, nil, 0)
		require.NoError(t, err)
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil {
				declaredIn[fn.Name.Name] = path
			}
		}
		if path == "main.go" {
			runFuncs = checkRunFuncs(t, file)
		}
	}
	require.NotEmpty(t, runFuncs, "main.go declares the checks")

	want := []string{"helper.go", "main.go"}
	for name, fn := range runFuncs {
		require.Equal(t, name+".go", declaredIn[fn], "the run function %s of check %q", fn, name)
		want = append(want, name+".go")
	}
	sort.Strings(want)
	require.Equal(t, want, files, "one source file per check, plus main.go and helper.go")
}

// checkRunFuncs reads the checks list of file, main.go, into a map of check
// name to the name of its run function.
func checkRunFuncs(t *testing.T, file *ast.File) map[string]string {
	t.Helper()

	funcs := make(map[string]string)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok || len(valueSpec.Names) != 1 || valueSpec.Names[0].Name != "checks" || len(valueSpec.Values) != 1 {
				continue
			}
			list, ok := valueSpec.Values[0].(*ast.CompositeLit)
			require.True(t, ok, "checks is a composite literal")
			for _, elt := range list.Elts {
				entry, ok := elt.(*ast.CompositeLit)
				require.True(t, ok, "every check is a composite literal")
				var name, fn string
				for _, kv := range entry.Elts {
					pair, ok := kv.(*ast.KeyValueExpr)
					require.True(t, ok, "every check field is keyed")
					key, ok := pair.Key.(*ast.Ident)
					require.True(t, ok, "every check field is named")
					switch key.Name {
					case "name":
						lit, ok := pair.Value.(*ast.BasicLit)
						require.True(t, ok, "the name of a check is a literal")
						var err error
						name, err = strconv.Unquote(lit.Value)
						require.NoError(t, err)
					case "run":
						ident, ok := pair.Value.(*ast.Ident)
						require.True(t, ok, "the run function of a check is named")
						fn = ident.Name
					}
				}
				require.NotEmpty(t, name)
				require.NotEmpty(t, fn)
				funcs[name] = fn
			}
		}
	}
	return funcs
}
