package ggcheck_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

// TestModelFileBoundariesFlagsFilesDeclaringSeveralModels pins the
// one-model-per-file rule: a model file declaring two model structs is
// flagged, while one declaring a model beside its request type is not.
func TestModelFileBoundariesFlagsFilesDeclaringSeveralModels(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), `package sample

import "github.com/hydroan/gst/model"

type Sample struct {
	model.Base
}

type SampleArchive struct {
	model.Base
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.go"), `package record

import "github.com/hydroan/gst/model"

type Record struct {
	model.Base
}

type RecordCreateReq struct {
	Name string `+"`json:\"name\"`"+`
}
`)

	violations := runCheck(ggcheck.ModelFileBoundaries)

	want := []string{"Model file '" + filepath.Join("model", "sample", "sample.go") + "' should contain at most one model struct (found: Sample, SampleArchive)"}
	if !slices.Equal(violations, want) {
		t.Fatalf("violations = %#v, want %#v", violations, want)
	}
}
