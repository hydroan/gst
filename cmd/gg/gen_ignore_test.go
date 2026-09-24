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
	"github.com/hydroan/gst/internal/ggprune"
)

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
// pruneLeftovers honors the kept set so the file stays on disk. This
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

	// The ignored action drops out of the expected registration set, so
	// without the kept set its file would be planned for deletion...
	if plan := ggprune.PlanFiles([]string{signupServiceFile}, allModels, nil, ggconfig.PruneConfig{}); !slices.Equal(plan.Delete, []string{signupServiceFile}) {
		t.Fatalf("PlanFiles().Delete = %v, want %q after ignore", plan.Delete, signupServiceFile)
	}

	// ...but pruneLeftovers must keep the file on disk.
	if err := os.MkdirAll(filepath.Dir(signupServiceFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(signupServiceFile, []byte("package account\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pruneLeftovers([]string{signupServiceFile}, allModels, result.KeptServiceFiles, result.KeptServiceDirs, gghelper.NewProjectIgnore(), ggconfig.PruneConfig{})
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

// collectActions returns the actions Design.Range still yields, i.e. the
// actions that remain enabled.
func collectActions(design *dsl.Design) []*dsl.Action {
	var actions []*dsl.Action
	design.Range(func(route string, act *dsl.Action) {
		actions = append(actions, act)
	})
	return actions
}

// TestGenRunWarnsAboutConfigFilesItDoesNotRead pins that gg gen names every
// file next to gst.yaml that looks like gg configuration but is not read, so a
// setting written into one of them is not lost without a word.
func TestGenRunWarnsAboutConfigFilesItDoesNotRead(t *testing.T) {
	projectDir := newGenProject(t)
	writeProjectFile(t, filepath.Join(projectDir, "model", "sample", "record.go"), `package sample

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	Name string `+"`json:\"name\"`"+`

	model.Base
}

func (Record) TableName() string { return "records" }

func (Record) Design() {
	dsl.Migrate()
}
`)
	writeProjectFile(t, filepath.Join(projectDir, ".gg.yaml"), "prune:\n  ignore:\n    - service/sample\n")
	writeProjectFile(t, filepath.Join(projectDir, "gst.yml"), "version: 1\n")

	var genErr error
	stdout := captureStdout(t, func() {
		genErr = genRunWithOptions(genRunOptions{Quiet: true})
	})
	if genErr != nil {
		t.Fatal(genErr)
	}
	for _, want := range []string{"gg no longer reads .gg.yaml", "gg reads only gst.yaml, not gst.yml"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, stdout)
		}
	}
}
