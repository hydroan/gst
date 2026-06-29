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

func TestRunModuleCopyGenKeepsQuietProjectChecks(t *testing.T) {
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

	if err := os.MkdirAll(filepath.Join(projectDir, "model", "copytest", "session"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module tmpapp\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "model", "copytest", "session", "sessions.go"), []byte(`package session

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Session2 struct {
	model.Empty
}

func (Session2) Design() {
	dsl.Route("copytest/sessions", func() {
		dsl.List(func() {
			dsl.Service()
		})
	})
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	if checks := runProjectChecksQuiet(); checks == 0 {
		t.Fatal("test fixture should fail project checks before module-copy generation")
	}
	module = ""

	err := runModuleCopyGen()
	if err == nil || !strings.Contains(err.Error(), "project checks failed") {
		t.Fatalf("runModuleCopyGen() error = %v, want project checks failed", err)
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
