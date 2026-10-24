package gghelper_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/stretchr/testify/require"
)

// TestEmbeddedProgramsAreMainPackages pins where the programs gg builds
// inside a project come from: each is embedded from its own package under
// program/, a main package the framework's build compiles, with no template
// placeholder left for gg to fill.
func TestEmbeddedProgramsAreMainPackages(t *testing.T) {
	programs := map[string]map[string]string{
		"migrate": gghelper.MigrateProgram(),
		"inspect": gghelper.InspectProgram(),
	}
	for name, files := range programs {
		require.Contains(t, files, "main.go", name)
		for file, source := range files {
			parsed, err := parser.ParseFile(token.NewFileSet(), file, source, parser.PackageClauseOnly)
			require.NoError(t, err, "%s/%s", name, file)
			require.Equal(t, "main", parsed.Name.Name, "%s/%s", name, file)
			require.NotContains(t, source, "{{", "%s/%s", name, file)
		}
	}
}

// TestInspectWritesTheForwardedInternalTypesUnderTheirPublicPaths pins that
// the inspection program writes a framework-internal type back the way a
// business model names it: through the public package forwarding it, while
// reflection hands the program the internal path a project cannot import.
// Every type a public package forwards from an internal package is either
// written by typeExpr through a public package forwarding it under its own
// name, or named in nonColumnTypes, by itself or by its package, with what
// it is that no model stores.
func TestInspectWritesTheForwardedInternalTypesUnderTheirPublicPaths(t *testing.T) {
	forwarded := forwardedInternalTypes(t, filepath.Join("..", ".."))
	for _, problem := range inspectRewriteProblems(forwarded, inspectRewrites(t, gghelper.InspectProgram()["main.go"])) {
		t.Error(problem)
	}
}

// forwardedType is a type alias a public package of the framework declares
// for a type of an internal one: name is the alias, target the internal type
// it stands for, public the import path of the package declaring it.
type forwardedType struct {
	public, name, target string
}

// forwardedInternalTypes reads, from the framework's source under root, the
// type aliases its public packages declare for types of internal packages,
// keyed by the internal package's import path. The public packages are the
// framework's own, those outside internal/, cmd/, examples/ and module/, the
// last holding the business modules, whose types typeExpr leaves to its
// default; test files are left out. An alias with type parameters, or of an
// instantiation, is left out as well: it stands for instantiations, whose
// names reflection prints with their type arguments in brackets, and typeExpr
// writes none of those back, generating the column as Column[any].
func forwardedInternalTypes(t *testing.T, root string) map[string][]forwardedType {
	t.Helper()
	internalPrefix := ggconst.ImportPathGst + "/internal/"
	forwarded := make(map[string][]forwardedType)
	err := filepath.WalkDir(root, func(file string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(root, file)
		if relErr != nil {
			return relErr
		}
		if entry.IsDir() {
			if rel != "." && (slices.Contains([]string{"internal", "cmd", "examples", "module"}, rel) ||
				strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "_") || entry.Name() == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), file, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return parseErr
		}
		imports := make(map[string]string, len(parsed.Imports))
		for _, spec := range parsed.Imports {
			importPath, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil {
				return unquoteErr
			}
			name := path.Base(importPath)
			if spec.Name != nil {
				name = spec.Name.Name
			}
			imports[name] = importPath
		}
		public := ggconst.ImportPathGst
		if dir := filepath.Dir(rel); dir != "." {
			public += "/" + filepath.ToSlash(dir)
		}
		for _, decl := range parsed.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || !typeSpec.Assign.IsValid() || typeSpec.TypeParams != nil {
					continue
				}
				selector, ok := typeSpec.Type.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				qualifier, ok := selector.X.(*ast.Ident)
				if !ok || !strings.HasPrefix(imports[qualifier.Name], internalPrefix) {
					continue
				}
				internal := imports[qualifier.Name]
				forwarded[internal] = append(forwarded[internal], forwardedType{public: public, name: typeSpec.Name.Name, target: selector.Sel.Name})
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, forwarded, "no public package forwards an internal type; the walk reads the wrong tree")
	return forwarded
}

