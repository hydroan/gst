package gghelper_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/gghelper"
)

// TestModulePath pins what the module directive may look like: go itself
// accepts a deprecation comment after it and a quoted path, and both name the
// same module.
func TestModulePath(t *testing.T) {
	tests := []struct {
		name    string
		goMod   string
		want    string
		wantErr bool
	}{
		{name: "plain", goMod: "module example.com/app\n\ngo 1.27\n", want: "example.com/app"},
		{name: "deprecation comment", goMod: "module example.com/app // Deprecated: use example.com/app/v2\n\ngo 1.27\n", want: "example.com/app"},
		{name: "quoted path", goMod: "module \"example.com/app\"\n\ngo 1.27\n", want: "example.com/app"},
		{name: "no module directive", goMod: "go 1.27\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := os.WriteFile("go.mod", []byte(tt.goMod), 0o600); err != nil {
				t.Fatal(err)
			}

			got, err := gghelper.ModulePath()

			if tt.wantErr {
				if err == nil {
					t.Fatalf("ModulePath() = %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("ModulePath() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("without a go.mod", func(t *testing.T) {
		t.Chdir(t.TempDir())

		if got, err := gghelper.ModulePath(); err == nil {
			t.Fatalf("ModulePath() = %q, want an error", got)
		}
	})
}

// TestModulePathInWorkspaceReturnsCurrentModuleOnly pins that a go.work
// listing other modules does not leak them: the module path is the one the
// working directory's go.mod declares.
func TestModulePathInWorkspaceReturnsCurrentModuleOnly(t *testing.T) {
	dir := t.TempDir()
	appDir := filepath.Join(dir, "app")
	libDir := filepath.Join(dir, "lib")
	for moduleDir, moduleName := range map[string]string{appDir: "example.com/app", libDir: "example.com/lib"} {
		if err := os.MkdirAll(moduleDir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte("module "+moduleName+"\n\ngo 1.24\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	goWork := filepath.Join(dir, "go.work")
	if err := os.WriteFile(goWork, []byte("go 1.24\n\nuse (\n\t./app\n\t./lib\n)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(appDir)
	t.Setenv("GOWORK", goWork)

	got, err := gghelper.ModulePath()
	if err != nil {
		t.Fatal(err)
	}
	if got != "example.com/app" {
		t.Fatalf("ModulePath() = %q, want %q (a workspace must not leak other modules)", got, "example.com/app")
	}
}

// TestIsFrameworkProject pins that the framework repository is recognized by
// the module it declares, a deprecation comment included, and that a project
// depending on the framework is not one.
func TestIsFrameworkProject(t *testing.T) {
	tests := []struct {
		name  string
		goMod string
		want  bool
	}{
		{name: "framework", goMod: "module github.com/hydroan/gst\n\ngo 1.27\n", want: true},
		{name: "framework with a deprecation comment", goMod: "module github.com/hydroan/gst // Deprecated: nothing\n\ngo 1.27\n", want: true},
		{name: "project depending on the framework", goMod: "module example.com/app\n\nrequire github.com/hydroan/gst v1.0.0\n", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(tt.goMod), 0o600); err != nil {
				t.Fatal(err)
			}

			if got := gghelper.IsFrameworkProject(dir); got != tt.want {
				t.Fatalf("IsFrameworkProject() = %t, want %t", got, tt.want)
			}
		})
	}

	t.Run("without a go.mod", func(t *testing.T) {
		if gghelper.IsFrameworkProject(t.TempDir()) {
			t.Fatal("IsFrameworkProject() = true, want false without a go.mod")
		}
	})
}
