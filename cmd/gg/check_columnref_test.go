package main

import (
	"path/filepath"
	"slices"
	"strconv"
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

func TestCheckColumnReferenceMintingAllowsGenericCodeTypeParameters(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)

	// Generic code has no concrete model and so no generated Cols var: it
	// names its own type parameter as the model, whether the function
	// declares the parameter or its receiver does, including from a function
	// literal inside such a function.
	writeCheckFile(t, filepath.Join(projectDir, "helper", "retention", "retention.go"), `package retention

import (
	"time"

	"github.com/hydroan/gst/types"
)

func expired[M types.Model](cutoff time.Time, ids ...string) []types.Filter {
	byID := func() types.Filter {
		return types.NewColumn[M, string]("id").In(ids...)
	}
	return []types.Filter{types.NewTimeColumn[M]("created_at").Lte(cutoff), byID()}
}

type Totals[M types.Model] struct{}

func (Totals[M]) amount() types.Term {
	return types.NewNumericColumn[M, int64]("amount").Sum()
}

func (*Totals[M]) rows() types.Term {
	return types.NewColumn[M, string]("id").Count()
}

type Pair[M types.Model, V comparable] struct{}

func (p *Pair[M, V]) match(value V) types.Filter {
	return types.NewColumn[M, V]("value").Eq(value)
}
`)
	// A dot import spells the constructor bare.
	writeCheckFile(t, filepath.Join(projectDir, "helper", "scope", "scope.go"), `package scope

import . "github.com/hydroan/gst/types"

func owned[M Model](groupIDs ...string) Filter {
	return NewColumn[M, string]("group_id").In(groupIDs...)
}
`)

	violations := CheckColumnReferenceMinting(newProjectIgnoreMatcher())

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}

func TestCheckColumnReferenceMintingFlagsConcreteModelsInGenericCode(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)

	source := `package retention

import "github.com/hydroan/gst/types"

type Sample struct{}

func (*Sample) TableName() string { return "samples" }

type M = *Sample

func concrete[N types.Model]() types.Filter {
	return types.NewColumn[*Sample, string]("code").Eq("concrete")
}

func packageLevel() types.Filter {
	return types.NewColumn[M, string]("code").Eq("package")
}

func reused[M types.Model](local bool) types.Filter {
	if local {
		type M = *Sample
		return types.NewColumn[M, string]("code").Eq("reused inside")
	}
	return types.NewColumn[M, string]("code").Eq("reused outside")
}

func parameter[M types.Model]() types.Filter {
	return types.NewColumn[M, string]("code").Eq("parameter")
}
`
	writeCheckFile(t, filepath.Join(projectDir, "helper", "retention", "retention.go"), source)

	violations := CheckColumnReferenceMinting(newProjectIgnoreMatcher())

	// A concrete model has a generated Cols var wherever the call sits, and a
	// name that is no type parameter of the enclosing function denotes a
	// concrete type. A type parameter name that a type declaration in the body
	// reuses is distrusted throughout that body, outside the declaring block
	// too. Only the call naming an untouched type parameter is left alone.
	flagged := []string{`Eq("concrete")`, `Eq("package")`, `Eq("reused inside")`, `Eq("reused outside")`}
	if len(violations) != len(flagged) {
		t.Fatalf("expected %d violations, got %#v", len(flagged), violations)
	}
	for _, call := range flagged {
		location := filepath.Join("helper", "retention", "retention.go") + ":" + strconv.Itoa(sourceLine(t, source, call)) + ":"
		if !slices.ContainsFunc(violations, func(violation string) bool { return strings.HasPrefix(violation, location) }) {
			t.Fatalf("expected a violation at %s, got %#v", location, violations)
		}
	}
}
