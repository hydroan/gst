package ggcheck_test

import (
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

// TestServiceFileBoundariesCountsFrameworkServiceStructs pins which structs
// the check counts as service structs: those embedding the framework's
// service.Base, under whatever alias the package is imported, while a struct
// embedding the Base of the project's own service package is not one.
func TestServiceFileBoundariesCountsFrameworkServiceStructs(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n")

	// Two service structs in one file, the framework package imported under
	// an alias.
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "record.go"), `package record

import (
	svc "github.com/hydroan/gst/service"
	"tmpapp/model"
)

type Creator struct {
	svc.Base[*model.Record, *model.Record, *model.Record]
}

type Lister struct {
	svc.Base[*model.Record, *model.Record, *model.Record]
}
`)
	// Structs embedding the Base of the project's own service package are not
	// service structs of the framework.
	writeCheckFile(t, filepath.Join(projectDir, "service", "note", "note.go"), `package note

import (
	"tmpapp/model"
	"tmpapp/service"
)

type First struct {
	service.Base[*model.Note, *model.Note, *model.Note]
}

type Second struct {
	service.Base[*model.Note, *model.Note, *model.Note]
}
`)

	violations := runCheck(ggcheck.ServiceFileBoundaries)

	if len(violations) != 1 {
		t.Fatalf("expected one violation, got %#v", violations)
	}
	assertViolationContains(t, violations, filepath.Join("service", "record", "record.go"), "should contain at most one service struct (found: Creator, Lister)")
}
