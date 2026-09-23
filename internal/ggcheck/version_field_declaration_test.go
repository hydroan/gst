package ggcheck_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

func TestVersionFieldDeclaration(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)

	// An embedded Version and a bare named field are both deviations; a
	// compliant field and an unrelated local Version type are not.
	writeCheckFile(t, filepath.Join(projectDir, "model", "document", "document.go"), `package document

import "github.com/hydroan/gst/model"

type Embedded struct {
	model.Version

	model.Base
}

func (Embedded) TableName() string { return "embeddeds" }

type Bare struct {
	Version model.Version `+"`json:\"version\"`"+`

	model.Base
}

func (Bare) TableName() string { return "bares" }

type Partial struct {
	Version model.Version `+"`json:\"version\" gorm:\"not null\"`"+`

	model.Base
}

func (Partial) TableName() string { return "partials" }

type Compliant struct {
	Version model.Version `+"`json:\"version,omitempty\" gorm:\"not null;default:1\"`"+`

	model.Base
}

func (Compliant) TableName() string { return "compliant_rows" }

type Hidden struct {
	Version model.Version `+"`json:\"-\" gorm:\"not null;default:1\"`"+`

	model.Base
}

func (Hidden) TableName() string { return "hiddens" }
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "document", "aliased.go"), `package document

import gstmodel "github.com/hydroan/gst/model"

type Aliased struct {
	Revision gstmodel.Version `+"`json:\"revision\"`"+`

	gstmodel.Base
}

func (Aliased) TableName() string { return "aliaseds" }
`)
	// A dot import names the version and the base without a qualifier.
	writeCheckFile(t, filepath.Join(projectDir, "model", "document", "dotted.go"), `package document

import . "github.com/hydroan/gst/model"

type Dotted struct {
	Rev Version `+"`json:\"version\"`"+`

	Base
}

func (Dotted) TableName() string { return "dotteds" }
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "document", "unrelated.go"), `package document

type Version int64

type Unrelated struct {
	Value Version
}
`)
	// A request DTO carries model.Version so clients can hand the version
	// back; it is not a database model (no embedded base) and must not be
	// held to the gorm tag contract.
	writeCheckFile(t, filepath.Join(projectDir, "model", "document", "request.go"), `package document

import "github.com/hydroan/gst/model"

type UpdateReq struct {
	GroupID string        `+"`json:\"group_id\"`"+`
	Version model.Version `+"`json:\"version\"`"+`
}
`)

	violations := runCheck(ggcheck.VersionFieldDeclaration)

	require := func(substr string) {
		t.Helper()
		for _, violation := range violations {
			if strings.Contains(violation, substr) {
				return
			}
		}
		t.Fatalf("expected a violation containing %q, got %#v", substr, violations)
	}
	if len(violations) != 6 {
		t.Fatalf("expected 6 violations, got %#v", violations)
	}
	require("struct 'Embedded' embeds model.Version")
	require("field 'Bare.Version' (model.Version) is missing gorm not null, gorm default:1, json omitempty")
	require("field 'Partial.Version' (model.Version) is missing gorm default:1, json omitempty")
	require("field 'Aliased.Revision' (model.Version) is missing gorm not null, gorm default:1, json omitempty")
	require("field 'Dotted.Rev' (model.Version) is missing gorm not null, gorm default:1, json omitempty")
	require("field 'Hidden.Version' (model.Version) carries json:\"-\"")
}

func TestVersionFieldDeclarationActionTypes(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)

	// The Design-referenced request deviates by wire name, its nested item
	// type (reached through a slice of pointers) by a missing omitempty, and
	// the response — referenced only through a type alias — hides the field
	// entirely. The compliant request and the unreferenced stray DTO must
	// stay silent, and the model struct itself is left to the model-side
	// scan.
	writeCheckFile(t, filepath.Join(projectDir, "model", "note", "note.go"), `package note

import "github.com/hydroan/gst/model"

type Note struct {
	Version model.Version `+"`json:\"version,omitempty\" gorm:\"not null;default:1\"`"+`

	model.Base
}

func (Note) TableName() string { return "notes" }

func (Note) Design() {
	Update(func() {
		Payload[*NoteUpdateReq]()
		Result[*NoteRenameRsp]()
	})
	Patch(func() {
		Payload[*NotePatchReq]()
	})
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "note", "request.go"), `package note

import "github.com/hydroan/gst/model"

type NoteUpdateReq struct {
	Title   string        `+"`json:\"title\"`"+`
	Items   []*NoteItem   `+"`json:\"items\"`"+`
	Version model.Version `+"`json:\"revision,omitempty\"`"+`
}

type NoteItem struct {
	Content string        `+"`json:\"content\"`"+`
	Version model.Version `+"`json:\"version\"`"+`
}

type NoteUpdateRsp struct {
	Version model.Version `+"`json:\"-\"`"+`
}

type NoteRenameRsp = NoteUpdateRsp

type NotePatchReq struct {
	Version model.Version `+"`json:\"version,omitempty\"`"+`
}

type StrayReq struct {
	Version model.Version `+"`json:\"ver\"`"+`
}
`)

	violations := runCheck(ggcheck.VersionFieldDeclaration)

	require := func(substr string) {
		t.Helper()
		for _, violation := range violations {
			if strings.Contains(violation, substr) {
				return
			}
		}
		t.Fatalf("expected a violation containing %q, got %#v", substr, violations)
	}
	if len(violations) != 3 {
		t.Fatalf("expected 3 violations, got %#v", violations)
	}
	require(`field 'NoteUpdateReq.Version' (model.Version) in a DSL action type must carry json:"version,omitempty" (got json:"revision,omitempty")`)
	require(`field 'NoteItem.Version' (model.Version) in a DSL action type must carry json:"version,omitempty" (got json:"version")`)
	require(`field 'NoteUpdateRsp.Version' (model.Version) in a DSL action type carries json:"-"`)
}

// TestVersionFieldFindingsHealThroughTagInsertions pins what gg gen writes for
// a deviating model.Version field of a database model: a field without a tag
// gains the whole tag, and a partial tag gains only the settings it lacks.
// The two fields are the example of the doc comment of TagInsertions.
func TestVersionFieldFindingsHealThroughTagInsertions(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)
	samplePath := filepath.Join("model", "sample", "sample.go")
	writeCheckFile(t, filepath.Join(projectDir, samplePath), `package sample

import "github.com/hydroan/gst/model"

type Sample struct {
	model.Base
	Version model.Version
}
`)
	recordPath := filepath.Join("model", "record", "record.go")
	writeCheckFile(t, filepath.Join(projectDir, recordPath), `package record

import "github.com/hydroan/gst/model"

type Record struct {
	model.Base
	Revision model.Version `+"`json:\"revision\" gorm:\"not null\"`"+`
}
`)

	findings, err := ggcheck.VersionFieldFindings()
	if err != nil {
		t.Fatal(err)
	}

	healed := make(map[string]string, len(findings))
	for _, finding := range findings {
		source, readErr := os.ReadFile(finding.Path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		insertions := finding.TagInsertions()
		// Bottom-up, as gg gen applies them, keeps earlier offsets valid.
		slices.SortStableFunc(insertions, func(a, b ggcheck.TagInsertion) int { return b.Offset - a.Offset })
		for _, insertion := range insertions {
			source = slices.Insert(source, insertion.Offset, []byte(insertion.Text)...)
		}
		healed[finding.Path] = string(source)
	}

	want := map[string]string{
		samplePath: "\tVersion model.Version `json:\"version,omitempty\" gorm:\"not null;default:1\"`\n",
		recordPath: "\tRevision model.Version `json:\"revision,omitempty\" gorm:\"not null;default:1\"`\n",
	}
	if len(healed) != len(want) {
		t.Fatalf("healed files = %d, want %d: %v", len(healed), len(want), findings)
	}
	for path, line := range want {
		if !strings.Contains(healed[path], line) {
			t.Fatalf("healed %s = %q, want it to contain %q", path, healed[path], line)
		}
	}
}
