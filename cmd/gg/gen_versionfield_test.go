package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFillVersionFieldTags(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() { modelDir = oldModelDir })

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)
	modelDir = "model"

	path := filepath.Join(projectDir, "model", "config", "config.go")
	writeCheckFile(t, path, `package config

import "github.com/hydroan/gst/model"

type Bare struct {
	Version model.Version

	model.Base
}

func (Bare) TableName() string { return "bares" }

type Tagged struct {
	Version model.Version `+"`json:\"version\"`"+`

	model.Base
}

func (Tagged) TableName() string { return "taggeds" }

type Partial struct {
	Version model.Version `+"`json:\"version\" gorm:\"not null\"`"+`

	model.Base
}

func (Partial) TableName() string { return "partials" }
`)

	if err := fillVersionFieldTags(true); err != nil {
		t.Fatal(err)
	}

	healed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := string(healed)

	// A bare field gains the whole tag (json included); a json section
	// without omitempty gains it in place; a missing or partial gorm section
	// gains the missing settings. All three converge on the same full shape.
	if got := strings.Count(source, "`json:\"version,omitempty\" gorm:\"not null;default:1\"`"); got != 3 {
		t.Fatalf("all three fields should heal to the full tag, found %d of them in:\n%s", got, source)
	}

	// The healed file passes the check and a second run changes nothing.
	if violations := CheckVersionFieldDeclarations(newProjectIgnoreMatcher()); len(violations) != 0 {
		t.Fatalf("healed file should pass the check, got %#v", violations)
	}
	if err := fillVersionFieldTags(true); err != nil {
		t.Fatal(err)
	}
	after, rereadErr := os.ReadFile(path)
	if rereadErr != nil {
		t.Fatal(rereadErr)
	}
	if string(after) != source {
		t.Fatal("a second fill run must be a no-op")
	}
}

func TestFillVersionFieldTagsRejectsEmbedded(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() { modelDir = oldModelDir })

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)
	modelDir = "model"

	writeCheckFile(t, filepath.Join(projectDir, "model", "config", "config.go"), `package config

import "github.com/hydroan/gst/model"

type Embedded struct {
	model.Version

	model.Base
}

func (Embedded) TableName() string { return "embeddeds" }
`)

	err := fillVersionFieldTags(true)
	if err == nil || !strings.Contains(err.Error(), "embeds model.Version") {
		t.Fatalf("embedded declaration must abort generation, got %v", err)
	}
}

func TestFillVersionFieldTagsRejectsHiddenJSON(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() { modelDir = oldModelDir })

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)
	modelDir = "model"

	// json:"-" hides the version clients must hand back; un-hiding it is a
	// semantic decision, so gen aborts instead of healing.
	writeCheckFile(t, filepath.Join(projectDir, "model", "config", "config.go"), `package config

import "github.com/hydroan/gst/model"

type Hidden struct {
	Version model.Version `+"`json:\"-\" gorm:\"not null;default:1\"`"+`

	model.Base
}

func (Hidden) TableName() string { return "hiddens" }
`)

	err := fillVersionFieldTags(true)
	if err == nil || !strings.Contains(err.Error(), "json:\"-\"") {
		t.Fatalf("a hidden json field must abort generation, got %v", err)
	}
}
