package ggmodule

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddModuleRegistersFrameworkModule(t *testing.T) {
	projectDir := newModuleCommandProject(t)

	result, err := AddModule(projectDir, "mfa")
	if err != nil {
		t.Fatalf("AddModule() error = %v", err)
	}
	if result.Status != ChangeCreated {
		t.Fatalf("AddModule status = %s, want %s", result.Status, ChangeCreated)
	}

	content := readProjectModuleFile(t, projectDir)
	if !strings.Contains(content, `"github.com/hydroan/gst/module/mfa"`) {
		t.Fatalf("module.go missing mfa import:\n%s", content)
	}
	if !strings.Contains(content, "mfa.Register()") {
		t.Fatalf("module.go missing mfa.Register call:\n%s", content)
	}

	second, err := AddModule(projectDir, "mfa")
	if err != nil {
		t.Fatalf("second AddModule() error = %v", err)
	}
	if second.Status != ChangeSkipped {
		t.Fatalf("second AddModule status = %s, want %s", second.Status, ChangeSkipped)
	}
}

func TestAddModuleUsesAliasWhenPackageNameDiffers(t *testing.T) {
	projectDir := newModuleCommandProject(t)

	result, err := AddModule(projectDir, "version")
	if err != nil {
		t.Fatalf("AddModule(version) error = %v", err)
	}
	if result.Status != ChangeCreated {
		t.Fatalf("AddModule(version) status = %s, want %s", result.Status, ChangeCreated)
	}

	content := readProjectModuleFile(t, projectDir)
	if !strings.Contains(content, `versionmod "github.com/hydroan/gst/module/version"`) {
		t.Fatalf("module.go missing versionmod alias import:\n%s", content)
	}
	if !strings.Contains(content, "versionmod.Register()") {
		t.Fatalf("module.go missing versionmod.Register call:\n%s", content)
	}
}

func TestAddModuleKeepsExistingInitCommentsIntact(t *testing.T) {
	projectDir := newModuleCommandProject(t)
	moduleFile := filepath.Join(projectDir, "module", "module.go")
	if err := os.WriteFile(moduleFile, []byte(`// Package module provides business logic modules for the application.
//
// Recommended pattern:
//   - Organize each resource into its own subpackage under module/, e.g., module/user.
//   - Inside each subpackage, expose a Register() function that calls module.Use.
//   - Call these Register() functions from module.Init() to centralize startup.
//
// See module/helloworld for a complete example.
package module

func init() {
	// TODO: Call your module Register() functions here
	// Example:
	//   user.Register()
	//   order.Register()
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := AddModule(projectDir, "logmgmt")
	if err != nil {
		t.Fatalf("AddModule(logmgmt) error = %v", err)
	}
	if result.Status != ChangeCreated {
		t.Fatalf("AddModule(logmgmt) status = %s, want %s", result.Status, ChangeCreated)
	}

	content := readProjectModuleFile(t, projectDir)
	if !strings.Contains(content, "logmgmt.Register()") {
		t.Fatalf("module.go missing contiguous logmgmt.Register call:\n%s", content)
	}
	if strings.Contains(content, "logmgmt.\n") {
		t.Fatalf("module.go split selector from Register call:\n%s", content)
	}
	if !strings.Contains(content, "// TODO: Call your module Register() functions here") {
		t.Fatalf("module.go dropped existing init comments:\n%s", content)
	}
}

func TestAddModuleReusesExistingImportAlias(t *testing.T) {
	projectDir := newModuleCommandProject(t)
	moduleFile := filepath.Join(projectDir, "module", "module.go")
	if err := os.WriteFile(moduleFile, []byte(`package module

import mfamod "github.com/hydroan/gst/module/mfa"

func init() {
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := AddModule(projectDir, "mfa")
	if err != nil {
		t.Fatalf("AddModule() error = %v", err)
	}
	if result.Status != ChangeCreated {
		t.Fatalf("AddModule status = %s, want %s", result.Status, ChangeCreated)
	}

	content := readProjectModuleFile(t, projectDir)
	if !strings.Contains(content, "mfamod.Register()") {
		t.Fatalf("module.go missing existing alias Register call:\n%s", content)
	}
	if strings.Contains(content, "mfa.Register()") {
		t.Fatalf("module.go used package name instead of existing import alias:\n%s", content)
	}
}

func TestAddModuleRejectsLocalSourceCopy(t *testing.T) {
	projectDir := newModuleCommandProject(t)
	if err := os.MkdirAll(filepath.Join(projectDir, "model", "mfa"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "model", "mfa", "mfa.go"), []byte("package mfa\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := AddModule(projectDir, "mfa")
	if err == nil || !strings.Contains(err.Error(), "already exists as local source") {
		t.Fatalf("AddModule() error = %v, want local source conflict", err)
	}
}
