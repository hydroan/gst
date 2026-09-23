package main

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
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
	models, err := codegen.FindModels("tmpapp", "model", gghelper.NewProjectIgnore())
	if err != nil {
		t.Fatal(err)
	}
	codegen.ResolveRoutes(models, nil)

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
	if routeResult := codegen.ResolveRoutes(models, routeRules); len(routeResult.Matches) != 1 {
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

// writeSignupModelFixture writes a Signup model under projectDir/model/account
// whose Create action declares Service() with Filename("signup.go") on a
// nested "/signup" route, the shape of a module-copied framework action.
func writeSignupModelFixture(t *testing.T, projectDir string) {
	t.Helper()
	modelSource := `package account

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Signup struct {
	model.Empty
}

type SignupReq struct {
	Username string ` + "`json:\"username\"`" + `
}

type SignupRsp struct {
	UserID string ` + "`json:\"user_id\"`" + `
}

func (Signup) Design() {
	dsl.Route("/signup", func() {
		dsl.Create(func() {
			dsl.Service()
			dsl.Public()
			dsl.Filename("signup.go")
			dsl.Payload[*SignupReq]()
			dsl.Result[*SignupRsp]()
		})
	})
}
`
	fixtureModelDir := filepath.Join(projectDir, "model", "account")
	if err := os.MkdirAll(fixtureModelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureModelDir, "signup.go"), []byte(modelSource), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestRouteIgnoresKeepServiceFilesForPrune verifies the full route ignore
// contract on a nested-route service action: the action is disabled with its
// match reported, its service file and directory are recorded as kept, and
// pruneServiceFiles honors the kept set so the file stays on disk. This
// keeps an ignored module route file-identical with gg module copy output
// instead of turning it into a deletion candidate.
func TestRouteIgnoresKeepServiceFilesForPrune(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeSignupModelFixture(t, projectDir)

	allModels, err := codegen.FindModels("tmpapp", ggconst.DirModel, gghelper.NewProjectIgnore())
	if err != nil {
		t.Fatal(err)
	}
	if len(allModels) != 1 {
		t.Fatalf("len(allModels) = %d, want 1", len(allModels))
	}

	// Resolve the service file the Signup action owns before ignoring it.
	var signupServiceFile string
	allModels[0].Design.Range(func(route string, act *dsl.Action) {
		if act.Service {
			signupServiceFile = gen.ServiceTarget(allModels[0], act, ggconst.DirModel, ggconst.DirService).FilePath
		}
	})
	if signupServiceFile == "" {
		t.Fatal("fixture should declare a service-bearing action")
	}

	result := codegen.ResolveRoutes(allModels, parseRules(t, "POST /api/signup"))

	// The nested-route action is disabled and reported.
	if len(result.Matches) != 1 {
		t.Fatalf("len(Matches) = %d, want 1", len(result.Matches))
	}
	match := result.Matches[0]
	if match.Method != http.MethodPost || match.Path != "/api/signup" || match.Model != "Signup" {
		t.Fatalf("Matches[0] = %+v, want POST /api/signup (Signup)", match)
	}
	if remaining := collectActions(allModels[0].Design); len(remaining) != 0 {
		t.Fatalf("remaining actions = %d, want 0", len(remaining))
	}

	if !result.KeptServiceFiles[signupServiceFile] {
		t.Fatalf("KeptServiceFiles = %v, want %q kept", result.KeptServiceFiles, signupServiceFile)
	}
	if !result.KeptServiceDirs[filepath.Clean(filepath.Dir(signupServiceFile))] {
		t.Fatalf("KeptServiceDirs = %v, want %q kept", result.KeptServiceDirs, filepath.Dir(signupServiceFile))
	}

	// The ignored action drops out of the expected registration set...
	if expected := currentServiceFiles(allModels); len(expected) != 0 {
		t.Fatalf("currentServiceFiles = %v, want empty after ignore", expected)
	}

	// ...but pruneServiceFiles must keep the file on disk.
	if err := os.MkdirAll(filepath.Dir(signupServiceFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(signupServiceFile, []byte("package account\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pruneServiceFiles([]string{signupServiceFile}, allModels, result.KeptServiceFiles, result.KeptServiceDirs, gghelper.NewProjectIgnore())
	if _, err := os.Stat(signupServiceFile); err != nil {
		t.Fatalf("ignored action's service file should survive prune: %v", err)
	}
}

// TestGenRunAppliesRouteIgnoresFromGstYAML is an end-to-end test for the
// gst.yaml -> gg gen pipeline: it runs genRunWithOptions against a temporary
// project whose gst.yaml ignores one route, then asserts the generated
// router/router.gen.go reflects that ignore (kept action registered, ignored
// action absent).
func TestGenRunAppliesRouteIgnoresFromGstYAML(t *testing.T) {
	projectDir := newGenProject(t)
	if err := os.WriteFile(filepath.Join(projectDir, "gst.yaml"), []byte(`version: 1
gen:
  routes:
    ignore:
      /api/samples: [GET]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "model"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "model", "sample.go"), []byte(`package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Empty
}

type SampleListRsp struct{}

func (Sample) Design() {
	dsl.Route("samples", func() {
		dsl.Create(func() {})
		dsl.List(func() {
			dsl.Result[*SampleListRsp]()
		})
	})
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := genRunWithOptions(genRunOptions{Quiet: true}); err != nil {
		t.Fatalf("genRunWithOptions() error = %v", err)
	}

	routerCode, err := os.ReadFile(filepath.Join(projectDir, "router", ggconst.FileRouterGen))
	if err != nil {
		t.Fatal(err)
	}
	// The kept action is registered.
	if !strings.Contains(string(routerCode), "consts.Create") {
		t.Errorf("router.go should register the samples Create action:\n%s", routerCode)
	}
	// Business contract of gen.routes.ignore: the ignored route must not be
	// registered.
	if strings.Contains(string(routerCode), "consts.List") {
		t.Errorf("router.go must not register the ignored samples List action:\n%s", routerCode)
	}
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

// findModelsFromSource writes source into a temporary project's model
// directory, moves the test into the project and scans it with
// codegen.FindModels. Tests build their models this way because a directly
// built dsl.Design leaves undeclared action fields nil, which panics inside
// dsl.Design.Range, so they must go through the DSL parser; a test that needs
// the routes gg gen registers resolves them with codegen.ResolveRoutes, which
// reads the model file paths relative to the project root.
func findModelsFromSource(t *testing.T, pkgDir, filename, source string) []*gen.ModelInfo {
	t.Helper()
	projectDir := t.TempDir()
	fixtureModelDir := filepath.Join(projectDir, "model", pkgDir)
	if err := os.MkdirAll(fixtureModelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureModelDir, filename), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(projectDir)

	allModels, err := codegen.FindModels("tmpapp", "model", gghelper.NewProjectIgnore())
	if err != nil {
		t.Fatal(err)
	}
	return allModels
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
