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
