package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	runtimedebug "runtime/debug"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggmodule"
	"github.com/spf13/cobra"
)

func TestRunModuleListReportsCopyableModules(t *testing.T) {
	projectDir := newModuleListProjectWithFramework(t)
	writeModuleListFrameworkModule(t, projectDir, "copytest", "copytest", true)
	writeModuleListFrameworkModule(t, projectDir, "plain", "plain", false)
	t.Chdir(projectDir)

	var output bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&output)

	if err := runModuleList(cmd); err != nil {
		t.Fatalf("runModuleList() error = %v", err)
	}

	got := output.String()
	for _, want := range []string{
		"Framework Modules",
		"┌",
		"│",
		"└",
		"NAME",
		"PACKAGE",
		"ADD",
		"COPY",
		"IMPORT",
		"copytest",
		"yes",
		"plain",
		"no",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("module list output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "ADDABLE") || strings.Contains(got, "COPYABLE") {
		t.Fatalf("module list output should use short ADD/COPY columns:\n%s", got)
	}
	if !strings.Contains(got, "copytest") || !strings.Contains(got, "github.com/hydroan/gst/module/copytest") {
		t.Fatalf("module list output should mark copytest copyable:\n%s", got)
	}
	if !strings.Contains(got, "plain") || !strings.Contains(got, "github.com/hydroan/gst/module/plain") {
		t.Fatalf("module list output should mark plain non-copyable:\n%s", got)
	}
	for _, want := range []string{
		"│ NAME",
		" NAME ",
		"│ copytest",
		" copytest ",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("module list output should pad table cells, missing %q:\n%s", want, got)
		}
	}
}

func TestRunModuleListDoesNotDependOnGoPretty(t *testing.T) {
	buildInfo, ok := runtimedebug.ReadBuildInfo()
	if !ok {
		t.Fatal("read build info")
	}
	for _, dep := range buildInfo.Deps {
		if dep.Path == "github.com/jedib0t/go-pretty/v6" {
			t.Fatalf("cmd/gg should not depend on %s", dep.Path)
		}
	}
}

func TestPrintModuleCopyPlanReportsExtraTargetModelFilesAsWarningSection(t *testing.T) {
	plan := &ggmodule.CopyPlan{
		ExtraModelFiles: []string{filepath.Join("model", "authz", "design.go")},
	}

	output := captureStdout(t, func() {
		printModuleCopyPlan(plan)
	})

	for _, want := range []string{
		"Extra Target Model Files",
		filepath.Join("model", "authz", "design.go"),
		"not present in the framework source",
		"not deleted automatically",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "Extra target model files:") {
		t.Fatalf("output should not show extra model files as a normal plan group:\n%s", output)
	}
}

func TestPrintModuleCopyPlanReportsExtraTargetServiceFilesAsWarningSection(t *testing.T) {
	plan := &ggmodule.CopyPlan{
		ExtraServiceFiles: []string{filepath.Join("service", "mfa", "user_authenticator.go")},
	}

	output := captureStdout(t, func() {
		printModuleCopyPlan(plan)
	})

	for _, want := range []string{
		"Extra Target Service Files",
		filepath.Join("service", "mfa", "user_authenticator.go"),
		"not produced by this module copy plan",
		"not deleted automatically",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "Extra target service files:") {
		t.Fatalf("output should not show extra service files as a normal plan group:\n%s", output)
	}
}

func TestRunModuleCopyGenAllowsPreexistingProjectCheckViolations(t *testing.T) {
	projectDir := newModuleCopyGenProject(t)
	writePluralModelFile(t, projectDir, "session", "sessions.go", "Session2", "copytest/sessions")

	baseline := collectProjectCheckBaseline()
	if len(baseline) == 0 {
		t.Fatal("test fixture should fail project checks before module-copy generation")
	}
	module = ""

	var genErr error
	output := captureStdout(t, func() {
		genErr = runModuleCopyGen(baseline)
	})
	if genErr != nil {
		t.Fatalf("runModuleCopyGen() error = %v, want nil for pre-existing violations", genErr)
	}
	if strings.Contains(output, "sessions.go") {
		t.Fatalf("module-copy generation should not report pre-existing violations:\n%s", output)
	}
}

func TestRunModuleCopyGenFailsOnNewProjectCheckViolations(t *testing.T) {
	projectDir := newModuleCopyGenProject(t)
	writePluralModelFile(t, projectDir, "session", "sessions.go", "Session2", "copytest/sessions")

	baseline := collectProjectCheckBaseline()
	if len(baseline) == 0 {
		t.Fatal("test fixture should fail project checks before module-copy generation")
	}
	module = ""

	writePluralModelFile(t, projectDir, "account", "accounts.go", "Account2", "copytest/accounts")

	var genErr error
	output := captureStdout(t, func() {
		genErr = runModuleCopyGen(baseline)
	})
	if genErr == nil || !strings.Contains(genErr.Error(), "project checks failed") {
		t.Fatalf("runModuleCopyGen() error = %v, want project checks failed", genErr)
	}
	if !strings.Contains(output, "accounts.go") {
		t.Fatalf("module-copy generation should report the newly introduced violation:\n%s", output)
	}
	if strings.Contains(output, "sessions.go") {
		t.Fatalf("module-copy generation should not report pre-existing violations:\n%s", output)
	}
}

// newModuleCopyGenProject creates a temp gst project and points the gg command
// globals at it, so module-copy generation tests run against an isolated tree.
func newModuleCopyGenProject(t *testing.T) string {
	t.Helper()

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
	return projectDir
}

// writePluralModelFile writes a model file whose plural file name violates the
// singular-naming project check, giving tests a deterministic violation.
func writePluralModelFile(t *testing.T, projectDir, pkgName, fileName, modelName, route string) {
	t.Helper()

	dir := filepath.Join(projectDir, "model", "copytest", pkgName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(`package `+pkgName+`

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type `+modelName+` struct {
	model.Empty
}

func (`+modelName+`) Design() {
	dsl.Route("`+route+`", func() {
		dsl.List(func() {
			dsl.Service()
		})
	})
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writePipe

	fn()

	if closeErr := writePipe.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	os.Stdout = oldStdout

	output, err := io.ReadAll(readPipe)
	if err != nil {
		t.Fatal(err)
	}
	if closeErr := readPipe.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	return string(output)
}

func newModuleListProjectWithFramework(t *testing.T) string {
	t.Helper()

	projectDir := t.TempDir()
	for _, path := range []string{
		filepath.Join(projectDir, "module"),
		filepath.Join(projectDir, "internal", "gst"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module tmpapp\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "internal", "gst", "go.mod"), []byte("module github.com/hydroan/gst\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return projectDir
}

func writeModuleListFrameworkModule(t *testing.T, projectDir, name, packageName string, copyable bool) {
	t.Helper()

	moduleDir := filepath.Join(projectDir, "internal", "gst", "module", name)
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "register.go"), []byte("package "+packageName+"\n\nfunc Register() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if copyable {
		if err := os.WriteFile(filepath.Join(moduleDir, "module.json"), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
