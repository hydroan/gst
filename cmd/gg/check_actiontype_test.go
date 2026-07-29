package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckActionTypeFormStructAndSliceForms(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() {
		modelDir = oldModelDir
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"

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
	oldModelDir := modelDir
	t.Cleanup(func() {
		modelDir = oldModelDir
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"

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
	oldModelDir := modelDir
	t.Cleanup(func() {
		modelDir = oldModelDir
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"

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

func TestCheckActionTypeFormAllowsDefaultCRUDAndEmptySides(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() {
		modelDir = oldModelDir
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"

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
