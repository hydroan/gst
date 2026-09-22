package ggcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestInterfaceDeclaresMethods(t *testing.T) {
	// Each type below is an interface, declaring methods itself, through what
	// it embeds, or not at all; a cycle of embeddings must end the walk.
	file, err := parser.ParseFile(token.NewFileSet(), "sample.go", `package sample

import (
	"fmt"
	. "io"
)

type SampleBinder interface{ Bind() }

type SampleAny = interface{}

type SampleOwn interface{ Bind() }

type SampleEmpty interface{}

type SampleEmbedsLocal interface{ SampleBinder }

type SampleEmbedsEmptyAlias interface{ SampleAny }

type SampleEmbedsAny interface{ any }

type SampleEmbedsError interface{ error }

type SampleEmbedsForeign interface{ fmt.Stringer }

type SampleEmbedsDotImported interface{ Reader }

type SampleCycleA interface{ SampleCycleB }

type SampleCycleB interface{ SampleCycleA }
`, 0)
	if err != nil {
		t.Fatalf("parse source failed: %v", err)
	}
	typeExprs := make(map[string]ast.Expr)
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			if typeSpec, ok := spec.(*ast.TypeSpec); ok {
				typeExprs[typeSpec.Name.Name] = typeSpec.Type
			}
		}
	}

	tests := []struct {
		name string
		want bool
	}{
		{name: "SampleOwn", want: true},
		{name: "SampleEmpty", want: false},
		{name: "SampleEmbedsLocal", want: true},
		{name: "SampleEmbedsEmptyAlias", want: false},
		{name: "SampleEmbedsAny", want: false},
		{name: "SampleEmbedsError", want: true},
		{name: "SampleEmbedsForeign", want: true},
		{name: "SampleEmbedsDotImported", want: true},
		{name: "SampleCycleA", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := interfaceDeclaresMethods(typeExprs[tt.name], typeExprs, make(map[string]bool)); got != tt.want {
				t.Fatalf("interfaceDeclaresMethods(%s) = %t, want %t", tt.name, got, tt.want)
			}
		})
	}
}
