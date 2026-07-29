package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/codegen"
	"github.com/hydroan/gst/internal/ggconfig"
)

// migratingModelSource declares a table-backed model with one List action,
// the shape of a module-copied framework model.
const migratingModelSource = `package user

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type User struct {
	model.Base
}

func (User) Design() {
	dsl.Migrate()
	dsl.Route("iam/admin/users", func() {
		dsl.List(func() {})
	})
}
`

func TestApplyModelIgnoresDisablesRegistration(t *testing.T) {
	models := findModelsFromSource(t, filepath.Join("iam", "user"), "user.go", migratingModelSource)
	rules := []ggconfig.ModelRule{{Name: "User", From: "model/iam", Raw: "User"}}

	result := applyModelIgnores(models, rules)

	if len(result.Matches) != 1 || result.Matches[0].Model != "User" {
		t.Fatalf("Matches = %+v, want exactly the User model", result.Matches)
	}
	m := models[0]
	if m.Design.Migrate {
		t.Fatal("Design.Migrate = true, want false after ignore")
	}
	if !m.RegisterIgnored {
		t.Fatal("RegisterIgnored = false, want true after ignore")
	}
	// The List action stays enabled, so the model is reported as live.
	if len(result.LiveActionModels) != 1 || result.LiveActionModels[0].Model != "User" {
		t.Fatalf("LiveActionModels = %+v, want the User model", result.LiveActionModels)
	}
	if len(result.Unmatched) != 0 {
		t.Fatalf("Unmatched = %+v, want empty", result.Unmatched)
	}
}

func TestApplyModelIgnoresScopesRuleByFromDirectory(t *testing.T) {
	models := findModelsFromSource(t, "admin", "user.go", `package admin

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type User struct {
	model.Base
}

func (User) Design() {
	dsl.Migrate()
}
`)
	rules := []ggconfig.ModelRule{{Name: "User", From: "model/iam", Raw: "User"}}

	result := applyModelIgnores(models, rules)

	if len(result.Matches) != 0 {
		t.Fatalf("Matches = %+v, want empty for an out-of-scope model", result.Matches)
	}
	if len(result.Unmatched) != 1 || result.Unmatched[0].Raw != "User" {
		t.Fatalf("Unmatched = %+v, want the User rule", result.Unmatched)
	}
	if !models[0].Design.Migrate {
		t.Fatal("Design.Migrate flipped for an out-of-scope model")
	}
}

func TestApplyModelIgnoresReportsRuleWithoutMigratingModel(t *testing.T) {
	// An action-only model never registers, so ignoring it has no effect and
	// the rule is reported as stale.
	models := findModelsFromSource(t, "probe", "probe.go", `package probe

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Probe struct {
	model.Empty
}

type ProbeListRsp struct {
	Total int64 `+"`json:\"total\"`"+`
}

func (Probe) Design() {
	dsl.Route("probes", func() {
		dsl.List(func() {
			dsl.Service()
			dsl.Flatten()
			dsl.Filename("probe_list.go")
			dsl.Result[*ProbeListRsp]()
		})
	})
}
`)
	rules := []ggconfig.ModelRule{{Name: "Probe", Raw: "Probe"}}

	result := applyModelIgnores(models, rules)

	if len(result.Matches) != 0 {
		t.Fatalf("Matches = %+v, want empty", result.Matches)
	}
	if len(result.Unmatched) != 1 {
		t.Fatalf("Unmatched = %+v, want the Probe rule", result.Unmatched)
	}
}

func TestApplyModelIgnoresReportsMultiDirectoryMatch(t *testing.T) {
	projectDir := t.TempDir()
	writeModelFixture(t, projectDir, filepath.Join("model", "iam", "user"), "user.go", migratingModelSource)
	writeModelFixture(t, projectDir, filepath.Join("model", "admin"), "user.go", `package admin

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type User struct {
	model.Base
}

func (User) Design() {
	dsl.Migrate()
}
`)
	t.Chdir(projectDir)
	models, err := codegen.FindModels("tmpapp", "model", "service", nil)
	if err != nil {
		t.Fatal(err)
	}
	buildHierarchicalEndpoints(models)
	propagateParentParams(models)

	result := applyModelIgnores(models, []ggconfig.ModelRule{{Name: "User", Raw: "User"}})

	if len(result.Matches) != 2 {
		t.Fatalf("len(Matches) = %d, want 2", len(result.Matches))
	}
	if len(result.MultiSourceRules) != 1 {
		t.Fatalf("MultiSourceRules = %+v, want one entry", result.MultiSourceRules)
	}
	wantDirs := []string{"model/admin", "model/iam"}
	if got := result.MultiSourceRules[0].Dirs; !slices.Equal(got, wantDirs) {
		t.Fatalf("MultiSourceRules[0].Dirs = %v, want %v", got, wantDirs)
	}
}

func TestApplyModelIgnoresAfterRouteIgnoresSeesDisabledActions(t *testing.T) {
	// Route ignores run first in genRunWithOptions; a model whose actions
	// were all disabled there must not be reported as live.
	models := findModelsFromSource(t, filepath.Join("iam", "user"), "user.go", migratingModelSource)
	routeRules := parseRules(t, "GET /api/iam/admin/users")
	if routeResult := applyRouteIgnores(models, routeRules); len(routeResult.Matches) != 1 {
		t.Fatalf("route Matches = %+v, want the List action", routeResult.Matches)
	}

	result := applyModelIgnores(models, []ggconfig.ModelRule{{Name: "User", From: "model/iam", Raw: "User"}})

	if len(result.Matches) != 1 {
		t.Fatalf("Matches = %+v, want the User model", result.Matches)
	}
	if len(result.LiveActionModels) != 0 {
		t.Fatalf("LiveActionModels = %+v, want empty after route ignores disabled every action", result.LiveActionModels)
	}
}

// writeModelFixture writes one model source file under projectDir/dir.
func writeModelFixture(t *testing.T, projectDir, dir, filename, source string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(projectDir, dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, dir, filename), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}
