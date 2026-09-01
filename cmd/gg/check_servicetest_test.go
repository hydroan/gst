package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckServiceTestCoverageAllowsCoveredAndExemptServiceFiles(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckProjectGoMod(t, projectDir)

	// A covered default action and a covered custom-filename action.
	writeCheckFile(t, filepath.Join(projectDir, "model", "record.go"), `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	dsl.Create(func() {
		dsl.Service()
	})
	dsl.List(func() {})
	dsl.Route("/parse", func() {
		dsl.Create(func() {
			dsl.Service()
			dsl.Filename("parse")
		})
	})
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "create.go"), "package record\n")
	// The internal test form covers a service file on its own.
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "create_internal_test.go"), "package record\n")
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "parse.go"), "package record\n")
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "parse_test.go"), "package record\n")

	// A flattened action writes into the service package of the model package.
	writeCheckFile(t, filepath.Join(projectDir, "model", "pkg", "item.go"), `package pkg

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Item struct {
	model.Empty
}

func (Item) Design() {
	dsl.Route("/parse", func() {
		dsl.Create(func() {
			dsl.Service()
			dsl.Filename("flat")
			dsl.Flatten()
		})
	})
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "pkg", "flat.go"), "package pkg\n")
	writeCheckFile(t, filepath.Join(projectDir, "service", "pkg", "flat_test.go"), "package pkg\n")

	// A service file gg gen has not generated yet is gg gen's business.
	writeCheckFile(t, filepath.Join(projectDir, "model", "draft.go"), `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Draft struct {
	model.Base
}

func (Draft) Design() {
	dsl.Create(func() {
		dsl.Service()
	})
}
`)

	// A route-ignored action keeps its service file on disk without a route,
	// so no test can exercise it.
	writeCheckFile(t, filepath.Join(projectDir, "model", "legacy.go"), `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Legacy struct {
	model.Base
}

func (Legacy) Design() {
	dsl.Create(func() {
		dsl.Service()
	})
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "gst.yaml"), `version: 1
gen:
  routes:
    ignore:
      /api/legacies: [POST]
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "legacy", "create.go"), "package legacy\n")

	// A copyable framework module owns service/sample, and module code is
	// tested inside the framework repository.
	writeFrameworkModuleFixture(t, projectDir, "sample")
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "thing.go"), `package sample

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Thing struct {
	model.Base
}

func (Thing) Design() {
	dsl.Create(func() {
		dsl.Service()
	})
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "sample", "thing", "create.go"), "package thing\n")

	violations := CheckServiceTestCoverage(newProjectIgnoreMatcher())

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}

func TestCheckServiceTestCoverageFlagsServiceFilesWithoutTests(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckProjectGoMod(t, projectDir)
	writeCheckFile(t, filepath.Join(projectDir, "model", "record.go"), `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	dsl.Create(func() {
		dsl.Service()
	})
	dsl.Route("/parse", func() {
		dsl.Create(func() {
			dsl.Service()
			dsl.Filename("parse")
		})
	})
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "create.go"), "package record\n")
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "parse.go"), "package record\n")

	violations := CheckServiceTestCoverage(newProjectIgnoreMatcher())

	if len(violations) != 2 {
		t.Fatalf("expected two violations, got %#v", violations)
	}
	createPath := filepath.Join("service", "record", "create.go")
	if !strings.Contains(violations[0], createPath) || !strings.Contains(violations[0], "create_test.go") {
		t.Fatalf("expected missing create_test.go violation for %s, got %#v", createPath, violations)
	}
	parsePath := filepath.Join("service", "record", "parse.go")
	if !strings.Contains(violations[1], parsePath) || !strings.Contains(violations[1], "parse_test.go") {
		t.Fatalf("expected missing parse_test.go violation for %s, got %#v", parsePath, violations)
	}
}

func TestCheckServiceTestOrganizationAllowsPairedSpecialAndMarkedFiles(t *testing.T) {
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

	violations := CheckServiceTestOrganization(newProjectIgnoreMatcher())

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}

func TestCheckServiceTestOrganizationFlagsUnpairedAndMisplacedTests(t *testing.T) {
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

	violations := CheckServiceTestOrganization(newProjectIgnoreMatcher())

	if len(violations) != 4 {
		t.Fatalf("expected four violations, got %#v", violations)
	}
	assertViolationContains(t, violations, filepath.Join("service", "record", "fixtures_test.go"), "TestSetup")
	assertViolationContains(t, violations, filepath.Join("service", "record", "main_test.go"), "TestExtra")
	assertViolationContains(t, violations, filepath.Join("service", "record", "orphan_internal_test.go"), "does not match any source file")
	assertViolationContains(t, violations, filepath.Join("service", "record", "stray_test.go"), "does not match any source file")
}

// writeFrameworkModuleFixture writes a minimal framework source tree under
// projectDir/internal/gst declaring one copyable module, which is how
// CopyableModuleNames discovers module-owned service subtrees.
func writeFrameworkModuleFixture(t *testing.T, projectDir, name string) {
	t.Helper()

	frameworkDir := filepath.Join(projectDir, "internal", "gst")
	writeCheckFile(t, filepath.Join(frameworkDir, "go.mod"), "module github.com/hydroan/gst\n\ngo 1.26\n")
	writeCheckFile(t, filepath.Join(frameworkDir, "module", name, "register.go"), "package "+name+"\n\nfunc Register() {}\n")
	writeCheckFile(t, filepath.Join(frameworkDir, "module", name, "module.json"), "{}\n")
}

// assertViolationContains asserts that exactly one violation mentions path and
// that this violation also carries want.
func assertViolationContains(t *testing.T, violations []string, path, want string) {
	t.Helper()

	matched := make([]string, 0, 1)
	for _, violation := range violations {
		if strings.Contains(violation, path) {
			matched = append(matched, violation)
		}
	}
	if len(matched) != 1 {
		t.Fatalf("expected one violation for %s, got %#v", path, violations)
	}
	if !strings.Contains(matched[0], want) {
		t.Fatalf("expected violation for %s to contain %q, got %q", path, want, matched[0])
	}
}
