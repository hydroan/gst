package codegen_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/gghelper"
)

// TestResolveRoutesIgnoresDefaultEndpointActions verifies that route rules
// disable exactly the default-endpoint actions they match. Like every case in
// this file, it parses its models from source instead of building a
// dsl.Design literal: dsl.Design.Range (via the unexported rangeAction
// helper) dereferences every action pointer unconditionally (e.g.
// d.Delete.Enabled), so an action field a hand-built literal leaves nil
// panics, and only dsl.Parse, which codegen.FindModels runs, initializes all
// thirteen of them.
func TestResolveRoutesIgnoresDefaultEndpointActions(t *testing.T) {
	models := findModels(t, map[string]string{filepath.Join("model", "iam", "admin", "users.go"): `package admin

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type User struct {
	model.Base
}

func (User) Design() {
	dsl.Endpoint("users")
	dsl.Param("id")
	dsl.List(func() {})
	dsl.Get(func() {})
	dsl.Create(func() {})
}
`})
	design := findDesign(t, models, "User")
	rules := parseRules(
		t,
		"GET /api/iam/admin/users",
		"GET /api/iam/admin/users/:id",
	)

	result := codegen.ResolveRoutes(models, rules)

	if len(result.Matches) != 2 {
		t.Fatalf("len(Matches) = %d, want 2", len(result.Matches))
	}
	if len(result.Unmatched) != 0 {
		t.Fatalf("Unmatched = %v, want empty", result.Unmatched)
	}
	// The surviving action set is exactly the Create action.
	remaining := collectActions(design)
	if len(remaining) != 1 || remaining[0].Phase != consts.PHASE_CREATE {
		t.Fatalf("remaining actions = %v, want only PHASE_CREATE", remainingPhases(remaining))
	}
}

// TestResolveRoutesIgnoresStreamingAndExportRoutes verifies that rules match
// the SSE and export actions under GET, the method the framework router
// serves them by.
func TestResolveRoutesIgnoresStreamingAndExportRoutes(t *testing.T) {
	models := findModels(t, map[string]string{filepath.Join("model", "notice", "notice.go"): `package notice

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Notice struct {
	model.Base
}

func (Notice) Design() {
	dsl.Endpoint("notices")
	dsl.SSE(func() {
		dsl.Service()
	})
	dsl.Export(func() {
		dsl.Service()
	})
	dsl.Create(func() {})
}
`})
	design := findDesign(t, models, "Notice")
	rules := parseRules(
		t,
		"GET /api/notice/notices",
		"GET /api/notice/notices/export",
	)

	result := codegen.ResolveRoutes(models, rules)

	if len(result.Unmatched) != 0 {
		t.Fatalf("Unmatched = %v, want empty", result.Unmatched)
	}
	remaining := collectActions(design)
	if len(remaining) != 1 || remaining[0].Phase != consts.PHASE_CREATE {
		t.Fatalf("remaining actions = %v, want only PHASE_CREATE", remainingPhases(remaining))
	}
}

func TestResolveRoutesReportsUnmatchedIgnoreRules(t *testing.T) {
	models := findModels(t, map[string]string{filepath.Join("model", "group", "group.go"): `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Group struct {
	model.Base
}

func (Group) Design() {
	dsl.Endpoint("groups")
	dsl.List(func() {})
}
`})
	design := findDesign(t, models, "Group")
	rules := parseRules(t, "POST /api/signup")

	result := codegen.ResolveRoutes(models, rules)

	if len(result.Matches) != 0 {
		t.Fatalf("Matches = %v, want empty", result.Matches)
	}
	if len(result.Unmatched) != 1 || result.Unmatched[0].Raw != "POST /api/signup" {
		t.Fatalf("Unmatched = %v, want the signup rule", result.Unmatched)
	}
	if remaining := collectActions(design); len(remaining) != 1 {
		t.Fatalf("remaining actions = %d, want 1 (nothing disabled)", len(remaining))
	}
}

