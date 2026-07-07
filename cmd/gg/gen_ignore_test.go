package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/types/consts"
)

// Deviation from the brief: dsl.Design cannot be constructed directly with
// only the List/Get/Create fields set. dsl.Design.Range (via the unexported
// rangeAction helper) dereferences every action pointer unconditionally
// (e.g. d.Delete.Enabled), so any action field left nil by a hand-built
// Design literal panics. Only dsl.Parse (used by codegen.FindModels)
// initializes all twelve action pointers, so these two cases are built from
// temporary model files parsed through codegen.FindModels, per the brief's
// documented fallback.
func TestApplyRouteIgnoresDisablesDefaultEndpointActions(t *testing.T) {
	models := findModelsFromSource(t, filepath.Join("iam", "admin"), "users.go", `package admin

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
`)
	design := findDesign(t, models, "User")
	rules := parseRules(
		t,
		"GET /api/iam/admin/users",
		"GET /api/iam/admin/users/:id",
	)

	result := applyRouteIgnores(models, rules)

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

func TestApplyRouteIgnoresReportsUnmatchedRules(t *testing.T) {
	models := findModelsFromSource(t, "group", "group.go", `package model

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
`)
	design := findDesign(t, models, "Group")
	rules := parseRules(t, "POST /api/signup")

	result := applyRouteIgnores(models, rules)

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

func TestApplyRouteIgnoresDisablesNestedRouteActions(t *testing.T) {
	projectDir := t.TempDir()
	writeSignupModelFixture(t, projectDir)

	allModels, err := codegen.FindModels("tmpapp", filepath.Join(projectDir, "model"), filepath.Join(projectDir, "service"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(allModels) != 1 {
		t.Fatalf("len(allModels) = %d, want 1", len(allModels))
	}
	buildHierarchicalEndpoints(allModels)
	propagateParentParams(allModels)

	result := applyRouteIgnores(allModels, parseRules(t, "POST /api/signup"))

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
}

// writeSignupModelFixture writes a Signup model under projectDir/model/account
// whose Create action declares Service() with Filename("signup.go") on a
// nested "/signup" route. Shared by the nested-route ignore test and the
// prune-expectations test, both of which need the same service-bearing
// action to verify it disappears after an ignore rule matches it.
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

// TestApplyRouteIgnoresKeepsServiceFilesForPrune verifies that ignoring a
// service-bearing action records its service file and directory as kept, and
// that pruneServiceFiles honors the kept set: the file stays on disk even
// though the disabled action no longer contributes to currentServiceFiles.
// This keeps an ignored module route file-identical with gg module copy
// output instead of turning it into a deletion candidate.
func TestApplyRouteIgnoresKeepsServiceFilesForPrune(t *testing.T) {
	projectDir := t.TempDir()
	writeSignupModelFixture(t, projectDir)

	relModelDir := filepath.Join(projectDir, "model")
	relServiceDir := filepath.Join(projectDir, "service")
	allModels, err := codegen.FindModels("tmpapp", relModelDir, relServiceDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	buildHierarchicalEndpoints(allModels)
	propagateParentParams(allModels)

	// Resolve the service file the Signup action owns before ignoring it.
	var signupServiceFile string
	allModels[0].Design.Range(func(route string, act *dsl.Action) {
		if act.Service {
			signupServiceFile = gen.ServiceTarget(allModels[0], act, relModelDir, relServiceDir).FilePath
		}
	})
	if signupServiceFile == "" {
		t.Fatal("fixture should declare a service-bearing action")
	}

	oldModelDir, oldServiceDir := modelDir, serviceDir
	t.Cleanup(func() { modelDir, serviceDir = oldModelDir, oldServiceDir })
	modelDir, serviceDir = relModelDir, relServiceDir

	result := applyRouteIgnores(allModels, parseRules(t, "POST /api/signup"))

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
	pruneServiceFiles([]string{signupServiceFile}, allModels, result.KeptServiceFiles, result.KeptServiceDirs)
	if _, err := os.Stat(signupServiceFile); err != nil {
		t.Fatalf("ignored action's service file should survive prune: %v", err)
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

// findModelsFromSource writes source into a temporary project's model
// directory and scans it with codegen.FindModels, running the same
// endpoint/param setup genRunWithOptions performs before applyRouteIgnores.
// This is the fallback construction path documented in the task brief: a
// directly built dsl.Design leaves undeclared action fields nil, which
// panics inside dsl.Design.Range, so tests must go through the DSL parser.
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

	// buildHierarchicalEndpoints derives directory-based endpoints by
	// trimming a literal "model/" prefix from ModelFilePath, so modelDir
	// must be passed as a relative path (matching how gg gen invokes it
	// from the project root) rather than the absolute t.TempDir() path.
	t.Chdir(projectDir)

	allModels, err := codegen.FindModels("tmpapp", "model", "service", nil)
	if err != nil {
		t.Fatal(err)
	}
	buildHierarchicalEndpoints(allModels)
	propagateParentParams(allModels)
	return allModels
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

// TestGenRunAppliesRouteIgnoresFromGstYAML is an end-to-end test for the
// gst.yaml -> gg gen pipeline: it runs genRunWithOptions against a temporary
// project whose gst.yaml ignores one route, then asserts the generated
// router/router.go reflects that ignore (kept action registered, ignored
// action absent).
func TestGenRunAppliesRouteIgnoresFromGstYAML(t *testing.T) {
	// Save and restore gg global flags, same pattern as
	// TestRunModuleCopyGenKeepsQuietProjectChecks.
	oldModelDir := modelDir
	oldServiceDir := serviceDir
	oldRouterDir := routerDir
	oldDaoDir := daoDir
	oldExcludes := excludes
	oldModule := module
	oldPrune := prune
	oldCleanOrphans := cleanOrphans
	t.Cleanup(func() {
		modelDir = oldModelDir
		serviceDir = oldServiceDir
		routerDir = oldRouterDir
		daoDir = oldDaoDir
		excludes = oldExcludes
		module = oldModule
		prune = oldPrune
		cleanOrphans = oldCleanOrphans
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"
	serviceDir = "service"
	routerDir = "router"
	daoDir = "dao"
	excludes = nil
	module = ""
	prune = false
	cleanOrphans = false

	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module tmpapp\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "gst.yaml"), []byte(`version: 1
gen:
  routes:
    ignore:
      /api/tickets: [GET]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "model"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "model", "ticket.go"), []byte(`package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Ticket struct {
	model.Empty
}

func (Ticket) Design() {
	dsl.Route("tickets", func() {
		dsl.Create(func() {})
		dsl.List(func() {})
	})
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := genRunWithOptions(genRunOptions{Quiet: true}); err != nil {
		t.Fatalf("genRunWithOptions() error = %v", err)
	}

	routerCode, err := os.ReadFile(filepath.Join(projectDir, "router", "router.go"))
	if err != nil {
		t.Fatal(err)
	}
	// The kept action is registered.
	if !strings.Contains(string(routerCode), "consts.Create") {
		t.Errorf("router.go should register the tickets Create action:\n%s", routerCode)
	}
	// Business contract of gen.routes.ignore: the ignored route must not be
	// registered.
	if strings.Contains(string(routerCode), "consts.List") {
		t.Errorf("router.go must not register the ignored tickets List action:\n%s", routerCode)
	}
}
