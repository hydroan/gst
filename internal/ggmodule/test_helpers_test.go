package ggmodule

import (
	"os"
	"path/filepath"
	"testing"
)

func newModuleCommandProject(t *testing.T) string {
	t.Helper()
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module tmpapp\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	moduleDir := filepath.Join(projectDir, "module")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "module.go"), []byte(`package module

func init() {
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	return projectDir
}

func newModuleCommandProjectWithFramework(t *testing.T) string {
	t.Helper()
	projectDir := newModuleCommandProject(t)
	writeFakeFrameworkModule(t, projectDir, "copytest", "copytest", "")
	writeFakeFrameworkModule(t, projectDir, "aliased", "aliasedmod", "")
	writeFakeFrameworkModule(t, projectDir, "configured", "configured", "config string")
	t.Chdir(projectDir)
	return projectDir
}

func writeFakeFrameworkModule(t *testing.T, projectDir string, name string, packageName string, registerParam string) {
	t.Helper()
	moduleDir := filepath.Join(projectDir, "internal", "gst", "module", name)
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	signature := "func Register() {}"
	if registerParam != "" {
		signature = "func Register(" + registerParam + ") {}"
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "register.go"), []byte("package "+packageName+"\n\n"+signature+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	frameworkMod := filepath.Join(projectDir, "internal", "gst", "go.mod")
	if err := os.MkdirAll(filepath.Dir(frameworkMod), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(frameworkMod, []byte("module github.com/hydroan/gst\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readProjectModuleFile(t *testing.T, projectDir string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(projectDir, "module", "module.go"))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