// TestResolveRoutesScopesIgnoreRuleByFromDirectory verifies the From
// contract: when a framework model and a project model declare the same
// route, a rule with From only disables the model under that directory, and
// a From-less rule disables both while reporting the multi-directory match
// so the user is warned about a likely swallowed re-declaration.
func TestResolveRoutesScopesIgnoreRuleByFromDirectory(t *testing.T) {
	sources := map[string]string{
		filepath.Join("model", "iam", "user", "user.go"): `package user

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type User struct {
	model.Base
}

func (User) Design() {
	dsl.Route("iam/admin/users", func() {
		dsl.List(func() {})
	})
}
`,
		filepath.Join("model", "admin", "admin.go"): `package admin

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Admin struct {
	model.Empty
}

type UserListRsp struct {
	Total int64 ` + "`json:\"total\"`" + `
}

func (Admin) Design() {
	dsl.Route("iam/admin/users", func() {
		dsl.List(func() {
			dsl.Service()
			dsl.Flatten()
			dsl.Filename("user_list.go")
			dsl.Result[*UserListRsp]()
		})
	})
}
`,
	}

	t.Run("from-scoped rule only disables the framework model", func(t *testing.T) {
		allModels := findModels(t, sources)
		if len(allModels) != 2 {
			t.Fatalf("len(allModels) = %d, want 2", len(allModels))
		}
		rules := parseRules(t, "GET /api/iam/admin/users")
		rules[0].From = "model/iam"

		result := codegen.ResolveRoutes(allModels, rules)

		if len(result.Matches) != 1 || result.Matches[0].Model != "User" {
			t.Fatalf("Matches = %+v, want exactly the framework User model", result.Matches)
		}
		if len(result.MultiSourceRules) != 0 {
			t.Fatalf("MultiSourceRules = %+v, want empty for a from-scoped rule", result.MultiSourceRules)
		}
		remaining := make([]string, 0, 1)
		for _, m := range allModels {
			m.Design.Range(func(route string, act *dsl.Action) {
				remaining = append(remaining, m.ModelName)
			})
		}
		if len(remaining) != 1 || remaining[0] != "Admin" {
			t.Fatalf("remaining models = %v, want only the project Admin model", remaining)
		}
	})

	t.Run("from-less rule disables both and reports multi-directory match", func(t *testing.T) {
		allModels := findModels(t, sources)
		if len(allModels) != 2 {
			t.Fatalf("len(allModels) = %d, want 2", len(allModels))
		}

		result := codegen.ResolveRoutes(allModels, parseRules(t, "GET /api/iam/admin/users"))

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
	})
}

// findModels writes sources, keyed by their paths under a temporary project,
// and returns the models codegen.FindModels finds in its model directory.
// The test moves into the project first, since ResolveRoutes reads model
// file paths relative to the project root, as gg gen passes them. Tests
// build their models this way because a directly built dsl.Design leaves
// undeclared action fields nil, which panics inside dsl.Design.Range, so
// they must go through the DSL parser.
func findModels(t *testing.T, sources map[string]string) []*gen.ModelInfo {
	t.Helper()
	projectDir := t.TempDir()
	for path, source := range sources {
		path = filepath.Join(projectDir, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(projectDir)

	models, err := codegen.FindModels("tmpapp", "model", gghelper.NewProjectIgnore())
	if err != nil {
		t.Fatal(err)
	}
	return models
}

// parseRules builds RouteRules from raw entries, failing the test on parse errors.
func parseRules(t *testing.T, raws ...string) []ggconfig.RouteRule {
	t.Helper()
	rules := make([]ggconfig.RouteRule, 0, len(raws))
	for _, raw := range raws {
		rule, err := ggconfig.ParseRouteRule(raw)
		if err != nil {
			t.Fatalf("ParseRouteRule(%q) error = %v", raw, err)
		}
		rules = append(rules, rule)
	}
	return rules
}

// findDesign returns the Design for the named model, failing the test if
// it is not present among models.
func findDesign(t *testing.T, models []*gen.ModelInfo, modelName string) *dsl.Design {
	t.Helper()
	for _, m := range models {
		if m.ModelName == modelName {
			return m.Design
		}
	}
	t.Fatalf("model %q not found", modelName)
	return nil
}

// collectActions returns the actions Design.Range still yields, i.e. the
// actions that remain enabled.
func collectActions(design *dsl.Design) []*dsl.Action {
	var actions []*dsl.Action
	design.Range(func(route string, act *dsl.Action) {
		actions = append(actions, act)
	})
	return actions
}

func remainingPhases(actions []*dsl.Action) []consts.Phase {
	phases := make([]consts.Phase, 0, len(actions))
	for _, act := range actions {
		phases = append(phases, act.Phase)
	}
	return phases
}
