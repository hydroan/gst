package ggcheck_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

// TestProtobufDefinitionsReportsWhatStopsGeneration pins that the check
// derives the protobuf definitions the way gg gen does and reports each of
// its diagnostics, here a field of a model declaring GRPC() without a pb
// tag, and that a project without such a model has nothing to report.
func TestProtobufDefinitionsReportsWhatStopsGeneration(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoModAgainstRealFramework(t, projectDir)
	writeCheckFile(t, filepath.Join(projectDir, "model", "note.go"), `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Note is a note.
type Note struct {
	Title string `+"`"+`json:"title"`+"`"+`

	model.Base
}

func (Note) TableName() string { return "notes" }

func (Note) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("notes")
	dsl.Create(func() {})
}
`)

	violations := runCheck(ggcheck.ProtobufDefinitions)

	if len(violations) != 1 || !strings.Contains(violations[0], "tmpapp/model.Note.title: the field has no pb tag") {
		t.Fatalf("violations = %#v, want the missing pb tag reported once", violations)
	}

	writeCheckFile(t, filepath.Join(projectDir, "model", "note.go"), strings.Replace(`package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Note is a note.
type Note struct {
	Title string `+"`"+`json:"title"`+"`"+`

	model.Base
}

func (Note) TableName() string { return "notes" }

func (Note) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("notes")
	dsl.Create(func() {})
}
`, "\tdsl.GRPC()\n", "", 1))

	if violations := runCheck(ggcheck.ProtobufDefinitions); len(violations) != 0 {
		t.Fatalf("a project without a gRPC model should have nothing to report, got %#v", violations)
	}
}
