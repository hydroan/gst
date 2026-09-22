package main

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

func TestCheckModelSingularNamingAllowsExemptPlurals(t *testing.T) {
	oldModelDir := modelDir
	t.Cleanup(func() {
		modelDir = oldModelDir
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"

	// types, data and stats are plural in form but name one body of content,
	// so model directories and files may keep them; records is an ordinary
	// plural.
	for _, dir := range []string{"types", "data", "stats", "records"} {
		if err := os.MkdirAll(filepath.Join(projectDir, "model", dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeCheckFile(t, filepath.Join(projectDir, "model", "stats", "stats.go"), "package stats\n")

	violations := CheckModelSingularNaming(newProjectIgnoreMatcher())

	if len(violations) != 1 || !strings.Contains(violations[0], filepath.Join("model", "records")) {
		t.Fatalf("expected only the ordinary plural model directory violation, got %#v", violations)
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

// TestCheckAllowedDirectoriesAcceptsConventionalProjectDirectories proves the
// directories a project conventionally keeps beside its packages — test
// suites, development scripts and Helm charts, next to the deployment
// manifests and operator scripts — pass the directory check, while a
// directory the project structure has no place for is still reported.
func TestCheckServiceFileBoundaryCountsFrameworkServiceStructs(t *testing.T) {
	oldServiceDir := serviceDir
	t.Cleanup(func() { serviceDir = oldServiceDir })

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	serviceDir = "service"
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

	violations := CheckServiceFileBoundary(newProjectIgnoreMatcher())

	if len(violations) != 1 {
		t.Fatalf("expected one violation, got %#v", violations)
	}
	assertViolationContains(t, violations, filepath.Join("service", "record", "record.go"), "should contain at most one service struct (found: Creator, Lister)")
}

func TestCheckAllowedDirectoriesAcceptsConventionalProjectDirectories(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n\nrequire github.com/hydroan/gst v0.0.0\n")
	for _, dir := range []string{"deploy", "scripts", "test", "hack", "charts", "sample"} {
		if err := os.MkdirAll(filepath.Join(projectDir, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	violations := CheckAllowedDirectories(newProjectIgnoreMatcher())
	want := []string{"Directory 'sample' is not allowed in project structure"}
	if !slices.Equal(violations, want) {
		t.Fatalf("expected only the unplaced directory reported, got %#v", violations)
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

// sourceLine returns the 1-based number of the first line of source that
// contains fragment, so a test can name the line a report points at without
// restating its number.
func sourceLine(t *testing.T, source string, fragment string) int {
	t.Helper()

	index := slices.IndexFunc(strings.Split(source, "\n"), func(line string) bool {
		return strings.Contains(line, fragment)
	})
	if index < 0 {
		t.Fatalf("no line of the source contains %q", fragment)
	}
	return index + 1
}

// writeCheckProjectGoMod writes the fixture project's go.mod together with the
// minimal framework source it resolves through the go module graph: checks
// that exempt copied framework modules resolve the framework source for every
// project they inspect.
func writeCheckProjectGoMod(t *testing.T, projectDir string) {
	t.Helper()

	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n\nrequire github.com/hydroan/gst v0.0.0-00010101000000-000000000000\n\nreplace github.com/hydroan/gst => ./internal/gst\n")
	writeCheckFile(t, filepath.Join(projectDir, "internal", "gst", "go.mod"), "module github.com/hydroan/gst\n\ngo 1.26\n")
	if err := os.MkdirAll(filepath.Join(projectDir, "internal", "gst", "module"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// writeCheckProjectGoModAgainstRealFramework writes the fixture project's
// go.mod resolving the framework to this repository's own source tree. Gen
// fixtures need it because the generated inspection program compiles against
// the framework for real, which a stub source tree cannot satisfy. The
// fixture reuses this repository's own go.mod requirements and go.sum so the
// build resolves the exact dependency versions the framework pins, instead of
// re-resolving the graph from scratch (which trips over ambiguous-import
// splits such as google.golang.org/genproto).
func writeCheckProjectGoModAgainstRealFramework(t *testing.T, projectDir string) {
	t.Helper()

	root := frameworkRepoRoot(t)
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Replace(string(goMod), "module github.com/hydroan/gst\n", "module tmpapp\n", 1)
	content += "\nrequire github.com/hydroan/gst v0.0.0-00010101000000-000000000000\n\nreplace github.com/hydroan/gst => " + root + "\n"
	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), content)
	goSum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	writeCheckFile(t, filepath.Join(projectDir, "go.sum"), string(goSum))
	recordFrameworkSources(t, root)
}

// recordFrameworkSources reads the framework's Go sources under root. The
// programs these fixtures build compile against those sources in a child go
// command, which go test does not see as an input of the test; reading them
// here does, so a change to the framework reruns the test instead of replaying
// a cached result that no longer holds.
func recordFrameworkSources(t *testing.T, root string) {
	t.Helper()

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if path != root && (name == "examples" || name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		_, readErr := os.ReadFile(path)
		return readErr
	})
	if err != nil {
		t.Fatal(err)
	}
}

// newGenProject creates a temporary project for tests that run gg gen, makes
// it the working directory and points the gg command globals at its
// conventional layout, restoring them when the test ends. Its go.mod resolves
// the framework to this repository, since generation compiles the column
// inspection program against the framework for real. Generation also caches
// that inspection under the user cache directory, keyed by project directory;
// nothing ever reads a throwaway project's entry again, so the entry is removed
// when the test ends.
func newGenProject(t *testing.T) string {
	t.Helper()

	oldModelDir := modelDir
	oldServiceDir := serviceDir
	oldRouterDir := routerDir
	oldDaoDir := daoDir
	oldExcludes := excludes
	oldModule := module
	oldPrune := prune
	oldCleanOrphans := cleanOrphans
	t.Cleanup(func() {
		modelDir = oldModelDir
		serviceDir = oldServiceDir
		routerDir = oldRouterDir
		daoDir = oldDaoDir
		excludes = oldExcludes
		module = oldModule
		prune = oldPrune
		cleanOrphans = oldCleanOrphans
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"
	serviceDir = "service"
	routerDir = "router"
	daoDir = "dao"
	excludes = nil
	module = ""
	prune = false
	cleanOrphans = false

	writeCheckProjectGoModAgainstRealFramework(t, projectDir)
	cacheDir, err := columnsCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if removeErr := os.RemoveAll(cacheDir); removeErr != nil {
			t.Error(removeErr)
		}
	})
	return projectDir
}

// frameworkRepoRoot returns the absolute path of this repository's root.
func frameworkRepoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}
