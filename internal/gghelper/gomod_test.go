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

			got, err := gghelper.ModulePath(".")

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

		if got, err := gghelper.ModulePath("."); err == nil {
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

	got, err := gghelper.ModulePath(".")
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

// TestModulePathOutsideAProject pins the error for a directory holding no
// go.mod, which says what gg runs in and what creates it: the current
// directory by that name, any other by its path.
func TestModulePathOutsideAProject(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	_, err := gghelper.ModulePath(".")
	if want := "no go.mod in the current directory: gg runs at the root of a gst project, which gg new creates"; err == nil || err.Error() != want {
		t.Fatalf("ModulePath(.) error = %v, want %q", err, want)
	}
	_, err = gghelper.ModulePath(dir)
	if want := "no go.mod in " + dir + ": gg runs at the root of a gst project, which gg new creates"; err == nil || err.Error() != want {
		t.Fatalf("ModulePath(%s) error = %v, want %q", dir, err, want)
	}
}

// TestFrameworkRequirement pins the example FrameworkRequirement gives, a
// go.mod requiring the framework and replacing it, and the answers around
// it: the version of a requirement not replaced, a replacement of another
// module that is none of the framework, and no version where
// RequiresFramework reports false.
func TestFrameworkRequirement(t *testing.T) {
	tests := []struct {
		name         string
		goMod        string
		wantVersion  string
		wantReplaced bool
	}{
		{name: "requires and replaces the framework", goMod: "module example.com/app\n\ngo 1.27\n\nrequire github.com/hydroan/gst v1.2.0\n\nreplace github.com/hydroan/gst => ../gst\n", wantVersion: "v1.2.0", wantReplaced: true},
		{name: "requires the framework", goMod: "module example.com/app\n\ngo 1.27\n\nrequire github.com/hydroan/gst v0.0.0-20260102030405-0123456789ab\n", wantVersion: "v0.0.0-20260102030405-0123456789ab"},
		{name: "replaces the framework in a block, its path quoted", goMod: "module example.com/app\n\ngo 1.27\n\nrequire github.com/hydroan/gst v1.2.0\n\nreplace (\n\texample.com/other => ../other\n\t\"github.com/hydroan/gst\" v1.2.0 => ../gst\n)\n", wantVersion: "v1.2.0", wantReplaced: true},
		{name: "replaces another module", goMod: "module example.com/app\n\ngo 1.27\n\nrequire (\n\tgithub.com/hydroan/gst v1.2.0\n\texample.com/other v1.0.0\n)\n\nreplace example.com/other => ../other\n", wantVersion: "v1.2.0"},
		{name: "requires no framework", goMod: "module example.com/app\n\ngo 1.27\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(tt.goMod), 0o600); err != nil {
				t.Fatal(err)
			}

			version, replaced, err := gghelper.FrameworkRequirement(dir)
			if err != nil {
				t.Fatalf("FrameworkRequirement() error = %v", err)
			}
			if version != tt.wantVersion || replaced != tt.wantReplaced {
				t.Fatalf("FrameworkRequirement() = %q, %t, want %q, %t", version, replaced, tt.wantVersion, tt.wantReplaced)
			}
		})
	}

	t.Run("without a go.mod", func(t *testing.T) {
		version, replaced, err := gghelper.FrameworkRequirement(t.TempDir())
		if err != nil || version != "" || replaced {
			t.Fatalf("FrameworkRequirement() = %q, %t, %v, want nothing without a go.mod", version, replaced, err)
		}
	})
}

func TestRequiresFramework(t *testing.T) {
	tests := []struct {
		name  string
		goMod string
		want  bool
	}{
		{name: "requires the framework", goMod: "module example.com/app\n\ngo 1.27\n\nrequire github.com/hydroan/gst v1.0.0\n", want: true},
		{name: "requires the framework next to a directive gg does not know", goMod: "module example.com/app\n\ngo 1.27\n\nnewdirective value\n\nrequire github.com/hydroan/gst v1.0.0\n", want: true},
		{name: "names the framework only in a comment", goMod: "// Started from github.com/hydroan/gst.\nmodule example.com/app\n\ngo 1.27\n", want: false},
		{name: "declares a module path extending the framework path", goMod: "module github.com/hydroan/gst-demo\n\ngo 1.27\n", want: false},
		{name: "requires a module whose path extends the framework path", goMod: "module example.com/app\n\ngo 1.27\n\nrequire github.com/hydroan/gst-extras v1.0.0\n", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(tt.goMod), 0o600); err != nil {
				t.Fatal(err)
			}

			got, err := gghelper.RequiresFramework(dir)
			if err != nil {
				t.Fatalf("RequiresFramework() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("RequiresFramework() = %t, want %t", got, tt.want)
			}
		})
	}

	t.Run("without a go.mod", func(t *testing.T) {
		got, err := gghelper.RequiresFramework(t.TempDir())
		if err != nil || got {
			t.Fatalf("RequiresFramework() = %t, %v, want false without a go.mod", got, err)
		}
	})

	t.Run("rejects a go.mod that does not parse", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n\nrequire github.com/hydroan/gst\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		if _, err := gghelper.RequiresFramework(dir); err == nil {
			t.Fatal("RequiresFramework() error = nil, want the parse failure")
		}
	})
}

// TestInWorkspace pins that the answer is the go command's own: the go.work
// file GOWORK names puts the working directory in workspace mode, GOWORK=off
// takes it out again, and a module on its own is in no workspace.
func TestInWorkspace(t *testing.T) {
	t.Run("a module on its own", func(t *testing.T) {
		t.Chdir(t.TempDir())
		writeProjectFile(t, "go.mod", "module tmpapp\n\ngo 1.27\n")
		t.Setenv("GOWORK", "")

		inWorkspace, err := gghelper.InWorkspace()
		if err != nil {
			t.Fatal(err)
		}
		if inWorkspace {
			t.Fatal("InWorkspace() = true for a module on its own, want false")
		}
	})

	t.Run("a workspace file", func(t *testing.T) {
		newWorkspaceProject(t)

		inWorkspace, err := gghelper.InWorkspace()
		if err != nil {
			t.Fatal(err)
		}
		if !inWorkspace {
			t.Fatal("InWorkspace() = false under a go.work file, want true")
		}
	})

	t.Run("GOWORK=off", func(t *testing.T) {
		newWorkspaceProject(t)
		t.Setenv("GOWORK", "off")

		inWorkspace, err := gghelper.InWorkspace()
		if err != nil {
			t.Fatal(err)
		}
		if inWorkspace {
			t.Fatal("InWorkspace() = true with GOWORK=off, want false")
		}
	})
}
