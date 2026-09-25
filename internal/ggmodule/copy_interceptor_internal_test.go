package ggmodule

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/gghelper"
)

// writeCopyTestInterceptor writes the interceptor file the copytest module
// declares under the fake framework root, the gRPC counterpart of the
// middleware the middleware tests declare.
func writeCopyTestInterceptor(t *testing.T, projectDir string) {
	t.Helper()
	frameworkRoot := filepath.Join(projectDir, "internal", "gst")
	if err := os.MkdirAll(filepath.Join(frameworkRoot, "interceptor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frameworkRoot, "interceptor", "copy_auth.go"), []byte(`package interceptor

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
}

// writeGRPCModel gives the project a model declaring GRPC(), which is what
// makes it a project serving gRPC.
func writeGRPCModel(t *testing.T, projectDir string) {
	t.Helper()
	path := filepath.Join(projectDir, "model", "record.go")
	if err := gghelper.EnsureParentDir(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Empty
}

func (Record) Design() {
	dsl.GRPC()
	dsl.Create(func() {})
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
}

const copyTestInterceptorManifest = `{
	"copy": {
		"interceptors": [
			{"sourceFile": "interceptor/copy_auth.go", "scope": "auth", "handler": "CopyAuth"}
		]
	}
}`

// TestBuildCopyPlanIncludesInterceptorFilesWhenTheProjectServesGRPC pins
// that a module's interceptors are copied into a project with a model
// declaring GRPC(): the file lands in the project's interceptor package
// under the ownership marker, its imports rewritten like a middleware file's.
func TestBuildCopyPlanIncludesInterceptorFilesWhenTheProjectServesGRPC(t *testing.T) {
	projectDir := newModuleCopyPlanProject(t)
	writeCopyTestModuleSource(t, projectDir, []byte(copyTestInterceptorManifest))
	writeCopyTestInterceptor(t, projectDir)
	writeGRPCModel(t, projectDir)
	t.Chdir(projectDir)

	plan, err := BuildCopyPlan("copytest", CopyOptions{})
	if err != nil {
		t.Fatalf("BuildCopyPlan() error = %v", err)
	}

	if !plan.ServesGRPC {
		t.Fatal("a project with a model declaring GRPC() serves gRPC")
	}
	target := filepath.Join("interceptor", "copy_auth.go")
	if targets := plan.InterceptorTargets(); !slices.Contains(targets, target) {
		t.Fatalf("InterceptorTargets() = %v, want %s", targets, target)
	}
	content := moduleCopyPlanFileContent(t, plan, target)
	if !strings.HasPrefix(content, moduleCopyMiddlewareMarker("copytest")+"\n\n") {
		t.Fatalf("copied interceptor must open with the ownership marker:\n%s", content)
	}
	for _, want := range []string{"package interceptor\n", `"tmpapp/model/copytest"`, `servicecopytest "tmpapp/service/copytest"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("copied interceptor missing %q:\n%s", want, content)
		}
	}
}

// TestBuildCopyPlanLeavesInterceptorsOutOfAProjectWithoutGRPC pins the
// other side: a project with no model declaring GRPC() gets no interceptor
// file, and one an earlier copy left there, carrying this module's marker,
// is stale and goes; the project's developers see no gRPC code at all.
func TestBuildCopyPlanLeavesInterceptorsOutOfAProjectWithoutGRPC(t *testing.T) {
	projectDir := newModuleCopyPlanProject(t)
	writeCopyTestModuleSource(t, projectDir, []byte(copyTestInterceptorManifest))
	writeCopyTestInterceptor(t, projectDir)
	leftover := filepath.Join(projectDir, "interceptor", "copy_auth.go")
	if err := gghelper.EnsureParentDir(leftover); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(leftover, []byte(moduleCopyMiddlewareMarker("copytest")+"\n\npackage interceptor\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(projectDir)

	plan, err := BuildCopyPlan("copytest", CopyOptions{})
	if err != nil {
		t.Fatalf("BuildCopyPlan() error = %v", err)
	}

	if plan.ServesGRPC {
		t.Fatal("a project without a model declaring GRPC() does not serve gRPC")
	}
	if targets := plan.InterceptorTargets(); len(targets) != 0 {
		t.Fatalf("InterceptorTargets() = %v, want none", targets)
	}
	want := []string{filepath.Join("interceptor", "copy_auth.go")}
	if stale := plan.StaleInterceptorTargets(); !slices.Equal(stale, want) {
		t.Fatalf("StaleInterceptorTargets() = %v, want %v", stale, want)
	}
}

// TestCopyExecutionCopiesInterceptorAndRegistersAuth pins the execution of
// a planned interceptor: the file is written as planned and its handler
// registered through the framework's interceptor package in the project's
// interceptor/interceptor.go, an existing init function kept.
func TestCopyExecutionCopiesInterceptorAndRegistersAuth(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, "interceptor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "interceptor", "interceptor.go"), []byte(`package interceptor

func init() {
	// keep existing comments
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(projectDir)

	source := []byte(`package interceptor

func CopyAuth() any {
	return nil
}
`)
	plan := &CopyPlan{
		Name:                 "copytest",
		ModelDir:             "model",
		ServiceDir:           "service",
		TargetMiddlewareDir:  "middleware",
		TargetInterceptorDir: "interceptor",
		ServesGRPC:           true,
		Files: []moduleCopyFile{{
			Kind:       moduleCopyFileInterceptor,
			TargetPath: filepath.Join("interceptor", "copy_auth.go"),
			Content:    source,
		}},
		Interceptors: []moduleCopyMiddleware{{
			SourcePath: filepath.Join("internal", "gst", "interceptor", "copy_auth.go"),
			TargetPath: filepath.Join("interceptor", "copy_auth.go"),
			Scope:      moduleCopyMiddlewareScopeAuth,
			Handler:    "CopyAuth",
		}},
	}
	exec := &CopyExecution{Plan: plan, RunGen: func() error { return nil }}

	if err := exec.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	copied, err := os.ReadFile(filepath.Join(projectDir, "interceptor", "copy_auth.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(copied) != string(source) {
		t.Fatalf("copied interceptor source changed:\n%s", copied)
	}
	registered, err := os.ReadFile(filepath.Join(projectDir, "interceptor", "interceptor.go"))
	if err != nil {
		t.Fatal(err)
	}
	code := string(registered)
	for _, want := range []string{`"github.com/hydroan/gst/interceptor"`, "interceptor.RegisterAuth(CopyAuth())", "keep existing comments"} {
		if !strings.Contains(code, want) {
			t.Fatalf("interceptor registration missing %q:\n%s", want, code)
		}
	}
}

// TestOrphanManagedFilesReadsTheInterceptorDirectory pins that the
// interceptor directory is judged like the middleware directory: a file
// carrying the marker of a module with no model directory is an orphan, the
// registration file interceptor.go never is.
func TestOrphanManagedFilesReadsTheInterceptorDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	for name, content := range map[string]string{
		"removed_auth.go": moduleCopyMiddlewareMarker("removed") + "\n\npackage interceptor\n",
		"kept_auth.go":    moduleCopyMiddlewareMarker("kept") + "\n\npackage interceptor\n",
		"interceptor.go":  moduleCopyMiddlewareMarker("removed") + "\n\npackage interceptor\n",
	} {
		path := filepath.Join("interceptor", name)
		if err := gghelper.EnsureParentDir(path); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join("model", "kept"), 0o755); err != nil {
		t.Fatal(err)
	}

	orphans, err := OrphanManagedFiles("interceptor", "model")
	if err != nil {
		t.Fatal(err)
	}
	want := []OrphanManagedFile{{Path: filepath.Join("interceptor", "removed_auth.go"), Module: "removed"}}
	if !slices.Equal(orphans, want) {
		t.Fatalf("OrphanManagedFiles() = %+v, want %+v", orphans, want)
	}
}

// TestRemoveManagedFilesRewritesTheInterceptorRegistration pins that
// deleting interceptor files takes their register calls with them, and the
// framework interceptor import once nothing uses it, the way middleware
// files are deleted.
func TestRemoveManagedFilesRewritesTheInterceptorRegistration(t *testing.T) {
	t.Chdir(t.TempDir())
	oldAuth := filepath.Join("interceptor", "old_auth.go")
	registration := filepath.Join("interceptor", "interceptor.go")
	if err := gghelper.EnsureParentDir(oldAuth); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldAuth, []byte(moduleCopyMiddlewareMarker("copytest")+"\n\npackage interceptor\n\nfunc OldAuth() any {\n\treturn nil\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registration, []byte("package interceptor\n\nimport \"github.com/hydroan/gst/interceptor\"\n\nfunc init() {\n\tinterceptor.RegisterAuth(OldAuth())\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var reported []string
	err := RemoveManagedFiles("interceptor", []string{oldAuth}, func(status CopyWriteStatus, path string) {
		reported = append(reported, string(status)+" "+path)
	})
	if err != nil {
		t.Fatal(err)
	}

	if want := []string{"DELETE " + oldAuth, "UPDATE " + registration}; !slices.Equal(reported, want) {
		t.Fatalf("reported = %q, want %q", reported, want)
	}
	code, err := os.ReadFile(registration)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(code), "OldAuth") || strings.Contains(string(code), `"github.com/hydroan/gst/interceptor"`) {
		t.Fatalf("the register call of the deleted interceptor and its import should be gone:\n%s", code)
	}
}

// TestCopyExecutionRemovesTheInterceptorPackageOnceTheProjectServesNoGRPC
// pins that a project which stopped serving gRPC is left with no gRPC code:
// the stale interceptor file goes with its register call, and the
// registration file that then registers nothing goes too, with the
// directory it leaves empty.
func TestCopyExecutionRemovesTheInterceptorPackageOnceTheProjectServesNoGRPC(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	stale := filepath.Join("interceptor", "copy_auth.go")
	registration := filepath.Join("interceptor", "interceptor.go")
	if err := gghelper.EnsureParentDir(stale); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte(moduleCopyMiddlewareMarker("copytest")+"\n\npackage interceptor\n\nfunc CopyAuth() any {\n\treturn nil\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registration, []byte("package interceptor\n\nimport \"github.com/hydroan/gst/interceptor\"\n\nfunc init() {\n\tinterceptor.RegisterAuth(CopyAuth())\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := &CopyPlan{
		Name:                  "copytest",
		ModelDir:              "model",
		ServiceDir:            "service",
		TargetMiddlewareDir:   "middleware",
		TargetInterceptorDir:  "interceptor",
		StaleInterceptorFiles: []string{stale},
	}
	exec := &CopyExecution{Plan: plan, RunGen: func() error { return nil }}

	if err := exec.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if _, err := os.Stat("interceptor"); !os.IsNotExist(err) {
		t.Fatalf("the interceptor directory should be gone with its last file; stat error = %v", err)
	}
}
