package ggcheck_test

import (
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

// TestValidateTagBanFlagsValidateKeys pins that a field carrying a validate
// tag is reported under its file and line with the rules it declares there, a
// model's field and a request type's alike, and whether or not it carries a
// binding tag beside it: the framework validates by binding alone. A field
// declaring its rules under binding, and one merely mentioning the word in
// another tag, pass.
func TestValidateTagBanFlagsValidateKeys(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.go"), `package record

import "github.com/hydroan/gst/model"

type Record struct {
	Name string `+"`json:\"name\" validate:\"required,max=64\"`"+`

	// Rules under binding are the ones the framework runs.
	Remark string `+"`json:\"remark\" binding:\"max=255\"`"+`

	// A value merely containing the word is no validate tag.
	Note string `+"`json:\"note\" gorm:\"comment:validate later\"`"+`

	model.Base
}

func (Record) TableName() string { return "records" }
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "create.go"), `package record

type RecordCreateReq struct {
	Title string `+"`json:\"title\" validate:\"required\"`"+`
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "item.go"), `package record

type ItemCreateReq struct {
	Code string `+"`json:\"code\" binding:\"required\" validate:\"len=6\"`"+`
}
`)

	violations := runCheck(ggcheck.ValidateTagBan)

	if len(violations) != 3 {
		t.Fatalf("expected three violations, got %#v", violations)
	}
	assertViolationContains(t, violations, filepath.Join("model", "record", "record.go"), ":6: field 'Record.Name' declares its validation rules under the validate tag (required,max=64), which the framework never runs; declare them under the binding tag instead")
	assertViolationContains(t, violations, filepath.Join("model", "record", "create.go"), ":4: field 'RecordCreateReq.Title' declares its validation rules under the validate tag (required), which the framework never runs; declare them under the binding tag instead")
	assertViolationContains(t, violations, filepath.Join("model", "record", "item.go"), ":4: field 'ItemCreateReq.Code' declares its validation rules under the validate tag (len=6), which the framework never runs; declare them under the binding tag instead")
}

// TestValidateTagBanSkipsCopiedModulesGeneratedAndTests pins what the ban
// leaves alone, the way the other checks of struct tags do: the subtrees
// copied framework modules own, generated files and test files.
func TestValidateTagBanSkipsCopiedModulesGeneratedAndTests(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)

	writeFrameworkModuleFixture(t, projectDir, "sample")

	// Copied module subtrees keep whatever the framework repository ships.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "entity.go"), `package sample

type EntityCreateReq struct {
	Code string `+"`json:\"code\" validate:\"required\"`"+`
}
`)
	// Generated files are owned by gg and skipped like in every other check.
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.gen.go"), `package record

type generated struct {
	Code string `+"`validate:\"required\"`"+`
}
`)
	// Test fixtures may drive a validator of their own.
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record_test.go"), `package record

type fixture struct {
	Code string `+"`validate:\"required\"`"+`
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.go"), `package record

type RecordCreateReq struct {
	Code string `+"`json:\"code\" binding:\"required,len=6\"`"+`
}
`)

	violations := runCheck(ggcheck.ValidateTagBan)

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}
