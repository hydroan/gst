package ggcheck_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

func TestArchitectureDependenciesAllowsSameServiceModuleImports(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n")
	writeCheckFile(t, filepath.Join(projectDir, "service", "iam", "account", "login.go"), `package account

import _ "tmpapp/service/iam/session"
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "record.go"), `package record

import _ "tmpapp/service/iam/session"
`)

	violations := runCheck(ggcheck.ArchitectureDependencies)

	for _, violation := range violations {
		if strings.Contains(violation, filepath.Join("service", "iam", "account", "login.go")) {
			t.Fatalf("same service module import should be allowed, got violations: %#v", violations)
		}
	}
	if len(violations) != 1 || !strings.Contains(violations[0], filepath.Join("service", "record", "record.go")) {
		t.Fatalf("expected only cross service module import violation, got %#v", violations)
	}
}

// TestArchitectureDependenciesReadsAModulePathWithADeprecationComment pins
// that the check still recognizes the project's own imports when the module
// directive carries the deprecation comment go documents, instead of letting
// every violation pass unseen.
func TestArchitectureDependenciesReadsAModulePathWithADeprecationComment(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp // Deprecated: use tmpapp/v2 instead.\n\ngo 1.26\n")
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "record.go"), `package record

import _ "tmpapp/service/iam/session"
`)

	violations := runCheck(ggcheck.ArchitectureDependencies)

	if len(violations) != 1 || !strings.Contains(violations[0], filepath.Join("service", "record", "record.go")) {
		t.Fatalf("expected the cross service module import violation, got %#v", violations)
	}
}

// TestArchitectureDependenciesReportsAnUnreadableModulePath pins that a
// project whose module path cannot be read fails the check instead of passing
// it: without the module path the check recognizes no project import at all.
func TestArchitectureDependenciesReportsAnUnreadableModulePath(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "record.go"), `package record

import _ "tmpapp/service/iam/session"
`)

	violations := runCheck(ggcheck.ArchitectureDependencies)

	if len(violations) != 1 || !strings.Contains(violations[0], "reading the module path") {
		t.Fatalf("expected one violation naming the unreadable module path, got %#v", violations)
	}
}