// inspectRewrites reads, from the source of the inspection program, the
// internal packages typeExpr rewrites, each to the public package whose
// import the rewrite adds, or to "" for one written without an import.
func inspectRewrites(t *testing.T, source string) map[string]string {
	t.Helper()
	internalPrefix := ggconst.ImportPathGst + "/internal/"
	parsed, err := parser.ParseFile(token.NewFileSet(), "main.go", source, parser.SkipObjectResolution)
	require.NoError(t, err)
	rewrites := make(map[string]string)
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "typeExpr" {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			clause, ok := node.(*ast.CaseClause)
			if !ok {
				return true
			}
			public := ""
			for _, stmt := range clause.Body {
				assign, ok := stmt.(*ast.AssignStmt)
				if !ok || len(assign.Lhs) != 1 {
					continue
				}
				index, ok := assign.Lhs[0].(*ast.IndexExpr)
				if !ok {
					continue
				}
				if target, ok := index.X.(*ast.Ident); !ok || target.Name != "imports" {
					continue
				}
				if literal, ok := index.Index.(*ast.BasicLit); ok {
					public, _ = strconv.Unquote(literal.Value)
				}
			}
			for _, expr := range clause.List {
				literal, ok := expr.(*ast.BasicLit)
				if !ok {
					continue
				}
				if value, unquoteErr := strconv.Unquote(literal.Value); unquoteErr == nil && strings.HasPrefix(value, internalPrefix) {
					rewrites[value] = public
				}
			}
			return true
		})
	}
	require.NotEmpty(t, rewrites, "typeExpr rewrites no internal package; the test no longer reads it")
	return rewrites
}

// inspectRewriteProblems lists what keeps typeExpr from writing a forwarded
// internal type the way a business model names it, and the entries of
// nonColumnTypes that no longer hold: the types of an internal package
// typeExpr leaves to its default that nonColumnTypes does not name, a type
// the public package typeExpr writes its package through does not forward
// under the same name, and an entry naming a package typeExpr rewrites, a
// type typeExpr writes through a public package or whose package is named
// already, or what no public package forwards.
func inspectRewriteProblems(forwarded map[string][]forwardedType, rewrites map[string]string) []string {
	var problems []string
	forwardedKeys := make(map[string]bool)
	for _, internal := range slices.Sorted(maps.Keys(forwarded)) {
		public, rewritten := rewrites[internal]
		_, packageNamed := nonColumnTypes[internal]
		forwardedKeys[internal] = true
		if rewritten && packageNamed {
			problems = append(problems, fmt.Sprintf("typeExpr rewrites %s, which nonColumnTypes names as well; drop it from nonColumnTypes", internal))
		}
		var defaulted []string
		for _, f := range forwarded[internal] {
			qualified := internal + "." + f.target
			if forwardedKeys[qualified] {
				continue
			}
			forwardedKeys[qualified] = true
			_, typeNamed := nonColumnTypes[qualified]
			written := rewritten && (public == "" || slices.Contains(forwarded[internal], forwardedType{public: public, name: f.target, target: f.target}))
			switch {
			case typeNamed && packageNamed:
				problems = append(problems, fmt.Sprintf("nonColumnTypes names %s and its package as well; drop %[1]s", qualified))
			case typeNamed && written:
				problems = append(problems, fmt.Sprintf("nonColumnTypes names %s, which typeExpr writes through a public package; drop it", qualified))
			}
			if written || packageNamed || typeNamed {
				continue
			}
			if rewritten {
				problems = append(problems, fmt.Sprintf("%s.%s forwards %s, which typeExpr writes as %s.%s, a name %[4]s does not forward; forward it from %[4]s, or name %[3]s in nonColumnTypes with what it is that no model stores",
					f.public, f.name, qualified, public, f.target))
				continue
			}
			defaulted = append(defaulted, f.public+"."+f.name)
		}
		if len(defaulted) > 0 {
			problems = append(problems, fmt.Sprintf("%s forward types of %s, which typeExpr writes under the internal path no project can import; rewrite the package in typeExpr, or name it, or each of those types, in nonColumnTypes with what they are that no model stores",
				strings.Join(defaulted, ", "), internal))
		}
	}
	for _, key := range slices.Sorted(maps.Keys(nonColumnTypes)) {
		if !forwardedKeys[key] {
			problems = append(problems, fmt.Sprintf("nonColumnTypes names %s, which no public package forwards; drop it", key))
		}
	}
	return problems
}

// nonColumnTypes are the forwarded internal types typeExpr leaves to its
// default, keyed by an internal package, standing for all of its forwarded
// types, or by a qualified type, standing for that one alone; each value
// says what the types are, which no model stores.
var nonColumnTypes = map[string]string{
	ggconst.ImportPathGst + "/internal/modelschema.Column":      "forwarded by modelschema to gg's own programs, which describe a model's columns with it",
	ggconst.ImportPathGst + "/internal/modelschema.ColumnClass": "forwarded by modelschema to gg's own programs, which classify a model's columns with it",
}

