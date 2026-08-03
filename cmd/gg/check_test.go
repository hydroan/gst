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
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "record.go"), `package record

import _ "tmpapp/service/iam/session"
`)

	violations := CheckArchitectureDependency(newProjectIgnoreMatcher())

	for _, violation := range violations {
		if strings.Contains(violation, filepath.Join("service", "iam", "account", "login.go")) {
			t.Fatalf("same service module import should be allowed, got violations: %#v", violations)
		}
	}
	if len(violations) != 1 || !strings.Contains(violations[0], filepath.Join("service", "record", "record.go")) {
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

	violations := CheckModelSingularNaming(newProjectIgnoreMatcher())

	for _, violation := range violations {
		if strings.Contains(violation, filepath.Join("model", "types")) {
			t.Fatalf("shared model types directory should be allowed, got violations: %#v", violations)
		}
	}
	if len(violations) != 1 || !strings.Contains(violations[0], filepath.Join("model", "records")) {
		t.Fatalf("expected only ordinary plural model directory violation, got %#v", violations)
	}
}

func TestCheckModelSingularNamingSkipsGitIgnoredPaths(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() {
		modelDir = oldModelDir
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"

	// A runtime artifact directory ignored by Git rules, such as the log
	// directory a test run leaves behind, must not fail naming checks.
	writeCheckFile(t, filepath.Join(projectDir, ".gitignore"), "logs\n")
	if err := os.MkdirAll(filepath.Join(projectDir, "model", "user", "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "model", "records"), 0o755); err != nil {
		t.Fatal(err)
	}

	violations := CheckModelSingularNaming(newProjectIgnoreMatcher())

	for _, violation := range violations {
		if strings.Contains(violation, filepath.Join("model", "user", "logs")) {
			t.Fatalf("git-ignored directory should be skipped, got violations: %#v", violations)
		}
	}
	if len(violations) != 1 || !strings.Contains(violations[0], filepath.Join("model", "records")) {
		t.Fatalf("expected only non-ignored plural directory violation, got %#v", violations)
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
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample_record", "sample_record.go"), "package samplerecord\n")

	// A black-box test file using the `<package>_test` package name is allowed.
	writeCheckFile(t, filepath.Join(projectDir, "model", "group", "group.go"), "package group\n")
	writeCheckFile(t, filepath.Join(projectDir, "model", "group", "sample_record_test.go"), "package group_test\n")

	// A genuine mismatch between package name and directory name (after stripping underscores) should still be reported.
	writeCheckFile(t, filepath.Join(projectDir, "model", "mismatch", "mismatch.go"), "package wrongname\n")

	violations := CheckModelPackageNaming(newProjectIgnoreMatcher())

	for _, violation := range violations {
		if strings.Contains(violation, filepath.Join("sample_record", "sample_record.go")) {
			t.Fatalf("underscore-stripped package name should be allowed, got violations: %#v", violations)
		}
		if strings.Contains(violation, filepath.Join("group", "sample_record_test.go")) {
			t.Fatalf("external test package name should be allowed, got violations: %#v", violations)
		}
	}
	if len(violations) != 1 || !strings.Contains(violations[0], filepath.Join("mismatch", "mismatch.go")) {
		t.Fatalf("expected only genuine package name mismatch violation, got %#v", violations)
	}
}

func TestCheckModelPackageNamingSkipsGitIgnoredPaths(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() {
		modelDir = oldModelDir
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"

	writeCheckFile(t, filepath.Join(projectDir, ".gitignore"), "generated\n")

	// A mismatched package inside a git-ignored directory must not be reported.
	writeCheckFile(t, filepath.Join(projectDir, "model", "user", "generated", "helper.go"), "package mismatched\n")

	// A genuine mismatch outside ignored paths should still be reported.
	writeCheckFile(t, filepath.Join(projectDir, "model", "mismatch", "mismatch.go"), "package wrongname\n")

	violations := CheckModelPackageNaming(newProjectIgnoreMatcher())

	for _, violation := range violations {
		if strings.Contains(violation, "helper.go") {
			t.Fatalf("git-ignored path should be skipped, got violations: %#v", violations)
		}
	}
	if len(violations) != 1 || !strings.Contains(violations[0], filepath.Join("mismatch", "mismatch.go")) {
		t.Fatalf("expected only genuine package name mismatch violation, got %#v", violations)
	}
}

func TestCheckDSLDesignRejectsExactOnBuiltinIDActions(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() {
		modelDir = oldModelDir
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"

	writeCheckFile(t, filepath.Join(projectDir, "model", "iam", "session.go"), `package iam

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Session struct {
	model.Base
}

func (Session) Design() {
	Delete(func() {
		Service()
		Exact()
	})
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "iam", "current.go"), `package iam

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Current struct {
	model.Base
}

func (Current) Design() {
	Route("iam/sessions/current", func() {
		Get(func() {
			Service()
			Exact()
			Result[*CurrentGetRsp]()
		})
	})
}
`)

	violations := CheckDSLDesign(newProjectIgnoreMatcher())

	if len(violations) != 1 {
		t.Fatalf("expected exactly one violation, got %#v", violations)
	}
	if !strings.Contains(violations[0], "uses dsl.Exact() but relies on the built-in controller") {
		t.Fatalf("unexpected violation message: %q", violations[0])
	}
	if !strings.Contains(violations[0], filepath.Join("model", "iam", "session.go")) {
		t.Fatalf("violation should point to the offending file, got %q", violations[0])
	}
}

func TestCheckJSONTagNamingFlagsDSLActionTypeTags(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() {
		modelDir = oldModelDir
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"

	// Explicit DSL Payload and Result types carry the wire format of custom
	// actions, so their json tags must be snake_case just like model structs.
	// The result type lives in another file of the same package to cover
	// cross-file references.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), `package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Empty
}

func (Sample) Design() {
	Create(func() {
		Service()
		Payload[*SampleCreateReq]()
		Result[*SampleCreateRsp]()
	})
}

type SampleCreateReq struct {
	UserName string `+"`json:\"userName\"`"+`
	Note     string `+"`json:\"note\"`"+`
	Secret   string `+"`json:\"-\"`"+`
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "create_rsp.go"), `package sample

type SampleCreateRsp struct {
	CreatedAt string `+"`json:\"createdAt\"`"+`
	PlainName string `+"`json:\"plain_name\"`"+`
}
`)

	violations := CheckJSONTagNaming(newProjectIgnoreMatcher())

	if len(violations) != 2 {
		t.Fatalf("expected two action type json tag violations, got %#v", violations)
	}
	joined := strings.Join(violations, "\n")
	if !strings.Contains(joined, filepath.Join("model", "sample", "sample.go")+": field 'UserName' json tag 'userName' should be 'user_name'") {
		t.Fatalf("expected payload type violation with file path, got %#v", violations)
	}
	if !strings.Contains(joined, filepath.Join("model", "sample", "create_rsp.go")+": field 'CreatedAt' json tag 'createdAt' should be 'created_at'") {
		t.Fatalf("expected cross-file result type violation with file path, got %#v", violations)
	}
}

func TestCheckJSONTagNamingSkipsUnreferencedActionLikeStructs(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() {
		modelDir = oldModelDir
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"

	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), `package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Empty
}

func (Sample) Design() {
	Create(func() {
		Service()
		Payload[*SampleCreateReq]()
	})
}

type SampleCreateReq struct {
	UserName string `+"`json:\"userName\"`"+`
}
`)
	// Req/Rsp-suffixed structs not referenced by any Design, such as outbound
	// DTOs mirroring an external contract, must keep their tags unchecked.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "push.go"), `package sample

type PushReq struct {
	DeviceID string `+"`json:\"deviceId\"`"+`
}

type PushRsp struct {
	PushedAt string `+"`json:\"pushedAt\"`"+`
}
`)

	violations := CheckJSONTagNaming(newProjectIgnoreMatcher())

	for _, violation := range violations {
		if strings.Contains(violation, "push.go") {
			t.Fatalf("unreferenced action-like struct should be skipped, got violations: %#v", violations)
		}
	}
	if len(violations) != 1 || !strings.Contains(violations[0], "json tag 'userName' should be 'user_name'") {
		t.Fatalf("expected only referenced payload type violation, got %#v", violations)
	}
}

func TestCheckJSONTagNamingFlagsModelStructTags(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() {
		modelDir = oldModelDir
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"

	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.go"), `package record

import "github.com/hydroan/gst/model"

type Record struct {
	model.Base

	DisplayName string `+"`json:\"displayName\"`"+`
}
`)

	violations := CheckJSONTagNaming(newProjectIgnoreMatcher())

	if len(violations) != 1 {
		t.Fatalf("expected one model struct json tag violation, got %#v", violations)
	}
	if !strings.Contains(violations[0], filepath.Join("model", "record", "record.go")+": field 'DisplayName' json tag 'displayName' should be 'display_name'") {
		t.Fatalf("expected model struct violation with file path, got %#v", violations)
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
