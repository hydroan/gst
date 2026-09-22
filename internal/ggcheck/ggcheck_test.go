package ggcheck_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

// TestEveryCheckIsDeclaredInAFileNamedAfterIt holds the layout the package
// documentation describes: every Check value is declared in a file of its own,
// named after the check, as dsl_design_rules.go for "DSL design rules".
func TestEveryCheckIsDeclaredInAFileNamedAfterIt(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checksIn := make(map[string][]string)
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, value := range valueSpec.Values {
					if name, ok := checkName(value); ok {
						checksIn[path] = append(checksIn[path], name)
					}
				}
			}
		}
	}

	if len(checksIn) == 0 {
		t.Fatal("found no Check declared in the package")
	}
	for path, names := range checksIn {
		if len(names) != 1 {
			t.Errorf("%s declares %d checks %q, want one check per file", path, len(names), names)
			continue
		}
		if want := strings.ReplaceAll(strings.ToLower(names[0]), " ", "_") + ".go"; path != want {
			t.Errorf("check %q is declared in %s, want %s", names[0], path, want)
		}
	}
}

// TestRunReturnsOneResultPerCheckInOrder pins that Run answers with one result
// per check, in the order the checks were given, each under its check's name.
func TestRunReturnsOneResultPerCheckInOrder(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record-item.go"), "package record\n")

	results := ggcheck.Run([]ggcheck.Check{ggcheck.ModelFileBoundaries, ggcheck.ModelFileNameHyphens})

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Name != "Model file boundaries" || len(results[0].Violations) != 0 {
		t.Fatalf("results[0] = %+v, want a clean Model file boundaries result", results[0])
	}
	if results[1].Name != "Model file name hyphens" || len(results[1].Violations) != 1 {
		t.Fatalf("results[1] = %+v, want one Model file name hyphens violation", results[1])
	}
}

// checkName returns the Name of a Check composite literal.
func checkName(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return "", false
	}
	if ident, ok := lit.Type.(*ast.Ident); !ok || ident.Name != "Check" {
		return "", false
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Name" {
			continue
		}
		if value, ok := kv.Value.(*ast.BasicLit); ok {
			name, err := strconv.Unquote(value.Value)
			return name, err == nil
		}
	}
	return "", false
}
