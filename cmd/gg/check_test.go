package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckArchitectureDependencyAllowsSameServiceModuleImports(t *testing.T) {
	oldModelDir := modelDir
	oldServiceDir := serviceDir
	oldDaoDir := daoDir
	t.Cleanup(func() {
		modelDir = oldModelDir
		serviceDir = oldServiceDir
		daoDir = oldDaoDir
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"
	serviceDir = "service"
	daoDir = "dao"

	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n")
	writeCheckFile(t, filepath.Join(projectDir, "service", "iam", "account", "login.go"), `package account

import _ "tmpapp/service/iam/session"
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "order", "order.go"), `package order

import _ "tmpapp/service/iam/session"
`)

	violations := CheckArchitectureDependency()

	for _, violation := range violations {
		if strings.Contains(violation, filepath.Join("service", "iam", "account", "login.go")) {
			t.Fatalf("same service module import should be allowed, got violations: %#v", violations)
		}
	}
	if len(violations) != 1 || !strings.Contains(violations[0], filepath.Join("service", "order", "order.go")) {
		t.Fatalf("expected only cross service module import violation, got %#v", violations)
	}
}

func TestCheckModelSingularNamingAllowsSharedTypesDirectory(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() {
		modelDir = oldModelDir
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"

	if err := os.MkdirAll(filepath.Join(projectDir, "model", "types"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "model", "records"), 0o755); err != nil {
		t.Fatal(err)
	}

	violations := CheckModelSingularNaming()

	for _, violation := range violations {
		if strings.Contains(violation, filepath.Join("model", "types")) {
			t.Fatalf("shared model types directory should be allowed, got violations: %#v", violations)
		}
	}
	if len(violations) != 1 || !strings.Contains(violations[0], filepath.Join("model", "records")) {
		t.Fatalf("expected only ordinary plural model directory violation, got %#v", violations)
	}
}

func TestCheckModelPackageNamingAllowsUnderscoreStrippedAndExternalTestPackages(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() {
		modelDir = oldModelDir
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"

	// A package name with underscores stripped from the directory name is allowed.
	writeCheckFile(t, filepath.Join(projectDir, "model", "receive_robot", "receive_robot.go"), "package receiverobot\n")

	// A black-box test file using the `<package>_test` package name is allowed.
	writeCheckFile(t, filepath.Join(projectDir, "model", "group", "group.go"), "package group\n")
	writeCheckFile(t, filepath.Join(projectDir, "model", "group", "receive_robot_test.go"), "package group_test\n")

	// A genuine mismatch between package name and directory name (after stripping underscores) should still be reported.
	writeCheckFile(t, filepath.Join(projectDir, "model", "mismatch", "mismatch.go"), "package wrongname\n")

	violations := CheckModelPackageNaming()

	for _, violation := range violations {
		if strings.Contains(violation, filepath.Join("receive_robot", "receive_robot.go")) {
			t.Fatalf("underscore-stripped package name should be allowed, got violations: %#v", violations)
		}
		if strings.Contains(violation, filepath.Join("group", "receive_robot_test.go")) {
			t.Fatalf("external test package name should be allowed, got violations: %#v", violations)
		}
	}
	if len(violations) != 1 || !strings.Contains(violations[0], filepath.Join("mismatch", "mismatch.go")) {
		t.Fatalf("expected only genuine package name mismatch violation, got %#v", violations)
	}
}

func writeCheckFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
