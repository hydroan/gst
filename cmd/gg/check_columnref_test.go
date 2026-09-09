package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckColumnReferenceMintingFlagsConstructors(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "service", "report", "helper.go"), `package report

import (
	"time"

	"github.com/hydroan/gst/types"
)

type Sample struct{}

func (*Sample) TableName() string { return "samples" }

var (
	code   = types.NewColumn[*Sample, string]("code")
	amount = types.NewNumericColumn[*Sample, int64]("amount")
	at     = types.NewTimeColumn[*Sample]("created_at")
)

func filters(since time.Time) []types.Filter {
	return []types.Filter{code.Eq("x"), amount.Gt(1), at.Gte(since)}
}
`)
	// An alias does not hide the constructor, and a test file is project
	// code like any other.
	writeCheckFile(t, filepath.Join(projectDir, "service", "report", "helper_test.go"), `package report

import gsttypes "github.com/hydroan/gst/types"

var tested = gsttypes.NewColumn[*Sample, string]("code")
`)
	// A dot import spells the constructor bare.
	writeCheckFile(t, filepath.Join(projectDir, "helper", "scope", "scope.go"), `package scope

import . "github.com/hydroan/gst/types"

type Row struct{}

func (*Row) TableName() string { return "rows" }

var group = NewColumn[*Row, string]("group_id")
`)

	violations := CheckColumnReferenceMinting(newProjectIgnoreMatcher())

	if len(violations) != 5 {
		t.Fatalf("expected five violations, got %#v", violations)
	}
	helperPath := filepath.Join("service", "report", "helper.go")
	for _, constructor := range []string{"types.NewColumn;", "types.NewNumericColumn;", "types.NewTimeColumn;"} {
		matched := 0
		for _, violation := range violations {
			if strings.Contains(violation, helperPath) && strings.Contains(violation, constructor) {
				matched++
			}
		}
		if matched != 1 {
			t.Fatalf("expected one %s violation in %s, got %#v", constructor, helperPath, violations)
		}
	}
	assertViolationContains(t, violations, filepath.Join("service", "report", "helper_test.go"), "mints a column reference through types.NewColumn")
	assertViolationContains(t, violations, filepath.Join("helper", "scope", "scope.go"), "mints a column reference through types.NewColumn")
}

func TestCheckColumnReferenceMintingSkipsGeneratedAndCopiedModules(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)

	writeFrameworkModuleFixture(t, projectDir, "sample")

	// Generated files carry the constructors by design.
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.gen.go"), `package record

import "github.com/hydroan/gst/types"

type Record struct{}

func (*Record) TableName() string { return "records" }

var RecordCols = struct{ Code types.Column[string] }{
	Code: types.NewColumn[*Record, string]("code"),
}
`)
	// Copied module subtrees keep whatever the framework repository ships.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "entity.go"), `package sample

import "github.com/hydroan/gst/types"

type Entity struct{}

func (*Entity) TableName() string { return "entities" }

var code = types.NewColumn[*Entity, string]("code")
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "sample", "helper.go"), `package sample

import "github.com/hydroan/gst/types"

type Entity struct{}

func (*Entity) TableName() string { return "entities" }

var code = types.NewColumn[*Entity, string]("code")
`)
	// A nested Go module belongs to another project.
	writeCheckFile(t, filepath.Join(projectDir, "tools", "go.mod"), "module tools\n\ngo 1.26\n")
	writeCheckFile(t, filepath.Join(projectDir, "tools", "main.go"), `package main

import "github.com/hydroan/gst/types"

type Row struct{}

func (*Row) TableName() string { return "rows" }

var code = types.NewColumn[*Row, string]("code")

func main() {}
`)
	// Reading the generated references is the sanctioned spelling.
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "list.go"), `package record

import (
	"tmpapp/model/record"

	"github.com/hydroan/gst/types"
)

func filters() []types.Filter {
	return []types.Filter{record.RecordCols.Code.Eq("x")}
}
`)

	violations := CheckColumnReferenceMinting(newProjectIgnoreMatcher())

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}
