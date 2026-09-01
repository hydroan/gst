package main

import (
	"path/filepath"
	"testing"
)

func TestCheckModelTableNameDeclarationFlagsMissingAndNonLiteral(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() { modelDir = oldModelDir })

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)
	modelDir = "model"

	// A model without any TableName declaration.
	writeCheckFile(t, filepath.Join(projectDir, "model", "group", "group.go"), `package group

import "github.com/hydroan/gst/model"

type Group struct {
	Name string `+"`json:\"name\"`"+`

	model.Base
}
`)
	// A TableName computed from runtime state instead of a literal.
	writeCheckFile(t, filepath.Join(projectDir, "model", "group", "dynamic.go"), `package group

import "github.com/hydroan/gst/model"

var prefix = "dyn"

type Dynamic struct {
	model.Base
}

func (Dynamic) TableName() string { return prefix + "_records" }
`)
	// An explicitly empty literal is as broken as no declaration.
	writeCheckFile(t, filepath.Join(projectDir, "model", "group", "unnamed.go"), `package group

import "github.com/hydroan/gst/model"

type Unnamed struct {
	model.Base
}

func (Unnamed) TableName() string { return "" }
`)
	// An AutoBase model reports with its own embedded base name.
	writeCheckFile(t, filepath.Join(projectDir, "model", "audit", "entry.go"), `package audit

import "github.com/hydroan/gst/model"

type Entry struct {
	model.AutoBase
}
`)
	// The method may live in another file of the same package.
	writeCheckFile(t, filepath.Join(projectDir, "model", "player", "player.go"), `package player

import "github.com/hydroan/gst/model"

type Player struct {
	model.Base
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "player", "table.go"), `package player

func (Player) TableName() string { return "players" }
`)
	// Virtual models embed model.Empty and have no table to declare.
	writeCheckFile(t, filepath.Join(projectDir, "model", "auth", "login.go"), `package auth

import "github.com/hydroan/gst/model"

type Login struct {
	model.Empty
}
`)

	violations := CheckModelTableNameDeclaration(newProjectIgnoreMatcher())

	if len(violations) != 4 {
		t.Fatalf("expected four violations, got %#v", violations)
	}
	assertViolationContains(t, violations, filepath.Join("model", "group", "group.go"), "model 'Group' embeds model.Base but declares no TableName() string")
	assertViolationContains(t, violations, filepath.Join("model", "group", "dynamic.go"), "TableName of model 'Dynamic' must be a single return of a non-empty string literal")
	assertViolationContains(t, violations, filepath.Join("model", "group", "unnamed.go"), "TableName of model 'Unnamed' must be a single return of a non-empty string literal")
	assertViolationContains(t, violations, filepath.Join("model", "audit", "entry.go"), "model 'Entry' embeds model.AutoBase but declares no TableName() string")
}

func TestCheckModelTableNameDeclarationSkipsCopiedModulesGeneratedAndTests(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() { modelDir = oldModelDir })

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)
	modelDir = "model"

	writeFrameworkModuleFixture(t, projectDir, "sample")

	// Copied module subtrees keep whatever the framework repository ships.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "entity.go"), `package sample

import "github.com/hydroan/gst/model"

type Entity struct {
	model.Base
}
`)
	// Generated files are owned by gg and skipped like in every other check.
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.gen.go"), `package record

import "github.com/hydroan/gst/model"

type Generated struct {
	model.Base
}
`)
	// Test fixtures are not business models.
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record_test.go"), `package record

import "github.com/hydroan/gst/model"

type fixture struct {
	model.Base
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.go"), `package record

import "github.com/hydroan/gst/model"

type Record struct {
	model.Base
}

func (Record) TableName() string { return "records" }
`)

	violations := CheckModelTableNameDeclaration(newProjectIgnoreMatcher())

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}
