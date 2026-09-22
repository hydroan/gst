package ggcheck_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

func TestServiceTestCoverageAllowsCoveredAndExemptServiceFiles(t *testing.T) {
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

	violations := runCheck(ggcheck.ServiceTestCoverage)

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}

func TestServiceTestCoverageFlagsServiceFilesWithoutTests(t *testing.T) {
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

	violations := runCheck(ggcheck.ServiceTestCoverage)

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