func TestProjectProgramRunsAgainstTheProjectModuleWithOverlay(t *testing.T) {
	t.Chdir(t.TempDir())
	goMod := "module tmpapp\n\ngo 1.27\n"
	onDisk := "package sample\n\nconst Source = \"disk\"\n"
	writeProjectFile(t, "go.mod", goMod)
	writeProjectFile(t, filepath.Join("sample", "sample.go"), onDisk)

	var out bytes.Buffer
	program := gghelper.ProjectProgram{
		Files:   map[string]string{"main.go": "package main\n\nimport (\n\t\"fmt\"\n\n\t\"tmpapp/sample\"\n)\n\nfunc main() { fmt.Print(sample.Source) }\n"},
		Stdout:  &out,
		Overlay: map[string]string{filepath.Join("sample", "sample.go"): "package sample\n\nconst Source = \"overlay\"\n"},
	}
	require.NoError(t, program.Run())

	// The program imports the project's own package and compiles it with the
	// overlay's replacement, while the files on disk stay as they were.
	require.Equal(t, "overlay", out.String())
	content, err := os.ReadFile(filepath.Join("sample", "sample.go"))
	require.NoError(t, err)
	require.Equal(t, onDisk, string(content))
	content, err = os.ReadFile("go.mod")
	require.NoError(t, err)
	require.Equal(t, goMod, string(content))
}

func TestProjectProgramPassesItsArguments(t *testing.T) {
	t.Chdir(t.TempDir())
	writeProjectFile(t, "go.mod", "module tmpapp\n\ngo 1.27\n")

	var out bytes.Buffer
	program := gghelper.ProjectProgram{
		Files:  map[string]string{"main.go": "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n\t\"strings\"\n)\n\nfunc main() { fmt.Print(strings.Join(os.Args[1:], \",\")) }\n"},
		Stdout: &out,
		Args:   []string{"sample", "with space"},
	}
	require.NoError(t, program.Run())

	require.Equal(t, "sample,with space", out.String())
}

func TestListProjectPackagesLeavesModuleFilesUntouched(t *testing.T) {
	t.Chdir(t.TempDir())
	goMod := "module tmpapp\n\ngo 1.27\n\nreplace example.com/dep => ./dep\n"
	writeProjectFile(t, "go.mod", goMod)
	writeProjectFile(t, filepath.Join("dep", "go.mod"), "module example.com/dep\n\ngo 1.27\n")
	writeProjectFile(t, filepath.Join("dep", "dep.go"), "package depname\n")

	listed, err := gghelper.ListProjectPackages([]string{"example.com/dep", "math/rand/v2"})
	require.NoError(t, err)

	// A package name need not match the last element of its path.
	require.Equal(t, "depname", listed["example.com/dep"].Name)
	require.Equal(t, "rand", listed["math/rand/v2"].Name)
	require.NotEmpty(t, listed["math/rand/v2"].GoFiles)
	// Resolving example.com/dep adds a requirement the project does not
	// declare yet, which must land in the copy of go.mod.
	content, err := os.ReadFile("go.mod")
	require.NoError(t, err)
	require.Equal(t, goMod, string(content))
}

// writeProjectFile writes content to path, creating its parent directories.
func writeProjectFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestProjectProgramRunsInsideAWorkspace(t *testing.T) {
	goMod := newWorkspaceProject(t)

	var out bytes.Buffer
	program := gghelper.ProjectProgram{
		Files:  map[string]string{"main.go": "package main\n\nimport (\n\t\"fmt\"\n\n\t\"example.com/lib\"\n)\n\nfunc main() { fmt.Print(libname.Source) }\n"},
		Stdout: &out,
	}
	require.NoError(t, program.Run())

	// The workspace, not a requirement of tmpapp, is what resolves lib, and
	// tmpapp's go.mod stays as it was.
	require.Equal(t, "workspace", out.String())
	content, err := os.ReadFile("go.mod")
	require.NoError(t, err)
	require.Equal(t, goMod, string(content))
}

func TestListProjectPackagesInsideAWorkspace(t *testing.T) {
	goMod := newWorkspaceProject(t)

	listed, err := gghelper.ListProjectPackages([]string{"example.com/lib", "math/rand/v2"})
	require.NoError(t, err)

	require.Equal(t, "libname", listed["example.com/lib"].Name)
	require.Equal(t, "rand", listed["math/rand/v2"].Name)
	content, err := os.ReadFile("go.mod")
	require.NoError(t, err)
	require.Equal(t, goMod, string(content))
}

// newWorkspaceProject lays out a workspace of two modules, tmpapp and
// example.com/lib, where tmpapp requires nothing and reaches lib through
// go.work alone, enters tmpapp and points GOWORK at the workspace file. It
// returns tmpapp's go.mod content, which the test expects to find unchanged.
func newWorkspaceProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	goMod := "module tmpapp\n\ngo 1.27\n"
	writeProjectFile(t, filepath.Join(dir, "app", "go.mod"), goMod)
	writeProjectFile(t, filepath.Join(dir, "lib", "go.mod"), "module example.com/lib\n\ngo 1.27\n")
	writeProjectFile(t, filepath.Join(dir, "lib", "lib.go"), "package libname\n\nconst Source = \"workspace\"\n")
	goWork := filepath.Join(dir, "go.work")
	writeProjectFile(t, goWork, "go 1.27\n\nuse (\n\t./app\n\t./lib\n)\n")
	t.Chdir(filepath.Join(dir, "app"))
	t.Setenv("GOWORK", goWork)
	return goMod
}
