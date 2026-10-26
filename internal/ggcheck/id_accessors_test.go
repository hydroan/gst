package ggcheck_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
	"github.com/hydroan/gst/internal/modelinspect"
)

// TestIDAccessorsRefusesAShadowedIDWithoutItsAccessors pins the check: a
// model declaring an ID of its own while keeping model.Base's ID accessors
// is reported under its file with the fix, before gg gen wrote any
// registration, and the same model declaring its own GetID, SetID and
// ClearID is clean.
func TestIDAccessorsRefusesAShadowedIDWithoutItsAccessors(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	// The check caches the column inspection under the user cache directory,
	// keyed by project directory; nothing reads a throwaway project's entry
	// again, so it goes when the test ends.
	cacheDir, err := modelinspect.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if removeErr := os.RemoveAll(cacheDir); removeErr != nil {
			t.Error(removeErr)
		}
	})
	writeCheckProjectGoModAgainstRealFramework(t, projectDir)
	writeCheckFile(t, filepath.Join(projectDir, "model", "record.go"), shadowedIDModel(""))

	violations := runCheck(ggcheck.IDAccessors)
	want := `model/record.go: model Record: its ID shadows model.Base's; declare its own GetID, SetID and ClearID too: model.Base's read and write the shadowed field, which no column maps`
	if len(violations) != 1 || violations[0] != want {
		t.Fatalf("violations = %#v, want the shadowed ID reported once under its file", violations)
	}

	writeCheckFile(t, filepath.Join(projectDir, "model", "record.go"), shadowedIDModel(ownIDAccessors))
	if violations := runCheck(ggcheck.IDAccessors); len(violations) != 0 {
		t.Fatalf("a model declaring its own ID accessors should have nothing to report, got %#v", violations)
	}
}

// ownIDAccessors declares the ID accessors of shadowedIDModel's Record
// against its own ID.
const ownIDAccessors = `
func (r *Record) GetID() string { return r.ID }

func (r *Record) SetID(id ...string) {
	if len(r.ID) == 0 && len(id) > 0 {
		r.ID = id[0]
	}
}

func (r *Record) ClearID() { r.ID = "" }
`

// shadowedIDModel is a table-backed model declaring an ID of its own over
// model.Base's, followed by the declarations in rest.
func shadowedIDModel(rest string) string {
	return `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Record is a record.
type Record struct {
	ID string ` + "`json:\"id\" gorm:\"primaryKey;size:191\"`" + `

	model.Base
}

func (Record) TableName() string { return "records" }

func (Record) Design() {
	dsl.Migrate()
	dsl.Endpoint("records")
	dsl.Create(func() {})
}
` + rest
}
