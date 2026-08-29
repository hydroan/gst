package ggmodule

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestBuildCopyPlanIncludesMiddlewareFiles(t *testing.T) {
	projectDir := newModuleCopyPlanProject(t)
	frameworkRoot := filepath.Join(projectDir, "internal", "gst")
	manifest := []byte(`{
		"copy": {
			"middleware": [
				{"sourceFile": "middleware/copy_auth.go", "scope": "auth", "handler": "CopyAuth"}
			]
		}
	}`)
	writeCopyTestModuleSource(t, projectDir, manifest)
	if err := os.MkdirAll(filepath.Join(frameworkRoot, "middleware"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frameworkRoot, "middleware", "copy_auth.go"), []byte(`package middleware

import (
	modelcopytest "github.com/hydroan/gst/internal/model/copytest"
	servicecopytest "github.com/hydroan/gst/internal/service/copytest"
)

func CopyAuth() any {
	_ = modelcopytest.CopyTest{}
	return servicecopytest.CopyAuthMarker()
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(projectDir)

	plan, err := BuildCopyPlan("copytest", CopyOptions{})
	if err != nil {
		t.Fatalf("BuildCopyPlan() error = %v", err)
	}

	targets := plan.MiddlewareTargets()
	if !slices.Contains(targets, filepath.Join("middleware", "copy_auth.go")) {
		t.Fatalf("MiddlewareTargets() = %v, want middleware/copy_auth.go", targets)
	}

	middleware := moduleCopyPlanFileContent(t, plan, filepath.Join("middleware", "copy_auth.go"))
	if !strings.HasPrefix(middleware, moduleCopyMiddlewareMarker("copytest")+"\n\n") {
		t.Fatalf("copied middleware must open with the ownership marker:\n%s", middleware)
	}
	for _, want := range []string{
		"package middleware\n",
		`"tmpapp/model/copytest"`,
		`servicecopytest "tmpapp/service/copytest"`,
		"_ = copytest.CopyTest{}",
		"return servicecopytest.CopyAuthMarker()",
	} {
		if !strings.Contains(middleware, want) {
			t.Fatalf("copied middleware missing %q:\n%s", want, middleware)
		}
	}
	if strings.Contains(middleware, "github.com/hydroan/gst/internal/model/copytest") || strings.Contains(middleware, "github.com/hydroan/gst/internal/service/copytest") || strings.Contains(middleware, "modelcopytest") {
		t.Fatalf("copied middleware leaked framework import artifacts:\n%s", middleware)
	}
}

// TestBuildCopyPlanRejectsMiddlewareHandlerWithArguments pins the plan-time
// guard on the handler signature. Registration is written into the project as
// handler(), so a handler taking arguments would copy cleanly and only fail
// when the copied project is compiled; the plan names the manifest instead.
func TestBuildCopyPlanRejectsMiddlewareHandlerWithArguments(t *testing.T) {
	projectDir := newModuleCopyPlanProject(t)
	frameworkRoot := filepath.Join(projectDir, "internal", "gst")
	manifest := []byte(`{
		"copy": {
			"middleware": [
				{"sourceFile": "middleware/copy_auth.go", "scope": "auth", "handler": "CopyAuth"}
			]
		}
	}`)
	writeCopyTestModuleSource(t, projectDir, manifest)
	if err := os.MkdirAll(filepath.Join(frameworkRoot, "middleware"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frameworkRoot, "middleware", "copy_auth.go"), []byte(`package middleware

func CopyAuth(burst int) any {
	return burst
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(projectDir)

	_, err := BuildCopyPlan("copytest", CopyOptions{})
	if err == nil {
		t.Fatal("BuildCopyPlan() error = nil, want a handler signature error")
	}
	if !strings.Contains(err.Error(), "must take no arguments") {
		t.Fatalf("BuildCopyPlan() error = %v, want it to report the handler signature", err)
	}
}

func TestBuildCopyPlanCollectsStaleMiddlewareFiles(t *testing.T) {
	projectDir := newModuleCopyPlanProject(t)
	frameworkRoot := filepath.Join(projectDir, "internal", "gst")
	manifest := []byte(`{
		"copy": {
			"middleware": [
				{"sourceFile": "middleware/copy_auth.go", "scope": "auth", "handler": "CopyAuth"}
			]
		}
	}`)
	writeCopyTestModuleSource(t, projectDir, manifest)
	if err := os.MkdirAll(filepath.Join(frameworkRoot, "middleware"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frameworkRoot, "middleware", "copy_auth.go"), []byte(`package middleware

func CopyAuth() any {
	return nil
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	projectMiddlewareDir := filepath.Join(projectDir, "middleware")
	if err := os.MkdirAll(projectMiddlewareDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(projectMiddlewareDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Owned by this module but no longer in the manifest: the only stale file.
	write("old_auth.go", moduleCopyMiddlewareMarker("copytest")+"\n\npackage middleware\n\nfunc OldAuth() any {\n\treturn nil\n}\n")
	// Owned by another module's copy: not this copy's business.
	write("other_module.go", moduleCopyMiddlewareMarker("othermod")+"\n\npackage middleware\n\nfunc OtherAuth() any {\n\treturn nil\n}\n")
	// Project-owned handler without a marker: never touched.
	write("project_own.go", "package middleware\n\nfunc ProjectOwn() any {\n\treturn nil\n}\n")
	// The registration file is project infrastructure, skipped even with a marker.
	write("middleware.go", moduleCopyMiddlewareMarker("copytest")+"\n\npackage middleware\n\nfunc init() {}\n")
	// Still declared by the manifest: a plan target, not stale.
	write("copy_auth.go", moduleCopyMiddlewareMarker("copytest")+"\n\npackage middleware\n\nfunc CopyAuth() any {\n\treturn nil\n}\n")

	t.Chdir(projectDir)

	plan, err := BuildCopyPlan("copytest", CopyOptions{Force: true})
	if err != nil {
		t.Fatalf("BuildCopyPlan() error = %v", err)
	}

	staleTargets := plan.StaleMiddlewareTargets()
	want := []string{filepath.Join("middleware", "old_auth.go")}
	if !slices.Equal(staleTargets, want) {
		t.Fatalf("StaleMiddlewareTargets() = %v, want %v", staleTargets, want)
	}
}

func TestCopyExecutionReconcilesMiddlewareRegistrationScopeChange(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, "middleware"), 0o755); err != nil {
		t.Fatal(err)
	}
	markedSource := moduleCopyMiddlewareMarker("copytest") + "\n\npackage middleware\n\nfunc CopyAuth() any {\n\treturn nil\n}\n"
	if err := os.WriteFile(filepath.Join(projectDir, "middleware", "copy_auth.go"), []byte(markedSource), 0o600); err != nil {
		t.Fatal(err)
	}
	// The manifest used to say scope "auth"; this copy switches it to
	// "global". ProjectOwn is not declared in any module-owned file, so its
	// registration must survive untouched.
	if err := os.WriteFile(filepath.Join(projectDir, "middleware", "middleware.go"), []byte(`package middleware

import "github.com/hydroan/gst/middleware"

func init() {
	middleware.RegisterAuth(CopyAuth())
	middleware.Register(ProjectOwn())
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(projectDir)

	plan := &CopyPlan{
		Name:                "copytest",
		ModelDir:            "model",
		ServiceDir:          "service",
		TargetMiddlewareDir: "middleware",
		Files: []moduleCopyFile{
			{
				Kind:        moduleCopyFileMiddleware,
				TargetPath:  filepath.Join("middleware", "copy_auth.go"),
				Content:     []byte(markedSource),
				Preexisting: true,
			},
		},
		Middleware: []moduleCopyMiddleware{
			{
				SourcePath: filepath.Join("internal", "gst", "middleware", "copy_auth.go"),
				TargetPath: filepath.Join("middleware", "copy_auth.go"),
				Scope:      moduleCopyMiddlewareScopeGlobal,
				Handler:    "CopyAuth",
			},
		},
	}
	exec := &CopyExecution{Plan: plan, Options: CopyOptions{Force: true}, RunGen: func() error { return nil }}
	if err := exec.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	registration, err := os.ReadFile(filepath.Join(projectDir, "middleware", "middleware.go"))
	if err != nil {
		t.Fatal(err)
	}
	code := string(registration)
	if !strings.Contains(code, "middleware.Register(CopyAuth())") {
		t.Fatalf("scope change must register the new scope:\n%s", code)
	}
	if strings.Contains(code, "middleware.RegisterAuth(CopyAuth())") {
		t.Fatalf("scope change must drop the old-scope registration:\n%s", code)
	}
	if !strings.Contains(code, "middleware.Register(ProjectOwn())") {
		t.Fatalf("project-owned registration must survive:\n%s", code)
	}

	rerun := &CopyExecution{Plan: plan, Options: CopyOptions{Force: true}, RunGen: func() error { return nil }}
	if rerunErr := rerun.Run(); rerunErr != nil {
		t.Fatalf("second Run() error = %v", rerunErr)
	}
	reread, rereadErr := os.ReadFile(filepath.Join(projectDir, "middleware", "middleware.go"))
	if rereadErr != nil {
		t.Fatal(rereadErr)
	}
	if string(reread) != code {
		t.Fatalf("reconciliation must be idempotent, second run changed the registration file:\n%s", reread)
	}
}

func TestCopyExecutionReconcilesMiddlewareRegistrationHandlerRename(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, "middleware"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldSource := moduleCopyMiddlewareMarker("copytest") + "\n\npackage middleware\n\nfunc OldAuth() any {\n\treturn nil\n}\n"
	if err := os.WriteFile(filepath.Join(projectDir, "middleware", "copy_auth.go"), []byte(oldSource), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "middleware", "middleware.go"), []byte(`package middleware

import "github.com/hydroan/gst/middleware"

func init() {
	middleware.RegisterAuth(OldAuth())
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(projectDir)

	newSource := moduleCopyMiddlewareMarker("copytest") + "\n\npackage middleware\n\nfunc NewAuth() any {\n\treturn nil\n}\n"
	plan := &CopyPlan{
		Name:                "copytest",
		ModelDir:            "model",
		ServiceDir:          "service",
		TargetMiddlewareDir: "middleware",
		Files: []moduleCopyFile{
			{
				Kind:        moduleCopyFileMiddleware,
				TargetPath:  filepath.Join("middleware", "copy_auth.go"),
				Content:     []byte(newSource),
				Preexisting: true,
			},
		},
		Middleware: []moduleCopyMiddleware{
			{
				SourcePath: filepath.Join("internal", "gst", "middleware", "copy_auth.go"),
				TargetPath: filepath.Join("middleware", "copy_auth.go"),
				Scope:      moduleCopyMiddlewareScopeAuth,
				Handler:    "NewAuth",
			},
		},
	}
	exec := &CopyExecution{Plan: plan, Options: CopyOptions{Force: true}, RunGen: func() error { return nil }}
	if err := exec.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	registration, err := os.ReadFile(filepath.Join(projectDir, "middleware", "middleware.go"))
	if err != nil {
		t.Fatal(err)
	}
	code := string(registration)
	if !strings.Contains(code, "middleware.RegisterAuth(NewAuth())") {
		t.Fatalf("handler rename must register the new handler:\n%s", code)
	}
	if strings.Contains(code, "OldAuth") {
		t.Fatalf("handler rename must drop the old handler registration:\n%s", code)
	}
}

func TestCopyExecutionPrunesStaleMiddlewareAndItsRegistration(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, "middleware"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(projectDir, "middleware", name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("old_auth.go", moduleCopyMiddlewareMarker("copytest")+"\n\npackage middleware\n\nfunc OldAuth() any {\n\treturn nil\n}\n")
	write("middleware.go", `package middleware

import "github.com/hydroan/gst/middleware"

func init() {
	middleware.RegisterAuth(OldAuth())
	middleware.RegisterAuth(ProjectOwn())
}
`)

	t.Chdir(projectDir)

	staleMiddleware := filepath.Join("middleware", "old_auth.go")
	exec := &CopyExecution{
		Plan: &CopyPlan{
			Name:                 "copytest",
			ModelDir:             "model",
			ServiceDir:           "service",
			TargetMiddlewareDir:  "middleware",
			StaleMiddlewareFiles: []string{staleMiddleware},
		},
		RunGen: func() error { return nil },
	}

	if err := exec.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if fileExists(staleMiddleware) {
		t.Fatalf("Run() left stale middleware %s in place", staleMiddleware)
	}
	if !slices.Contains(exec.DeletedFiles, staleMiddleware) {
		t.Fatalf("DeletedFiles = %v, want %s", exec.DeletedFiles, staleMiddleware)
	}

	registration, err := os.ReadFile(filepath.Join(projectDir, "middleware", "middleware.go"))
	if err != nil {
		t.Fatal(err)
	}
	code := string(registration)
	if strings.Contains(code, "OldAuth") {
		t.Fatalf("pruned middleware registration call survived:\n%s", code)
	}
	if !strings.Contains(code, "middleware.RegisterAuth(ProjectOwn())") {
		t.Fatalf("unrelated middleware registration must survive:\n%s", code)
	}
}

func TestCopyExecutionCopiesMiddlewareAndRegistersAuth(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, "middleware"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "middleware", "middleware.go"), []byte(`package middleware

func init() {
	// keep existing comments
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(projectDir)

	source := []byte(`package middleware

func CopyAuth() any {
	return nil
}
`)
	plan := &CopyPlan{
		Name:                "copytest",
		ModelDir:            "model",
		ServiceDir:          "service",
		TargetMiddlewareDir: "middleware",
		Files: []moduleCopyFile{
			{
				Kind:       moduleCopyFileMiddleware,
				TargetPath: filepath.Join("middleware", "copy_auth.go"),
				Content:    source,
			},
		},
		Middleware: []moduleCopyMiddleware{
			{
				SourcePath: filepath.Join("internal", "gst", "middleware", "copy_auth.go"),
				TargetPath: filepath.Join("middleware", "copy_auth.go"),
				Scope:      moduleCopyMiddlewareScopeAuth,
				Handler:    "CopyAuth",
			},
		},
	}
	exec := &CopyExecution{
		Plan:    plan,
		Options: CopyOptions{},
		RunGen: func() error {
			return nil
		},
	}

	if err := exec.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	copied, err := os.ReadFile(filepath.Join(projectDir, "middleware", "copy_auth.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(copied) != string(source) {
		t.Fatalf("copied middleware source changed:\n%s", copied)
	}

	registered, err := os.ReadFile(filepath.Join(projectDir, "middleware", "middleware.go"))
	if err != nil {
		t.Fatal(err)
	}
	code := string(registered)
	if !strings.Contains(code, `"github.com/hydroan/gst/middleware"`) {
		t.Fatalf("middleware registration import missing:\n%s", code)
	}
	if !strings.Contains(code, "middleware.RegisterAuth(CopyAuth())") {
		t.Fatalf("auth middleware registration missing:\n%s", code)
	}
	if strings.Contains(code, "gstmiddleware") {
		t.Fatalf("middleware registration used an unnecessary alias:\n%s", code)
	}
}
