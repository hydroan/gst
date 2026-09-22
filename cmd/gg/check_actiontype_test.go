package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckActionTypeFormStructAndSliceForms(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	// A struct action type must use the pointer form, while a slice or map
	// action type (declared through a named alias) must use the value form.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), `package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Base
}

func (Sample) Design() {
	Update(func() {
		Service()
		Payload[*SampleUpdateReq]()
		Result[SampleUpdateRsp]()
	})
	Patch(func() {
		Service()
		Payload[SamplePatchReq]()
		Result[*SamplePatchRsp]()
	})
}

type SampleUpdateReq struct {
	Name string `+"`json:\"name\"`"+`
}

type SampleUpdateRsp = []*Sample

type SamplePatchReq struct {
	Name string `+"`json:\"name\"`"+`
}

type SamplePatchRsp = []*Sample
`)

	violations := CheckActionTypeForm(newProjectIgnoreMatcher())

	if len(violations) != 2 {
		t.Fatalf("expected two form violations, got %#v", violations)
	}
	joined := strings.Join(violations, "\n")
	if !strings.Contains(joined, "Patch action declares Payload[SamplePatchReq] with the value form; a struct action type must use the pointer form Payload[*SamplePatchReq]") {
		t.Fatalf("expected value-form struct violation, got %#v", violations)
	}
	if !strings.Contains(joined, "Patch action declares Result[*SamplePatchRsp] with the pointer form; a slice or map action type must use the value form Result[SamplePatchRsp]") {
		t.Fatalf("expected pointer-form slice violation, got %#v", violations)
	}
}

func TestCheckActionTypeFormEmptyStructPairRule(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	// Empty struct action types are the delegation marker for actions that
	// carry no data: they are allowed only when both sides of the action are
	// empty (an omitted side counts as empty). An empty struct paired with a
	// real data type must be removed because omitting it defaults the side to
	// *model.Empty.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), `package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Base
}

func (Sample) Design() {
	Delete(func() {
		Service()
		Payload[*SampleDeleteReq]()
		Result[*SampleDeleteRsp]()
	})
	Update(func() {
		Service()
		Payload[*SampleUpdateReq]()
	})
	Create(func() {
		Service()
		Payload[*SampleCreateReq]()
		Result[*SampleCreateRsp]()
	})
}

type SampleDeleteReq struct{}

type SampleDeleteRsp struct{}

type SampleUpdateReq struct{}

type SampleCreateReq struct{}

type SampleCreateRsp struct {
	Name string `+"`json:\"name\"`"+`
}
`)

	violations := CheckActionTypeForm(newProjectIgnoreMatcher())

	for _, violation := range violations {
		if strings.Contains(violation, "SampleDeleteReq") || strings.Contains(violation, "SampleDeleteRsp") || strings.Contains(violation, "SampleUpdateReq") {
			t.Fatalf("empty struct with an empty peer side should be allowed, got violations: %#v", violations)
		}
	}
	if len(violations) != 1 || !strings.Contains(violations[0], "Create action declares Payload[*SampleCreateReq] whose type is an empty struct; remove the declaration so the framework defaults this side to *model.Empty") {
		t.Fatalf("expected single empty-struct removal violation, got %#v", violations)
	}
}

func TestCheckActionTypeFormRejectsUnsupportedArguments(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	// The type argument must be a named type declared in the same model
	// package: slice literals, cross-package selectors, and undeclared names
	// are rejected instead of being silently dropped by the parser.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), `package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"example.com/external"
)

type Sample struct {
	model.Base
}

func (Sample) Design() {
	Create(func() {
		Service()
		Payload[[]SampleItem]()
		Result[*external.CreateRsp]()
	})
	Update(func() {
		Service()
		Payload[*SampleMissingReq]()
	})
}

type SampleItem struct {
	Name string `+"`json:\"name\"`"+`
}
`)

	violations := CheckActionTypeForm(newProjectIgnoreMatcher())

	if len(violations) != 3 {
		t.Fatalf("expected three violations, got %#v", violations)
	}
	joined := strings.Join(violations, "\n")
	if strings.Count(joined, "type argument must be a named type declared in the same model package") != 2 {
		t.Fatalf("expected two unsupported-argument violations, got %#v", violations)
	}
	if !strings.Contains(joined, "Update action declares Payload[*SampleMissingReq] but the type is not declared in the model package") {
		t.Fatalf("expected undeclared-type violation, got %#v", violations)
	}
}

func TestCheckActionTypeFormRejectsInterfacePayloads(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	// A request body decodes into no interface with methods, whether the
	// interface declares them itself or embeds an interface that does, and
	// whether Payload names it by value or through a pointer. An interface
	// without methods holds any JSON value, and a Result is only encoded, so
	// neither is a violation.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), `package sample

import (
	"fmt"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Base
}

func (Sample) Design() {
	Create(func() {
		Service()
		Payload[SampleCreateReq]()
		Result[SampleCreateRsp]()
	})
	Update(func() {
		Service()
		Payload[*SampleUpdateReq]()
	})
	Patch(func() {
		Service()
		Payload[SamplePatchReq]()
	})
	Delete(func() {
		Service()
		Payload[SampleDeleteReq]()
	})
}

type SampleCreateReq interface {
	Bind()
}

type SampleCreateRsp interface {
	Render()
}

type SampleUpdateReq interface {
	fmt.Stringer
}

type SamplePatchReq interface {
	SampleBinder
}

type SampleBinder interface {
	Bind()
}

type SampleDeleteReq interface {
	SampleAny
}

type SampleAny = interface{}
`)

	violations := CheckActionTypeForm(newProjectIgnoreMatcher())

	if len(violations) != 3 {
		t.Fatalf("expected three interface violations, got %#v", violations)
	}
	joined := strings.Join(violations, "\n")
	for _, want := range []string{
		"Create action declares Payload[SampleCreateReq] whose type is an interface with methods, which no request body decodes into; declare a struct type and use the pointer form Payload[*SampleCreateReq]",
		"Update action declares Payload[*SampleUpdateReq] whose type is an interface with methods, which no request body decodes into; declare a struct type and use the pointer form Payload[*SampleUpdateReq]",
		"Patch action declares Payload[SamplePatchReq] whose type is an interface with methods, which no request body decodes into; declare a struct type and use the pointer form Payload[*SamplePatchReq]",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected violation %q, got %#v", want, violations)
		}
	}
}

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

func TestCheckActionTypeFormAllowsDefaultCRUDAndEmptySides(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	// Default CRUD actions keep the model type on both sides, and a GET
	// action with a declared Result carries the *model.Empty sentinel as its
	// request side; neither shape is a violation.
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.go"), `package record

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Migrate()
	Create(func() {})
	List(func() {
		Service()
		Result[*RecordListRsp]()
	})
}

type RecordListRsp struct {
	Items []*Record `+"`json:\"items\"`"+`
}
`)

	violations := CheckActionTypeForm(newProjectIgnoreMatcher())

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}
