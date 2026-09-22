package ggcheck_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

func TestModelSingularNamingAllowsExemptPlurals(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	// types, data and stats are plural in form but name one body of content,
	// so model directories and files may keep them; records, statistics and
	// metrics are ordinary plurals.
	for _, dir := range []string{"types", "data", "stats", "records", "statistics", "metrics"} {
		if err := os.MkdirAll(filepath.Join(projectDir, "model", dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeCheckFile(t, filepath.Join(projectDir, "model", "stats", "stats.go"), "package stats\n")

	violations := runCheck(ggcheck.ModelSingularNaming)

	if len(violations) != 3 {
		t.Fatalf("expected the ordinary plural model directory violations only, got %#v", violations)
	}
	for _, dir := range []string{"records", "statistics", "metrics"} {
		assertViolationContains(t, violations, filepath.Join("model", dir), "should be singular")
	}
}

func TestModelFileNameHyphens(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record-item.go"), "package record\n")
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record_note.go"), "package record\n")

	violations := runCheck(ggcheck.ModelFileNameHyphens)

	if len(violations) != 1 {
		t.Fatalf("expected one hyphenated model file violation, got %#v", violations)
	}
	assertViolationContains(t, violations, filepath.Join("model", "record", "record-item.go"), "should not contain hyphens (suggested: record_item.go)")
}

func TestModelSingularNamingSkipsGitIgnoredPaths(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	// A runtime artifact directory ignored by Git rules, such as the log
	// directory a test run leaves behind, must not fail naming checks.
	writeCheckFile(t, filepath.Join(projectDir, ".gitignore"), "logs\n")
	if err := os.MkdirAll(filepath.Join(projectDir, "model", "user", "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "model", "records"), 0o755); err != nil {
		t.Fatal(err)
	}

	violations := runCheck(ggcheck.ModelSingularNaming)

	for _, violation := range violations {
		if strings.Contains(violation, filepath.Join("model", "user", "logs")) {
			t.Fatalf("git-ignored directory should be skipped, got violations: %#v", violations)
		}
	}
	if len(violations) != 1 || !strings.Contains(violations[0], filepath.Join("model", "records")) {
		t.Fatalf("expected only non-ignored plural directory violation, got %#v", violations)
	}
}

func TestModelPackageNamingAllowsUnderscoreStrippedAndExternalTestPackages(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	// A package name with underscores stripped from the directory name is allowed.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample_record", "sample_record.go"), "package samplerecord\n")

	// A black-box test file using the `<package>_test` package name is allowed.
	writeCheckFile(t, filepath.Join(projectDir, "model", "group", "group.go"), "package group\n")
	writeCheckFile(t, filepath.Join(projectDir, "model", "group", "sample_record_test.go"), "package group_test\n")

	// A genuine mismatch between package name and directory name (after stripping underscores) should still be reported.
	writeCheckFile(t, filepath.Join(projectDir, "model", "mismatch", "mismatch.go"), "package wrongname\n")

	violations := runCheck(ggcheck.ModelPackageNaming)

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

func TestModelPackageNamingSkipsGitIgnoredPaths(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, ".gitignore"), "generated\n")

	// A mismatched package inside a git-ignored directory must not be reported.
	writeCheckFile(t, filepath.Join(projectDir, "model", "user", "generated", "helper.go"), "package mismatched\n")

	// A genuine mismatch outside ignored paths should still be reported.
	writeCheckFile(t, filepath.Join(projectDir, "model", "mismatch", "mismatch.go"), "package wrongname\n")

	violations := runCheck(ggcheck.ModelPackageNaming)

	for _, violation := range violations {
		if strings.Contains(violation, "helper.go") {
			t.Fatalf("git-ignored path should be skipped, got violations: %#v", violations)
		}
	}
	if len(violations) != 1 || !strings.Contains(violations[0], filepath.Join("mismatch", "mismatch.go")) {
		t.Fatalf("expected only genuine package name mismatch violation, got %#v", violations)
	}
}

// TestDSLDesignRulesRejectsBaseTypesEmbeddedThroughAPointer pins where gg
// check reports a base type embedded through a pointer: under the DSL design
// rules, which gate gg gen as well.
func TestDSLDesignRulesRejectsBaseTypesEmbeddedThroughAPointer(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.go"), `package record

import "github.com/hydroan/gst/model"

type Record struct {
	Name string

	*model.Base
}

func (Record) TableName() string { return "records" }
`)

	violations := runCheck(ggcheck.DSLDesignRules)

	if len(violations) != 1 || !strings.Contains(violations[0], "struct Record embeds *model.Base; embed model.Base by value") {
		t.Fatalf("expected the pointer-embedded base to be reported once, got %#v", violations)
	}
}

func TestDSLDesignRulesRejectsExactOnBuiltinIDActions(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

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

	violations := runCheck(ggcheck.DSLDesignRules)

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

func TestJSONTagNamingFlagsDSLActionTypeTags(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

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

	violations := runCheck(ggcheck.JSONTagNaming)

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

func TestJSONTagNamingSkipsUnreferencedActionLikeStructs(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

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

	violations := runCheck(ggcheck.JSONTagNaming)

	for _, violation := range violations {
		if strings.Contains(violation, "push.go") {
			t.Fatalf("unreferenced action-like struct should be skipped, got violations: %#v", violations)
		}
	}
	if len(violations) != 1 || !strings.Contains(violations[0], "json tag 'userName' should be 'user_name'") {
		t.Fatalf("expected only referenced payload type violation, got %#v", violations)
	}
}

func TestJSONTagNamingFlagsModelStructTags(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.go"), `package record

import "github.com/hydroan/gst/model"

type Record struct {
	model.Base

	DisplayName string `+"`json:\"displayName\"`"+`
}
`)

	violations := runCheck(ggcheck.JSONTagNaming)

	if len(violations) != 1 {
		t.Fatalf("expected one model struct json tag violation, got %#v", violations)
	}
	if !strings.Contains(violations[0], filepath.Join("model", "record", "record.go")+": field 'DisplayName' json tag 'displayName' should be 'display_name'") {
		t.Fatalf("expected model struct violation with file path, got %#v", violations)
	}
}

// TestDirectoryRestrictionsAcceptsConventionalProjectDirectories proves the
// directories a project conventionally keeps beside its packages — test
// suites, development scripts and Helm charts, next to the deployment
// manifests and operator scripts — pass the directory check, while a
// directory the project structure has no place for is still reported.
func TestServiceFileBoundariesCountsFrameworkServiceStructs(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n")

	// Two service structs in one file, the framework package imported under
	// an alias.
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "record.go"), `package record

import (
	svc "github.com/hydroan/gst/service"
	"tmpapp/model"
)

type Creator struct {
	svc.Base[*model.Record, *model.Record, *model.Record]
}

type Lister struct {
	svc.Base[*model.Record, *model.Record, *model.Record]
}
`)
	// Structs embedding the Base of the project's own service package are not
	// service structs of the framework.
	writeCheckFile(t, filepath.Join(projectDir, "service", "note", "note.go"), `package note

import (
	"tmpapp/model"
	"tmpapp/service"
)

type First struct {
	service.Base[*model.Note, *model.Note, *model.Note]
}

type Second struct {
	service.Base[*model.Note, *model.Note, *model.Note]
}
`)

	violations := runCheck(ggcheck.ServiceFileBoundaries)

	if len(violations) != 1 {
		t.Fatalf("expected one violation, got %#v", violations)
	}
	assertViolationContains(t, violations, filepath.Join("service", "record", "record.go"), "should contain at most one service struct (found: Creator, Lister)")
}

func TestDirectoryRestrictionsAcceptsConventionalProjectDirectories(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n\nrequire github.com/hydroan/gst v0.0.0\n")
	for _, dir := range []string{"deploy", "scripts", "test", "hack", "charts", "sample"} {
		if err := os.MkdirAll(filepath.Join(projectDir, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	violations := runCheck(ggcheck.DirectoryRestrictions)
	want := []string{"Directory 'sample' is not allowed in project structure"}
	if !slices.Equal(violations, want) {
		t.Fatalf("expected only the unplaced directory reported, got %#v", violations)
	}
}

// TestRunReturnsOneResultPerCheckInOrder pins that Run answers with one result
// per check, in the order the checks were given, each under its check's name.
func TestRunReturnsOneResultPerCheckInOrder(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record-item.go"), "package record\n")

	results := ggcheck.Run([]ggcheck.Check{ggcheck.ModelFileBoundaries, ggcheck.ModelFileNameHyphens})

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Name != "Model file boundaries" || len(results[0].Violations) != 0 {
		t.Fatalf("results[0] = %+v, want a clean Model file boundaries result", results[0])
	}
	if results[1].Name != "Model file name hyphens" || len(results[1].Violations) != 1 {
		t.Fatalf("results[1] = %+v, want one Model file name hyphens violation", results[1])
	}
}

// TestModelActionTypeNamingFlagsSuffixlessPayloadAndResultTypes pins the
// naming rule of explicit DSL action types: a Payload type ends with Req and a
// Result type with Rsp, while the model's own type is exempt.
func TestModelActionTypeNamingFlagsSuffixlessPayloadAndResultTypes(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	source := `package sample

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Base
}

type SampleCreateReq struct{}

type SampleCreateRsp struct{}

type SampleUpdateInput struct{}

type SampleUpdateOutput struct{}

func (Sample) Design() {
	dsl.Endpoint("samples")
	dsl.Create(func() {
		dsl.Payload[*SampleCreateReq]()
		dsl.Result[*SampleCreateRsp]()
	})
	dsl.Update(func() {
		dsl.Payload[*SampleUpdateInput]()
		dsl.Result[*SampleUpdateOutput]()
	})
	dsl.Get(func() {
		dsl.Result[*Sample]()
	})
}
`
	path := filepath.Join("model", "sample", "sample.go")
	writeCheckFile(t, filepath.Join(projectDir, path), source)

	violations := runCheck(ggcheck.ModelActionTypeNaming)

	want := []string{
		fmt.Sprintf("%s:%d: Payload type 'SampleUpdateInput' should end with Req", path, sourceLine(t, source, "dsl.Payload[*SampleUpdateInput]()")),
		fmt.Sprintf("%s:%d: Result type 'SampleUpdateOutput' should end with Rsp", path, sourceLine(t, source, "dsl.Result[*SampleUpdateOutput]()")),
	}
	if !slices.Equal(violations, want) {
		t.Fatalf("violations = %#v, want %#v", violations, want)
	}
}

// TestModelFileBoundariesFlagsFilesDeclaringSeveralModels pins the
// one-model-per-file rule: a model file declaring two model structs is
// flagged, while one declaring a model beside its request type is not.
func TestModelFileBoundariesFlagsFilesDeclaringSeveralModels(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), `package sample

import "github.com/hydroan/gst/model"

type Sample struct {
	model.Base
}

type SampleArchive struct {
	model.Base
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.go"), `package record

import "github.com/hydroan/gst/model"

type Record struct {
	model.Base
}

type RecordCreateReq struct {
	Name string `+"`json:\"name\"`"+`
}
`)

	violations := runCheck(ggcheck.ModelFileBoundaries)

	want := []string{"Model file '" + filepath.Join("model", "sample", "sample.go") + "' should contain at most one model struct (found: Sample, SampleArchive)"}
	if !slices.Equal(violations, want) {
		t.Fatalf("violations = %#v, want %#v", violations, want)
	}
}
