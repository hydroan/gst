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

func readProjectModuleFile(t *testing.T, projectDir string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(projectDir, "module", "module.go"))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
