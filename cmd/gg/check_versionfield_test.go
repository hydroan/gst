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
	Version model.Version `+"`json:\"version\" gorm:\"not null;default:1\"`"+`

	model.Base
}

func (Compliant) TableName() string { return "compliant_rows" }
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
	if len(violations) != 4 {
		t.Fatalf("expected 4 violations, got %#v", violations)
	}
	require("struct 'Embedded' embeds model.Version")
	require("field 'Bare.Version' (model.Version) is missing gorm setting(s) not null, default:1")
	require("field 'Partial.Version' (model.Version) is missing gorm setting(s) default:1")
	require("field 'Aliased.Revision' (model.Version) is missing gorm setting(s) not null, default:1")
}
