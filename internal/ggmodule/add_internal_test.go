package ggmodule

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddModuleRegistersFrameworkModule(t *testing.T) {
	projectDir := newModuleCommandProjectWithFramework(t)

	result, err := AddModule(projectDir, "copytest")
	if err != nil {
		t.Fatalf("AddModule() error = %v", err)
	}
	if result.Status != ChangeCreated {
		t.Fatalf("AddModule status = %s, want %s", result.Status, ChangeCreated)
	}

	content := readProjectModuleFile(t, projectDir)
	if !strings.Contains(content, `"github.com/hydroan/gst/module/copytest"`) {
		t.Fatalf("module.go missing copytest import:\n%s", content)
	}
	if !strings.Contains(content, "copytest.Register()") {
		t.Fatalf("module.go missing copytest.Register call:\n%s", content)
	}

	second, err := AddModule(projectDir, "copytest")
	if err != nil {
		t.Fatalf("second AddModule() error = %v", err)
	}
	if second.Status != ChangeSkipped {
		t.Fatalf("second AddModule status = %s, want %s", second.Status, ChangeSkipped)
	}
}

func TestAddModuleUsesAliasWhenPackageNameDiffers(t *testing.T) {
	projectDir := newModuleCommandProjectWithFramework(t)

	result, err := AddModule(projectDir, "aliased")
	if err != nil {
		t.Fatalf("AddModule(aliased) error = %v", err)
	}
	if result.Status != ChangeCreated {
		t.Fatalf("AddModule(aliased) status = %s, want %s", result.Status, ChangeCreated)
	}

	content := readProjectModuleFile(t, projectDir)
	if !strings.Contains(content, `aliasedmod "github.com/hydroan/gst/module/aliased"`) {
		t.Fatalf("module.go missing aliasedmod alias import:\n%s", content)
	}
	if !strings.Contains(content, "aliasedmod.Register()") {
		t.Fatalf("module.go missing aliasedmod.Register call:\n%s", content)
	}
}

func TestAddModuleKeepsExistingInitCommentsIntact(t *testing.T) {
	projectDir := newModuleCommandProjectWithFramework(t)
	moduleFile := filepath.Join(projectDir, "module", "module.go")
	if err := os.WriteFile(moduleFile, []byte(`// Package module provides business logic modules for the application.
//
// Recommended pattern:
//   - Organize each resource into its own subpackage under module/, e.g., module/sample.
//   - Inside each subpackage, expose a Register() function that calls module.Use.
//   - Call these Register() functions from module.Init() to centralize startup.
//
// See module/copytest for a complete example.
package module

func init() {
	// TODO: Call your module Register() functions here
	// Example:
	//   sample.Register()
	//   record.Register()
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := AddModule(projectDir, "copytest")
	if err != nil {
		t.Fatalf("AddModule(copytest) error = %v", err)
	}
	if result.Status != ChangeCreated {
		t.Fatalf("AddModule(copytest) status = %s, want %s", result.Status, ChangeCreated)
	}

	content := readProjectModuleFile(t, projectDir)
	if !strings.Contains(content, "copytest.Register()") {
		t.Fatalf("module.go missing contiguous copytest.Register call:\n%s", content)
	}
	if strings.Contains(content, "copytest.\n") {
		t.Fatalf("module.go split selector from Register call:\n%s", content)
	}
	if !strings.Contains(content, "// TODO: Call your module Register() functions here") {
		t.Fatalf("module.go dropped existing init comments:\n%s", content)
	}
}

// TestAddModuleKeepsTheOwnCodeOfModuleGo pins how gg module add writes
// module/module.go back: the way gopls saves it, so the project's own code
// keeps the form it was written in, two parameters of one type with both
// types and a bare return.
func TestAddModuleKeepsTheOwnCodeOfModuleGo(t *testing.T) {
	projectDir := newModuleCommandProjectWithFramework(t)
	moduleFile := filepath.Join(projectDir, "module", "module.go")
	if err := os.WriteFile(moduleFile, []byte(`package module

func init() {
}

func join(a string, b string) (s string) {
	s = a + b
	return
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := AddModule(projectDir, "copytest"); err != nil {
		t.Fatalf("AddModule(copytest) error = %v", err)
	}

	content := readProjectModuleFile(t, projectDir)
	if !strings.Contains(content, "copytest.Register()") {
		t.Fatalf("module.go missing the copytest.Register call:\n%s", content)
	}
	if want := "func join(a string, b string) (s string) {\n\ts = a + b\n\treturn\n}\n"; !strings.Contains(content, want) {
		t.Fatalf("module.go rewrote the project's own code:\n%s", content)
	}
}

func TestAddModuleReusesExistingImportAlias(t *testing.T) {
	projectDir := newModuleCommandProjectWithFramework(t)
	moduleFile := filepath.Join(projectDir, "module", "module.go")
	if err := os.WriteFile(moduleFile, []byte(`package module

import copytestmod "github.com/hydroan/gst/module/copytest"

func init() {
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := AddModule(projectDir, "copytest")
	if err != nil {
		t.Fatalf("AddModule() error = %v", err)
	}
	if result.Status != ChangeCreated {
		t.Fatalf("AddModule status = %s, want %s", result.Status, ChangeCreated)
	}

	content := readProjectModuleFile(t, projectDir)
	if !strings.Contains(content, "copytestmod.Register()") {
		t.Fatalf("module.go missing existing alias Register call:\n%s", content)
	}
	if strings.Contains(content, "copytest.Register()") {
		t.Fatalf("module.go used package name instead of existing import alias:\n%s", content)
	}
}

func TestAddModuleRejectsLocalSourceCopy(t *testing.T) {
	projectDir := newModuleCommandProjectWithFramework(t)
	if err := os.MkdirAll(filepath.Join(projectDir, "model", "copytest"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "model", "copytest", "copytest.go"), []byte("package copytest\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := AddModule(projectDir, "copytest")
	if err == nil || !strings.Contains(err.Error(), "already exists as local source") {
		t.Fatalf("AddModule() error = %v, want local source conflict", err)
	}
}
