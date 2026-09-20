package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestBuildLdflagsNameVariablesThatExist pins the injection against the
// package it writes into. The linker ignores a -X naming a variable it cannot
// find, silently: the build succeeds, every flag is accepted, and the running
// binary reports no version at all. Only reading the package's own source
// tells the two apart.
func TestBuildLdflagsNameVariablesThatExist(t *testing.T) {
	flags := buildLdflags(&BuildInfo{
		Version:   "v1.2.3",
		GitCommit: "abc1234",
		GitBranch: "main",
		BuildTime: "2026-07-01T00:00:00Z",
		GoVersion: runtime.Version(),
		Platform:  "darwin/arm64",
		Compiler:  "gc",
		BuildTags: "netgo",
	}, &Build{})

	declared := configStringVars(t)
	for flag := range strings.FieldsSeq(flags) {
		path, ok := strings.CutPrefix(strings.Trim(flag, "'"), "-X ")
		if !ok {
			continue
		}
		target, _, _ := strings.Cut(path, "=")
		pkg, name, found := strings.Cut(target, ".config.")
		if !found {
			t.Fatalf("a -X target must name the config package, got %q", target)
		}
		if !strings.HasSuffix(pkg, "-X github.com/hydroan/gst") && !strings.Contains(pkg, "github.com/hydroan/gst") {
			t.Fatalf("a -X target must name the framework's config package, got %q", target)
		}
		if _, ok := declared[name]; !ok {
			t.Fatalf("-X writes %q, which the config package does not declare: the linker would ignore it and the binary would carry no build information", name)
		}
	}
}

// configStringVars returns the package-level string variables the config
// package declares, which are the only targets a -X can write.
func configStringVars(t *testing.T) map[string]struct{} {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test's own file")
	}
	dir := filepath.Join(filepath.Dir(thisFile), "..", "..", "config")
	sources, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("listing the config package: %v", err)
	}

	fset := token.NewFileSet()
	declared := map[string]struct{}{}
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, source, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", source, err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				ident, ok := value.Type.(*ast.Ident)
				if !ok || ident.Name != "string" {
					continue
				}
				for _, varName := range value.Names {
					declared[varName.Name] = struct{}{}
				}
			}
		}
	}
	return declared
}
