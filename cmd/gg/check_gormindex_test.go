package main

import (
	"path/filepath"
	"testing"
)

func TestCheckGormTagIndexBanFlagsIndexKeys(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() { modelDir = oldModelDir })

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"

	writeCheckFile(t, filepath.Join(projectDir, "model", "group", "group.go"), `package group

import "github.com/hydroan/gst/model"

type Group struct {
	Code string `+"`json:\"code\" gorm:\"type:varchar(64);index:idx_groups_code,priority:1\"`"+`

	// A value merely containing the word stays allowed: keys compare per
	// semicolon-separated segment.
	Remark string `+"`json:\"remark\" gorm:\"type:varchar(255);comment:index of records\"`"+`

	model.Base
}

func (Group) TableName() string { return "groups" }
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "group", "channel.go"), `package group

import "github.com/hydroan/gst/model"

type Channel struct {
	Name string `+"`json:\"name\" gorm:\"uniqueIndex\"`"+`

	model.Base
}

func (Channel) TableName() string { return "channels" }
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "group", "device.go"), `package group

import "github.com/hydroan/gst/model"

type Device struct {
	Serial string `+"`json:\"serial\" gorm:\"unique\"`"+`

	model.Base
}

func (Device) TableName() string { return "devices" }
`)

	violations := CheckGormTagIndexBan(newProjectIgnoreMatcher())

	if len(violations) != 3 {
		t.Fatalf("expected three violations, got %#v", violations)
	}
	assertViolationContains(t, violations, filepath.Join("model", "group", "group.go"), "field 'Group.Code' configures an index through the gorm tag (index)")
	assertViolationContains(t, violations, filepath.Join("model", "group", "channel.go"), "field 'Channel.Name' configures an index through the gorm tag (uniqueIndex)")
	assertViolationContains(t, violations, filepath.Join("model", "group", "device.go"), "field 'Device.Serial' configures an index through the gorm tag (unique)")
}

func TestCheckGormTagIndexBanSkipsCopiedModulesGeneratedAndTests(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() { modelDir = oldModelDir })

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"

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

	violations := CheckGormTagIndexBan(newProjectIgnoreMatcher())

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}
