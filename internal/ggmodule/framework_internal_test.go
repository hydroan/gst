package ggmodule

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFrameworkSourceDir lays down a minimal directory that go list resolves
// as the framework module: a go.mod carrying the framework module path.
func writeFrameworkSourceDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+frameworkModulePath+"\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeProjectRequiringFramework writes a project whose go.mod requires the
// framework and resolves it through a replace directive, mirroring how real
// projects pin the framework in their module graph.
func writeProjectRequiringFramework(t *testing.T, frameworkDir string) string {
	t.Helper()
	projectDir := t.TempDir()
	goMod := "module tmpapp\n\ngo 1.26\n\nrequire " + frameworkModulePath + " v0.0.0-00010101000000-000000000000\n\nreplace " + frameworkModulePath + " => " + frameworkDir + "\n"
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte(goMod), 0o600); err != nil {
		t.Fatal(err)
	}
	return projectDir
}

// writeProjectWithoutFrameworkDependency writes a project whose go.mod does
// not require the framework at all.
func writeProjectWithoutFrameworkDependency(t *testing.T) string {
	t.Helper()
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module tmpapp\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return projectDir
}

func TestFindFrameworkRootResolvesFrameworkThroughGoModuleGraph(t *testing.T) {
	frameworkDir := writeFrameworkSourceDir(t)
	projectDir := writeProjectRequiringFramework(t, frameworkDir)
	t.Chdir(projectDir)

	got, err := findFrameworkRoot()
	if err != nil {
		t.Fatalf("findFrameworkRoot() error = %v", err)
	}
	gotResolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", got, err)
	}
	wantResolved, err := filepath.EvalSymlinks(frameworkDir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", frameworkDir, err)
	}
	if gotResolved != wantResolved {
		t.Fatalf("findFrameworkRoot() = %q, want %q", gotResolved, wantResolved)
	}
}

func TestFindFrameworkRootFailsWhenProjectDoesNotDependOnFramework(t *testing.T) {
	t.Chdir(writeProjectWithoutFrameworkDependency(t))

	_, err := findFrameworkRoot()
	if err == nil {
		t.Fatal("findFrameworkRoot() expected error for a project without the framework dependency")
	}
	if !strings.Contains(err.Error(), frameworkModulePath) {
		t.Fatalf("findFrameworkRoot() error %q should name the framework module %q", err, frameworkModulePath)
	}
}

func TestCopyableModuleNamesFailsWhenProjectDoesNotDependOnFramework(t *testing.T) {
	t.Chdir(writeProjectWithoutFrameworkDependency(t))

	names, err := CopyableModuleNames()
	if err == nil {
		t.Fatalf("CopyableModuleNames() = %v, expected error for a project without the framework dependency", names)
	}
}

func TestRequiredAssemblyFailsWhenProjectDoesNotDependOnFramework(t *testing.T) {
	t.Chdir(writeProjectWithoutFrameworkDependency(t))

	_, err := RequiredAssembly([]string{"sample"})
	if err == nil {
		t.Fatal("RequiredAssembly() expected error for a project without the framework dependency")
	}
}

func TestRequiredAssemblyWithoutModulesSkipsFrameworkResolution(t *testing.T) {
	t.Chdir(writeProjectWithoutFrameworkDependency(t))

	calls, err := RequiredAssembly(nil)
	if err != nil {
		t.Fatalf("RequiredAssembly(nil) error = %v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("RequiredAssembly(nil) = %v, want no calls", calls)
	}
}
