package main

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

func TestFillVersionFieldTags(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeProjectGoMod(t, projectDir)

	path := filepath.Join(projectDir, "model", "document", "document.go")
	writeProjectFile(t, path, `package document

import "github.com/hydroan/gst/model"

// Bare has no tag yet.
type Bare struct {
	Version model.Version // trailing comment

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
	// gains the missing settings. All three converge on the same full shape,
	// and the comments and layout of the file stay as they were.
	want := `package document

import "github.com/hydroan/gst/model"

// Bare has no tag yet.
type Bare struct {
	Version model.Version ` + "`json:\"version,omitempty\" gorm:\"not null;default:1\"`" + ` // trailing comment

	model.Base
}

func (Bare) TableName() string { return "bares" }

type Tagged struct {
	Version model.Version ` + "`json:\"version,omitempty\" gorm:\"not null;default:1\"`" + `

	model.Base
}

func (Tagged) TableName() string { return "taggeds" }

type Partial struct {
	Version model.Version ` + "`json:\"version,omitempty\" gorm:\"not null;default:1\"`" + `

	model.Base
}

func (Partial) TableName() string { return "partials" }
`
	if source != want {
		t.Fatalf("healed file =\n%s\nwant\n%s", source, want)
	}

	// The healed file passes the check and a second run changes nothing.
	if violations := ggcheck.Run([]ggcheck.Check{ggcheck.VersionFieldDeclaration})[0].Violations; len(violations) != 0 {
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
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeProjectGoMod(t, projectDir)

	writeProjectFile(t, filepath.Join(projectDir, "model", "document", "document.go"), `package document

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
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeProjectGoMod(t, projectDir)

	// json:"-" hides the version clients must hand back; un-hiding it is a
	// semantic decision, so gen aborts instead of healing.
	writeProjectFile(t, filepath.Join(projectDir, "model", "document", "document.go"), `package document

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

// TestHealedVersionTagRejectsAnInsertionOutsideTheTag pins the error path
// of healedVersionTag: a finding describes the file it was scanned from, so
// applied to a file whose tag sits elsewhere, its insertions fall outside
// the tag literal and the rewrite refuses rather than splicing blindly.
func TestHealedVersionTagRejectsAnInsertionOutsideTheTag(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeProjectGoMod(t, projectDir)
	source := `package document

import "github.com/hydroan/gst/model"

type Tagged struct {
	Version model.Version ` + "`json:\"version\"`" + `

	model.Base
}

func (Tagged) TableName() string { return "taggeds" }
`
	path := filepath.Join(projectDir, "model", "document", "document.go")
	writeProjectFile(t, path, source)
	findings, err := ggcheck.VersionFieldFindings()
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want the one tagged field", len(findings))
	}

	// The same file with a line added above the struct: every offset of the
	// finding now points before the tag literal.
	shifted := strings.Replace(source, "type Tagged struct", "// Tagged has moved down a line.\ntype Tagged struct", 1)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, shifted, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	field := versionField(file, findings[0])
	if field == nil {
		t.Fatal("field Tagged.Version not found")
	}

	_, err = healedVersionTag(fset, field.Tag, findings[0])
	if err == nil {
		t.Fatal("healedVersionTag() should refuse an insertion outside the tag literal")
	}
}
