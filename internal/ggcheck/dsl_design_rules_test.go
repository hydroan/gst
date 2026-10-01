package ggcheck_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

// TestDSLDesignRulesRejectsBaseTypesEmbeddedThroughAPointer pins where gg
// check reports a base type embedded through a pointer: under the DSL design
// rules, which gate gg gen as well.
func TestDSLDesignRulesRejectsBaseTypesEmbeddedThroughAPointer(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.go"), `package record

import "github.com/hydroan/gst/model"

type Record struct {
	Name string

	*model.Base
}

func (Record) TableName() string { return "records" }
`)

	violations := runCheck(ggcheck.DSLDesignRules)

	if len(violations) != 1 || !strings.Contains(violations[0], "struct Record embeds *model.Base; embed model.Base by value") {
		t.Fatalf("expected the pointer-embedded base to be reported once, got %#v", violations)
	}
}

func TestDSLDesignRulesRejectsExactOnBuiltinIDActions(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "model", "iam", "session.go"), `package iam

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Session struct {
	model.Base
}

func (Session) Design() {
	Delete(func() {
		Service()
		Exact()
	})
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "iam", "current.go"), `package iam

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Current struct {
	model.Base
}

func (Current) Design() {
	Route("iam/sessions/current", func() {
		Get(func() {
			Service()
			Exact()
			Result[*CurrentGetRsp]()
		})
	})
}
`)

	violations := runCheck(ggcheck.DSLDesignRules)

	if len(violations) != 1 {
		t.Fatalf("expected exactly one violation, got %#v", violations)
	}
	if !strings.Contains(violations[0], "uses dsl.Exact() but relies on the built-in controller") {
		t.Fatalf("unexpected violation message: %q", violations[0])
	}
	if !strings.Contains(violations[0], filepath.Join("model", "iam", "session.go")) {
		t.Fatalf("violation should point to the offending file, got %q", violations[0])
	}
}

// TestDSLDesignRulesReportsTheRoutesTwoActionsRegisterAlike pins that gg
// check reports, under the DSL design rules, two actions registering one
// path, the way gg gen refuses them: model/token.go and model/api/token.go,
// whose endpoints both resolve to /api/tokens.
func TestDSLDesignRulesReportsTheRoutesTwoActionsRegisterAlike(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)
	for _, path := range []string{"model/token.go", "model/api/token.go"} {
		writeCheckFile(t, filepath.Join(projectDir, path), "package "+filepath.Base(filepath.Dir(path))+`

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Token struct {
	model.Base
}

func (Token) Design() {
	dsl.Endpoint("tokens")
	dsl.List(func() {})
}
`)
	}

	violations := runCheck(ggcheck.DSLDesignRules)

	want := []string{"model/token.go: the List action of Token registers GET /api/tokens, as the List action of Token in model/api/token.go does; a path is served by one action"}
	if !slices.Equal(violations, want) {
		t.Fatalf("violations = %q, want %q", violations, want)
	}
}
