package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckVersionFieldDeclarations(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() { modelDir = oldModelDir })

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)
	modelDir = "model"

	// An embedded Version and a bare named field are both deviations; a
	// compliant field and an unrelated local Version type are not.
	writeCheckFile(t, filepath.Join(projectDir, "model", "config", "config.go"), `package config

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
	writeCheckFile(t, filepath.Join(projectDir, "model", "config", "aliased.go"), `package config

import gstmodel "github.com/hydroan/gst/model"

type Aliased struct {
	Revision gstmodel.Version `+"`json:\"revision\"`"+`

	gstmodel.Base
}

func (Aliased) TableName() string { return "aliaseds" }
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "config", "unrelated.go"), `package config

type Version int64

type Unrelated struct {
	Value Version
}
`)
	// A request DTO carries model.Version so clients can hand the version
	// back; it is not a database model (no embedded base) and must not be
	// held to the gorm tag contract.
	writeCheckFile(t, filepath.Join(projectDir, "model", "config", "request.go"), `package config

import "github.com/hydroan/gst/model"

type UpdateReq struct {
	GroupID string        `+"`json:\"group_id\"`"+`
	Version model.Version `+"`json:\"version\"`"+`
}
`)

	violations := CheckVersionFieldDeclarations(newProjectIgnoreMatcher())

	require := func(substr string) {
		t.Helper()
		for _, violation := range violations {
			if strings.Contains(violation, substr) {
				return
			}
		}
		t.Fatalf("expected a violation containing %q, got %#v", substr, violations)
	}
	if len(violations) != 5 {
		t.Fatalf("expected 5 violations, got %#v", violations)
	}
	require("struct 'Embedded' embeds model.Version")
	require("field 'Bare.Version' (model.Version) is missing gorm not null, gorm default:1, json omitempty")
	require("field 'Partial.Version' (model.Version) is missing gorm default:1, json omitempty")
	require("field 'Aliased.Revision' (model.Version) is missing gorm not null, gorm default:1, json omitempty")
	require("field 'Hidden.Version' (model.Version) carries json:\"-\"")
}

func TestCheckVersionFieldDeclarationsActionTypes(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() { modelDir = oldModelDir })

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)
	modelDir = "model"

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

	violations := CheckVersionFieldDeclarations(newProjectIgnoreMatcher())

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
