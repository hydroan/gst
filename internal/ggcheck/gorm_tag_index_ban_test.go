package ggcheck_test

import (
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

func TestGormTagIndexBanFlagsIndexKeys(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.go"), `package record

import "github.com/hydroan/gst/model"

type Record struct {
	Code string `+"`json:\"code\" gorm:\"type:varchar(64);index:idx_records_code,priority:1\"`"+`

	// A value merely containing the word stays allowed: keys compare per
	// semicolon-separated segment.
	Remark string `+"`json:\"remark\" gorm:\"type:varchar(255);comment:index of records\"`"+`

	model.Base
}

func (Record) TableName() string { return "records" }
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "item.go"), `package record

import "github.com/hydroan/gst/model"

type Item struct {
	Name string `+"`json:\"name\" gorm:\"uniqueIndex\"`"+`

	model.Base
}

func (Item) TableName() string { return "items" }
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "sample.go"), `package record

import "github.com/hydroan/gst/model"

type Sample struct {
	Serial string `+"`json:\"serial\" gorm:\"unique\"`"+`

	model.Base
}

func (Sample) TableName() string { return "samples" }
`)

	violations := runCheck(ggcheck.GormTagIndexBan)

	if len(violations) != 3 {
		t.Fatalf("expected three violations, got %#v", violations)
	}
	assertViolationContains(t, violations, filepath.Join("model", "record", "record.go"), "field 'Record.Code' configures an index through the gorm tag (index)")
	assertViolationContains(t, violations, filepath.Join("model", "record", "item.go"), "field 'Item.Name' configures an index through the gorm tag (uniqueIndex)")
	assertViolationContains(t, violations, filepath.Join("model", "record", "sample.go"), "field 'Sample.Serial' configures an index through the gorm tag (unique)")
}

// TestGormTagIndexBanReadsTagsAsGormDoes pins that the ban follows gorm's
// own reading of the tag: a key in any case configures the index all the
// same, while unique set to false, and a key kept inside a comment by a
// semicolon escaped as \;, configure none.
func TestGormTagIndexBanReadsTagsAsGormDoes(t *testing.T) {
	for _, tc := range []struct {
		name string
		tag  string
		key  string // the key the violation names; empty for no violation
	}{
		{name: "UpperCaseIndex", tag: `INDEX`, key: "index"},
		{name: "LowerCaseUniqueIndex", tag: `uniqueindex`, key: "uniqueIndex"},
		{name: "NamedUniqueIndexInMixedCase", tag: `UniqueIndex:uq_records_code`, key: "uniqueIndex"},
		{name: "CapitalizedUnique", tag: `Unique`, key: "unique"},
		{name: "UniqueSetToFalse", tag: `unique:false`},
		{name: "UniqueInsideAComment", tag: `comment:a\\;unique`},
		{name: "IndexInsideAComment", tag: `comment:b\\;index`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projectDir := t.TempDir()
			t.Chdir(projectDir)
			writeCheckProjectGoMod(t, projectDir)

			recordPath := filepath.Join("model", "record", "record.go")
			writeCheckFile(t, filepath.Join(projectDir, recordPath), `package record

import "github.com/hydroan/gst/model"

type Record struct {
	Code string `+"`json:\"code\" gorm:\""+tc.tag+"\"`"+`

	model.Base
}

func (Record) TableName() string { return "records" }
`)

			violations := runCheck(ggcheck.GormTagIndexBan)

			if tc.key == "" {
				if len(violations) != 0 {
					t.Fatalf("expected no violations, got %#v", violations)
				}
				return
			}
			if len(violations) != 1 {
				t.Fatalf("expected one violation, got %#v", violations)
			}
			assertViolationContains(t, violations, recordPath, "field 'Record.Code' configures an index through the gorm tag ("+tc.key+")")
		})
	}
}

func TestGormTagIndexBanSkipsCopiedModulesGeneratedAndTests(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)

	writeFrameworkModuleFixture(t, projectDir, "sample")

	// Copied module subtrees keep whatever the framework repository ships.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "entity.go"), `package sample

type Entity struct {
	Code string `+"`gorm:\"uniqueIndex\"`"+`
}
`)
	// Generated files are owned by gg and skipped like in every other check.
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.gen.go"), `package record

type generated struct {
	Code string `+"`gorm:\"index\"`"+`
}
`)
	// Test fixtures may exercise gorm's own tag machinery.
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record_test.go"), `package record

type fixture struct {
	Code string `+"`gorm:\"index\"`"+`
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.go"), `package record

type Record struct {
	Code string `+"`json:\"code\" gorm:\"type:varchar(64);not null\"`"+`
}
`)

	violations := runCheck(ggcheck.GormTagIndexBan)

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}
