package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// writeAssemblyFixtureFramework lays down a minimal framework source tree with
// one copyable module declaring one required assembly call. The check locates
// it the same way gg module copy does, through the project's go module graph.
func writeAssemblyFixtureFramework(t *testing.T, projectDir string) {
	t.Helper()

	frameworkRoot := filepath.Join(projectDir, "internal", "gst")
	writeCheckFile(t, filepath.Join(frameworkRoot, "go.mod"), "module github.com/hydroan/gst\n\ngo 1.26\n")
	writeCheckFile(t, filepath.Join(frameworkRoot, "module", "sample", "register.go"), "package sample\n\nfunc Register() {}\n")
	writeCheckFile(t, filepath.Join(frameworkRoot, "module", "sample", "module.json"), `{
		"copy": {
			"requiredAssembly": [
				{
					"import": "github.com/hydroan/gst/authn",
					"function": "SetSampleGate",
					"reason": "the sample gate stays off until it is installed"
				}
			]
		}
	}`)
}

// writeAssemblyFixtureProject writes a project that copied the sample module.
func writeAssemblyFixtureProject(t *testing.T, projectDir string) {
	t.Helper()

	writeCheckProjectGoMod(t, projectDir)
	writeAssemblyFixtureFramework(t, projectDir)
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), "package sample\n")
	writeCheckFile(t, filepath.Join(projectDir, "service", "sample", "gate.go"), "package sample\n")
}

func TestCheckModuleAssemblyReportsMissingCall(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeAssemblyFixtureProject(t, projectDir)

	violations := CheckModuleAssembly(newProjectIgnoreMatcher())

	if len(violations) != 1 {
		t.Fatalf("CheckModuleAssembly() = %v, want one violation", violations)
	}
	for _, want := range []string{"module sample", "authn.SetSampleGate", "the sample gate stays off"} {
		if !strings.Contains(violations[0], want) {
			t.Fatalf("violation %q missing %q", violations[0], want)
		}
	}
}

func TestCheckModuleAssemblyAcceptsAliasedCall(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeAssemblyFixtureProject(t, projectDir)

	// The qualifier resolves through the file's own import table, so an alias
	// satisfies the requirement exactly like the default package name.
	writeCheckFile(t, filepath.Join(projectDir, "module", "module.go"), `package module

import gstauthn "github.com/hydroan/gst/authn"

func init() {
	gstauthn.SetSampleGate(nil)
}
`)

	if violations := CheckModuleAssembly(newProjectIgnoreMatcher()); len(violations) != 0 {
		t.Fatalf("CheckModuleAssembly() = %v, want no violation", violations)
	}
}

func TestCheckModuleAssemblyRejectsSameNameFromAnotherPackage(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeAssemblyFixtureProject(t, projectDir)

	// Same function name, different package: the requirement is about the
	// framework gate, so a lookalike must not satisfy it.
	writeCheckFile(t, filepath.Join(projectDir, "module", "module.go"), `package module

import "tmpapp/service/sample"

func init() {
	sample.SetSampleGate(nil)
}
`)

	if violations := CheckModuleAssembly(newProjectIgnoreMatcher()); len(violations) != 1 {
		t.Fatalf("CheckModuleAssembly() = %v, want one violation", violations)
	}
}

func TestCheckModuleAssemblyIgnoresCallInsideCopiedModuleAndTests(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeAssemblyFixtureProject(t, projectDir)

	// Copied module code is linked only when generated registrations import
	// it, and test files never arm the binary; neither can satisfy the
	// requirement.
	call := `

import gstauthn "github.com/hydroan/gst/authn"

func init() {
	gstauthn.SetSampleGate(nil)
}
`
	writeCheckFile(t, filepath.Join(projectDir, "service", "sample", "gate.go"), "package sample\n"+call)
	writeCheckFile(t, filepath.Join(projectDir, "module", "module_test.go"), "package module\n"+call)

	if violations := CheckModuleAssembly(newProjectIgnoreMatcher()); len(violations) != 1 {
		t.Fatalf("CheckModuleAssembly() = %v, want one violation", violations)
	}
}

func TestCheckModuleAssemblySkipsModuleTheProjectNeverCopied(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)
	writeAssemblyFixtureFramework(t, projectDir)

	// No model/sample subtree means the module was never copied, so the
	// project owes nothing and no manifest is read.
	if violations := CheckModuleAssembly(newProjectIgnoreMatcher()); len(violations) != 0 {
		t.Fatalf("CheckModuleAssembly() = %v, want no violation", violations)
	}
}
