package ggcheck_test

import (
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

func TestServiceTestOrganizationAllowsPairedSpecialAndMarkedFiles(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "create.go"), "package record\n")
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "create_test.go"), `package record

import "testing"

func TestCreate(t *testing.T) { t.Log("ok") }
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "helper.go"), "package record\n")
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "helper_internal_test.go"), `package record

import "testing"

func TestHelperInternals(t *testing.T) { t.Log("ok") }
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "main_test.go"), `package record

import "testing"

func TestMain(m *testing.M) { m.Run() }
`)
	// Shared fixtures may declare helpers, which take more than the bare
	// *testing.T parameter and are not test cases.
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "fixtures_test.go"), `package record

import "testing"

func requireItem(t *testing.T, ok bool) { t.Helper() }
`)
	// A copyable framework module owns service/sample, so its stray test
	// files are the framework's business.
	writeFrameworkModuleFixture(t, projectDir, "sample")
	writeCheckFile(t, filepath.Join(projectDir, "service", "sample", "stray_test.go"), "package sample\n")

	violations := runCheck(ggcheck.ServiceTestOrganization)

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}

func TestServiceTestOrganizationFlagsUnpairedAndMisplacedTests(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "create.go"), "package record\n")
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "stray_test.go"), `package record

import "testing"

func TestStray(t *testing.T) { t.Log("ok") }
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "orphan_internal_test.go"), `package record

import "testing"

func TestOrphanInternals(t *testing.T) { t.Log("ok") }
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "main_test.go"), `package record

import "testing"

func TestMain(m *testing.M) { m.Run() }

func TestExtra(t *testing.T) { t.Log("ok") }
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "fixtures_test.go"), `package record

import "testing"

func TestSetup(t *testing.T) { t.Log("ok") }
`)

	violations := runCheck(ggcheck.ServiceTestOrganization)

	if len(violations) != 4 {
		t.Fatalf("expected four violations, got %#v", violations)
	}
	assertViolationContains(t, violations, filepath.Join("service", "record", "fixtures_test.go"), "TestSetup")
	assertViolationContains(t, violations, filepath.Join("service", "record", "main_test.go"), "TestExtra")
	assertViolationContains(t, violations, filepath.Join("service", "record", "orphan_internal_test.go"), "does not match any source file")
	assertViolationContains(t, violations, filepath.Join("service", "record", "stray_test.go"), "does not match any source file")
}
