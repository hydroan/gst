package ggmodule

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestBuildCopyPlanRejectsExcludedModelFileStillReferenced(t *testing.T) {
	projectDir := newModuleCopyPlanProject(t)
	writeCopyTestModuleSource(t, projectDir, []byte(`{"copy":{"excludeSourceFiles":["internal/model/copytest/naming.go"]}}`))
	sourceModelDir := filepath.Join(projectDir, "internal", "gst", "internal", "model", "copytest")
	if err := os.WriteFile(filepath.Join(sourceModelDir, "naming.go"), []byte(`package modelcopytest

const copyTestName = "copytest"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceModelDir, "uses_naming.go"), []byte(`package modelcopytest

var _ = copyTestName
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(projectDir)

	_, err := BuildCopyPlan("copytest", CopyOptions{})
	if err == nil {
		t.Fatal("BuildCopyPlan() succeeded, want an error for a referenced excluded model file")
	}
	for _, want := range []string{"excludeSourceFiles", "naming.go", "uses_naming.go"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("BuildCopyPlan() error = %v, want it to mention %q", err, want)
		}
	}
}

func TestBuildCopyPlanAllowsExcludedModelFileWithoutReferences(t *testing.T) {
	projectDir := newModuleCopyPlanProject(t)
	writeCopyTestModuleSource(t, projectDir, []byte(`{"copy":{"excludeSourceFiles":["internal/model/copytest/naming.go"]}}`))
	sourceModelDir := filepath.Join(projectDir, "internal", "gst", "internal", "model", "copytest")
	if err := os.WriteFile(filepath.Join(sourceModelDir, "naming.go"), []byte(`package modelcopytest

const copyTestName = "copytest"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(projectDir)

	plan, err := BuildCopyPlan("copytest", CopyOptions{})
	if err != nil {
		t.Fatalf("BuildCopyPlan() error = %v", err)
	}
	if slices.Contains(plan.ModelTargets(), filepath.Join("model", "copytest", "naming.go")) {
		t.Fatalf("ModelTargets() = %v, must not plan the excluded model file", plan.ModelTargets())
	}
}
