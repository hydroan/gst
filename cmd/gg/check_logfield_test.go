package main

import (
	"path/filepath"
	"testing"
)

func TestCheckLogFieldBoundednessFlagsMarshalerMethodsAndNamespaceCalls(t *testing.T) {
	oldModelDir, oldServiceDir := modelDir, serviceDir
	t.Cleanup(func() {
		modelDir, serviceDir = oldModelDir, oldServiceDir
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"
	serviceDir = "service"

	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), `package sample

import "go.uber.org/zap/zapcore"

type Sample struct {
	Name string
}

func (s Sample) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("name", s.Name)
	return nil
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "record.go"), `package record

import "go.uber.org/zap/zapcore"

type recordList []string

func (l *recordList) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	return nil
}
`)
	// Test files are checked too: a marshaler declared in a test helper still
	// bypasses the encoder for the entries that test code logs.
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "record_test.go"), `package record

import "go.uber.org/zap/zapcore"

type stubItem struct{}

func (stubItem) MarshalLogObject(enc zapcore.ObjectEncoder) error { return nil }
`)
	// The walk covers every project directory, not only model and service.
	writeCheckFile(t, filepath.Join(projectDir, "helper", "log.go"), `package helper

import "go.uber.org/zap"

func Fields() []zap.Field {
	return []zap.Field{zap.Namespace("request"), zap.String("id", "1")}
}
`)

	violations := CheckLogFieldBoundedness(newProjectIgnoreMatcher())

	if len(violations) != 4 {
		t.Fatalf("expected four violations, got %#v", violations)
	}
	assertViolationContains(t, violations, filepath.Join("model", "sample", "sample.go"), "type 'Sample' must not declare MarshalLogObject")
	assertViolationContains(t, violations, filepath.Join("service", "record", "record.go"), "type 'recordList' must not declare MarshalLogArray")
	assertViolationContains(t, violations, filepath.Join("service", "record", "record_test.go"), "type 'stubItem' must not declare MarshalLogObject")
	assertViolationContains(t, violations, filepath.Join("helper", "log.go"), "zap.Namespace must not be called")
}

func TestCheckLogFieldBoundednessResolvesZapImportForms(t *testing.T) {
	oldModelDir, oldServiceDir := modelDir, serviceDir
	t.Cleanup(func() {
		modelDir, serviceDir = oldModelDir, oldServiceDir
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"
	serviceDir = "service"

	writeCheckFile(t, filepath.Join(projectDir, "helper", "alias.go"), `package helper

import z "go.uber.org/zap"

func AliasFields() []z.Field {
	return []z.Field{z.Namespace("request")}
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "helper", "dot.go"), `package helper

import . "go.uber.org/zap"

func DotFields() []Field {
	return []Field{Namespace("request")}
}
`)
	// A Namespace selector on a non-zap package and a bare Namespace call
	// without a dot import both stay allowed: the check resolves the file's
	// actual zap import names instead of matching the bare identifier.
	writeCheckFile(t, filepath.Join(projectDir, "helper", "clean.go"), `package helper

import metrics "example.com/metrics"

func Clean() {
	metrics.Namespace("scope")
	scope("request")
}

func scope(string) {}
`)

	violations := CheckLogFieldBoundedness(newProjectIgnoreMatcher())

	if len(violations) != 2 {
		t.Fatalf("expected two violations, got %#v", violations)
	}
	assertViolationContains(t, violations, filepath.Join("helper", "alias.go"), "zap.Namespace must not be called")
	assertViolationContains(t, violations, filepath.Join("helper", "dot.go"), "zap.Namespace must not be called")
}

func TestCheckLogFieldBoundednessSkipsCopiedModulesAndGeneratedFiles(t *testing.T) {
	oldModelDir, oldServiceDir := modelDir, serviceDir
	t.Cleanup(func() {
		modelDir, serviceDir = oldModelDir, oldServiceDir
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"
	serviceDir = "service"

	writeFrameworkModuleFixture(t, projectDir, "sample")

	// Copied module subtrees keep whatever the framework repository ships.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "entity.go"), `package sample

import "go.uber.org/zap/zapcore"

type Entity struct{}

func (Entity) MarshalLogObject(enc zapcore.ObjectEncoder) error { return nil }
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "sample", "create.go"), `package sample

import "go.uber.org/zap/zapcore"

type items []string

func (items) MarshalLogArray(enc zapcore.ArrayEncoder) error { return nil }
`)
	// Generated files are owned by gg and skipped like in every other check.
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.gen.go"), `package record

import "go.uber.org/zap/zapcore"

type generated struct{}

func (generated) MarshalLogObject(enc zapcore.ObjectEncoder) error { return nil }
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.go"), `package record

type Record struct {
	Name string
}
`)

	violations := CheckLogFieldBoundedness(newProjectIgnoreMatcher())

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}
