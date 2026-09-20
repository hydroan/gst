package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// TestBuildLdflagsNameVariablesThatExist pins the injection against the
// package it writes into. The linker ignores a -X naming a variable it
// cannot find, silently: the build succeeds, every flag is accepted, and the
// running binary reports no build information at all. Only reading the
// package's own source tells the two apart.
func TestBuildLdflagsNameVariablesThatExist(t *testing.T) {
	flags := buildLdflags(&BuildInfo{
		Version:      "v1.2.3",
		GitCommit:    "abc1234",
		GitBranch:    "main",
		BuildTime:    "2026-07-01T00:00:00Z",
		GoVersion:    runtime.Version(),
		Platform:     "darwin/arm64",
		Compiler:     "gc",
		BuildTags:    "netgo osusergo",
		GitTreeState: "clean",
	}, &Build{})

	targets := ldflagTargets(t, flags)
	if len(targets) == 0 {
		t.Fatal("no -X target was read from the flags: a guard that checks nothing passes on its own")
	}

	declared := configStringVars(t)
	for _, target := range targets {
		name, ok := strings.CutPrefix(target, configPackage+".")
		if !ok {
			t.Errorf("-X writes %q, which is outside the framework's config package", target)
			continue
		}
		if _, ok := declared[name]; !ok {
			t.Errorf("-X writes %q, which the config package does not declare: the linker ignores it without a word and the binary carries no build information", target)
		}
	}
}

// TestBuildLdflagsWritesCustomVariablesWhereTheyWereNamed pins the other
// half: a variable the caller asks for is written where the caller named it.
// A path the tool invents for the caller would be the same silent miss.
func TestBuildLdflagsWritesCustomVariablesWhereTheyWereNamed(t *testing.T) {
	flags := buildLdflags(&BuildInfo{
		CustomVars: map[string]string{"myapp/build.Channel": "beta"},
	}, &Build{})

	if !slices.Contains(ldflagTargets(t, flags), "myapp/build.Channel") {
		t.Errorf("the custom variable is not written where it was named, flags are %s", flags)
	}
}

// configPackage is the import path of the package the build information is
// linked into.
const configPackage = "github.com/hydroan/gst/config"

// ldflagTargets returns the variables the flags write, read the way the
// linker reads them: every -X carries a variable's full path, an equals sign
// and the value. Values hold spaces, so the flags are scanned for the -X
// itself rather than split into words.
func ldflagTargets(t *testing.T, flags string) []string {
	t.Helper()

	var targets []string
	for rest := flags; ; {
		_, assignment, ok := strings.Cut(rest, "-X '")
		if !ok {
			return targets
		}
		target, remainder, ok := strings.Cut(assignment, "=")
		if !ok {
			t.Fatalf("a -X carries path=value, got %q", assignment)
		}
		targets = append(targets, target)
		rest = remainder
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
