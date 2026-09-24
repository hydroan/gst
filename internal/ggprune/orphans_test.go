package ggprune_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/ggprune"
)

func TestFindOrphanDirsFlagsUnreferencedDirs(t *testing.T) {
	setupOrphanPruneProject(t)

	writeProjectFile(t, filepath.Join("service", "authz", "role.go"), `package authz
`)
	writeProjectFile(t, filepath.Join("service", "leftover", "leftover.go"), `package leftover
`)

	orphans, keptHelpers := findOrphanDirs(t, []*gen.ModelInfo{orphanPruneModel()}, nil, ggconfig.PruneConfig{})

	wantDir := filepath.Join("service", "leftover")
	if len(orphans) != 1 || orphans[0].Path != wantDir {
		t.Fatalf("orphans = %#v, want single dir %q", orphans, wantDir)
	}
	wantFile := filepath.Join(wantDir, "leftover.go")
	if len(orphans[0].Files) != 1 || orphans[0].Files[0] != wantFile {
		t.Fatalf("orphans[0].Files = %#v, want [%s]", orphans[0].Files, wantFile)
	}
	if len(keptHelpers) != 0 {
		t.Fatalf("unreferenced dir should not be reported as kept helper, got %#v", keptHelpers)
	}
}

func TestFindOrphanDirsKeepsImportedHelperDirs(t *testing.T) {
	setupOrphanPruneProject(t)

	writeProjectFile(t, filepath.Join("service", "authz", "role.go"), `package authz

import _ "tmpapp/service/adminauth"
`)
	writeProjectFile(t, filepath.Join("service", "adminauth", "adminauth.go"), `package adminauth
`)

	orphans, keptHelpers := findOrphanDirs(t, []*gen.ModelInfo{orphanPruneModel()}, nil, ggconfig.PruneConfig{})

	if len(orphans) != 0 {
		t.Fatalf("imported helper dir should not be an orphan, got %#v", orphans)
	}
	wantDir := filepath.Join("service", "adminauth")
	if len(keptHelpers) != 1 || keptHelpers[0].Path != wantDir {
		t.Fatalf("keptHelpers = %#v, want single dir %q", keptHelpers, wantDir)
	}
	wantFile := filepath.Join(wantDir, "adminauth.go")
	if len(keptHelpers[0].Files) != 1 || keptHelpers[0].Files[0] != wantFile {
		t.Fatalf("keptHelpers[0].Files = %#v, want [%s]", keptHelpers[0].Files, wantFile)
	}
}

func TestFindOrphanDirsKeepsTransitiveHelperImports(t *testing.T) {
	setupOrphanPruneProject(t)

	writeProjectFile(t, filepath.Join("service", "authz", "role.go"), `package authz

import _ "tmpapp/service/helpera"
`)
	writeProjectFile(t, filepath.Join("service", "helpera", "helpera.go"), `package helpera

import _ "tmpapp/service/helperb"
`)
	writeProjectFile(t, filepath.Join("service", "helperb", "helperb.go"), `package helperb
`)

	orphans, keptHelpers := findOrphanDirs(t, []*gen.ModelInfo{orphanPruneModel()}, nil, ggconfig.PruneConfig{})

	if len(orphans) != 0 {
		t.Fatalf("transitively imported helper dirs should not be orphans, got %#v", orphans)
	}
	if len(keptHelpers) != 2 {
		t.Fatalf("keptHelpers = %#v, want helpera and helperb", keptHelpers)
	}
	wantDirs := []string{filepath.Join("service", "helpera"), filepath.Join("service", "helperb")}
	for i, want := range wantDirs {
		if keptHelpers[i].Path != want {
			t.Fatalf("keptHelpers[%d].Path = %q, want %q", i, keptHelpers[i].Path, want)
		}
	}
}

func TestFindOrphanDirsKeepsHelperDirsImportedByKeptDirs(t *testing.T) {
	setupOrphanPruneProject(t)

	// The service file belongs to a gst.yaml-ignored action: no model action
	// maps to it, but keptDirs marks its directory as owned.
	writeProjectFile(t, filepath.Join("service", "iam", "user", "list.go"), `package user

import _ "tmpapp/service/iam/adminauth"
`)
	writeProjectFile(t, filepath.Join("service", "iam", "adminauth", "adminauth.go"), `package adminauth
`)

	keptDirs := map[string]bool{filepath.Join("service", "iam", "user"): true}
	orphans, keptHelpers := findOrphanDirs(t, nil, keptDirs, ggconfig.PruneConfig{})

	if len(orphans) != 0 {
		t.Fatalf("helper dir imported by kept service files should not be an orphan, got %#v", orphans)
	}
	wantDir := filepath.Join("service", "iam", "adminauth")
	if len(keptHelpers) != 1 || keptHelpers[0].Path != wantDir {
		t.Fatalf("keptHelpers = %#v, want single dir %q", keptHelpers, wantDir)
	}
}

// TestFindOrphanDirsKeepsHelperDirsImportedByLiveCode pins that any live
// code of the project vouches for the service directory it imports, not only
// the code of the directories models own: code outside the service directory,
// a test file, the service root and a directory above a model's own or above
// a kept helper's, and what prune.ignore keeps, down to a single file inside
// an orphan. Deleting the directory would break the build, or the tests for a
// test file. A file a build constraint leaves out counts too: prune errs on the
// side of keeping.
func TestFindOrphanDirsKeepsHelperDirsImportedByLiveCode(t *testing.T) {
	importers := []struct {
		name    string
		path    string
		pkg     string
		header  string
		tail    string
		protect []string
		extra   map[string]string
	}{
		{name: "cronjob", path: filepath.Join("cronjob", "cleanup.go"), pkg: "cronjob"},
		{name: "file a build constraint leaves out", path: filepath.Join("cronjob", "tool.go"), pkg: "main", header: "//go:build ignore\n\n"},
		{name: "nested package", path: filepath.Join("internal", "task", "run.go"), pkg: "task"},
		{name: "project root", path: "wire.go", pkg: "main"},
		{name: "test file", path: filepath.Join("cronjob", "cleanup_test.go"), pkg: "cronjob"},
		{name: "service root", path: filepath.Join("service", "hooks.go"), pkg: "service"},
		{name: "directory above a model's", path: filepath.Join("service", "authz", "common.go"), pkg: "authz"},
		{
			name: "directory above a kept helper's",
			path: filepath.Join("service", "legacy", "legacy.go"),
			pkg:  "legacy",
			extra: map[string]string{
				filepath.Join("cronjob", "cleanup.go"):                "package cronjob\n\nimport _ \"tmpapp/service/legacy/util\"\n",
				filepath.Join("service", "legacy", "util", "util.go"): "package util\n",
			},
		},
		{name: "directory prune.ignore covers", path: filepath.Join("service", "legacy", "legacy.go"), pkg: "legacy", protect: []string{"service/legacy"}},
		{
			name:    "file prune.ignore covers in an orphan",
			path:    filepath.Join("service", "legacy", "kept.go"),
			pkg:     "legacy",
			protect: []string{"service/legacy/kept.go"},
			extra:   map[string]string{filepath.Join("service", "legacy", "util.go"): "package legacy\n"},
		},
	}
	for _, importer := range importers {
		t.Run(importer.name, func(t *testing.T) {
			setupHelperImportProject(t)
			for path, content := range importer.extra {
				writeProjectFile(t, path, content)
			}
			writeProjectFile(t, importer.path, importer.header+"package "+importer.pkg+"\n\nimport _ \"tmpapp/service/helper\"\n"+importer.tail)

			orphans, keptHelpers := findOrphanDirs(t, []*gen.ModelInfo{helperImportModel()}, nil, ggconfig.PruneConfig{Ignore: importer.protect})

			wantDir := filepath.Join("service", "helper")
			if slices.ContainsFunc(orphans, func(orphan ggprune.OrphanDir) bool { return orphan.Path == wantDir }) {
				t.Fatalf("helper dir imported by %s should not be an orphan, got %#v", importer.path, orphans)
			}
			if !slices.ContainsFunc(keptHelpers, func(helper ggprune.OrphanDir) bool { return helper.Path == wantDir }) {
				t.Fatalf("keptHelpers = %#v, want %q among them", keptHelpers, wantDir)
			}
		})
	}
}

// TestFindOrphanDirsIgnoresImportsFromCodeThatIsNotLive pins what vouches for
// nothing: a .gen.go file, whose imports follow the models it was generated
// from, so a stale service.gen.go keeps nothing a deleted model left behind; a
// directory no model owns, or a leftover would keep the helper it imports; a
// file this prune deletes besides the orphans, such as the service file of a
// disabled action or the middleware of a removed module, even in a directory
// a model owns; and code the project ignores, which gg check and gg gen
// do not read either: a file the project's Git ignore rules exclude or whose
// name begins with "_", and one below testdata, vendor, a hidden or an
// underscored directory, a nested module or a directory the go.mod ignores,
// the testdata next to a model's service files included.
func TestFindOrphanDirsIgnoresImportsFromCodeThatIsNotLive(t *testing.T) {
	importers := []struct {
		name    string
		path    string
		pkg     string
		deleted bool
		extra   map[string]string
	}{
		{name: "generated file", path: filepath.Join("service", "service.gen.go"), pkg: "service"},
		{name: "directory no model owns", path: filepath.Join("service", "leftover", "leftover.go"), pkg: "leftover"},
		{name: "middleware prune deletes", path: filepath.Join("middleware", "sample_auth.go"), pkg: "middleware", deleted: true},
		{name: "service file prune deletes", path: filepath.Join("service", "authz", "role", "list.go"), pkg: "role", deleted: true},
		{name: "Git ignored file", path: filepath.Join("cronjob", "local.go"), pkg: "cronjob", extra: map[string]string{".gitignore": "cronjob/local.go\n"}},
		{name: "Git ignored directory", path: filepath.Join("scratch", "try.go"), pkg: "scratch", extra: map[string]string{".gitignore": "scratch/\n"}},
		{name: "underscored file", path: filepath.Join("cronjob", "_old.go"), pkg: "cronjob"},
		{name: "underscored directory", path: filepath.Join("_draft", "draft.go"), pkg: "draft"},
		{name: "hidden directory", path: filepath.Join(".cache", "copy.go"), pkg: "cached"},
		{name: "testdata", path: filepath.Join("testdata", "fixture.go"), pkg: "fixture"},
		{name: "testdata in a model's service directory", path: filepath.Join("service", "authz", "role", "testdata", "fixture.go"), pkg: "fixture"},
		{name: "vendor", path: filepath.Join("vendor", "example.com", "lib", "lib.go"), pkg: "lib"},
		{name: "nested module", path: filepath.Join("tools", "main.go"), pkg: "main", extra: map[string]string{filepath.Join("tools", "go.mod"): "module tmpapp/tools\n\ngo 1.26\n"}},
		{name: "directory the go.mod ignores", path: filepath.Join("web", "web.go"), pkg: "web", extra: map[string]string{"go.mod": "module tmpapp\n\ngo 1.26\n\nignore ./web\n"}},
	}
	for _, importer := range importers {
		t.Run(importer.name, func(t *testing.T) {
			setupHelperImportProject(t)
			for path, content := range importer.extra {
				writeProjectFile(t, path, content)
			}
			writeProjectFile(t, importer.path, "package "+importer.pkg+"\n\nimport _ \"tmpapp/service/helper\"\n")
			var deleting []string
			if importer.deleted {
				deleting = append(deleting, importer.path)
			}

			orphans, keptHelpers := findOrphanDirs(t, []*gen.ModelInfo{helperImportModel()}, nil, ggconfig.PruneConfig{}, deleting...)

			wantDir := filepath.Join("service", "helper")
			if !slices.ContainsFunc(orphans, func(orphan ggprune.OrphanDir) bool { return orphan.Path == wantDir }) {
				t.Fatalf("orphans = %#v, want %q among them", orphans, wantDir)
			}
			if len(keptHelpers) != 0 {
				t.Fatalf("an import from %s should keep nothing, got %#v", importer.path, keptHelpers)
			}
		})
	}
}

// TestFindOrphanDirsFailsOnImportsItCannotRead pins that orphan detection
// fails instead of guessing when it cannot read the imports of part of the
// project: a directory or a file it may not open, or a file whose imports do
// not parse, even after some of them did. An import in the part it could not
// read might be all that keeps a directory, and the error names that part.
func TestFindOrphanDirsFailsOnImportsItCannotRead(t *testing.T) {
	tests := []struct {
		name   string
		locked string
		broken string
		want   string
	}{
		{name: "directory it may not open", locked: "data", want: "data"},
		{name: "file it may not open", locked: filepath.Join("cronjob", "job.go"), want: filepath.Join("cronjob", "job.go")},
		{name: "imports that stop parsing", broken: "package cronjob\n\nimport _ \"tmpapp/service/helper\"\n\nimport \"unterminated\n", want: filepath.Join("cronjob", "broken.go")},
		{name: "first import that does not parse", broken: "package cronjob\n\nimport (\n\talias extra \"tmpapp/service/helper\"\n)\n", want: filepath.Join("cronjob", "broken.go")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.locked != "" && os.Geteuid() == 0 {
				t.Skip("root reads a directory or a file whatever its permissions")
			}
			setupHelperImportProject(t)
			writeProjectFile(t, filepath.Join("data", "job.go"), "package data\n\nimport _ \"tmpapp/service/helper\"\n")
			writeProjectFile(t, filepath.Join("cronjob", "job.go"), "package cronjob\n\nimport _ \"tmpapp/service/helper\"\n")
			if tt.broken != "" {
				writeProjectFile(t, filepath.Join("cronjob", "broken.go"), tt.broken)
			}
			if tt.locked != "" {
				lockPath(t, tt.locked)
			}

			orphans, keptHelpers, err := ggprune.FindOrphanDirs([]*gen.ModelInfo{helperImportModel()}, nil, nil, "tmpapp", gghelper.NewProjectIgnore(), ggconfig.PruneConfig{})

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("FindOrphanDirs() error = %v, want one naming %s", err, tt.want)
			}
			if len(orphans) != 0 || len(keptHelpers) != 0 {
				t.Fatalf("FindOrphanDirs() = %#v, %#v, want no directories alongside the error", orphans, keptHelpers)
			}
		})
	}
}

// TestFindOrphanDirsSkipsIgnoredCodeItCannotRead pins that code the project
// ignores cannot fail orphan detection either, as it is left out before it is
// read: a directory it may not open, such as the volume of a database
// container, that the project's Git ignore rules exclude or its go.mod
// ignores, a file it may not open below a Git ignored directory, and a file
// below testdata whose imports do not parse, such as a template. The helper
// only that code imports is an orphan.
func TestFindOrphanDirsSkipsIgnoredCodeItCannotRead(t *testing.T) {
	importer := "package data\n\nimport _ \"tmpapp/service/helper\"\n"
	tests := []struct {
		name   string
		files  map[string]string
		locked string
	}{
		{
			name:   "Git ignored directory it may not open",
			files:  map[string]string{".gitignore": "data/\n", filepath.Join("data", "job.go"): importer},
			locked: "data",
		},
		{
			name:   "directory the go.mod ignores it may not open",
			files:  map[string]string{"go.mod": "module tmpapp\n\ngo 1.26\n\nignore ./data\n", filepath.Join("data", "job.go"): importer},
			locked: "data",
		},
		{
			name:   "file it may not open in a Git ignored directory",
			files:  map[string]string{".gitignore": "data/\n", filepath.Join("data", "job.go"): importer},
			locked: filepath.Join("data", "job.go"),
		},
		{
			name:  "testdata file whose imports do not parse",
			files: map[string]string{filepath.Join("testdata", "template.go"): "package {{.Package}}\n\nimport _ \"tmpapp/service/helper\"\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.locked != "" && os.Geteuid() == 0 {
				t.Skip("root reads a directory or a file whatever its permissions")
			}
			setupHelperImportProject(t)
			for path, content := range tt.files {
				writeProjectFile(t, path, content)
			}
			if tt.locked != "" {
				lockPath(t, tt.locked)
			}

			orphans, keptHelpers := findOrphanDirs(t, []*gen.ModelInfo{helperImportModel()}, nil, ggconfig.PruneConfig{})

			wantDir := filepath.Join("service", "helper")
			if !slices.ContainsFunc(orphans, func(orphan ggprune.OrphanDir) bool { return orphan.Path == wantDir }) {
				t.Fatalf("orphans = %#v, want %q among them", orphans, wantDir)
			}
			if len(keptHelpers) != 0 {
				t.Fatalf("code the project ignores should keep nothing, got %#v", keptHelpers)
			}
		})
	}
}

// TestFindOrphanDirsStaysOutOfSymlinkedDirectories pins that the walk for live
// code does not follow a symbolic link to a directory, as gg check's walks
// and the go command's ./... pattern do not: code reached only through one
// vouches for nothing.
func TestFindOrphanDirsStaysOutOfSymlinkedDirectories(t *testing.T) {
	outside := t.TempDir()
	writeProjectFile(t, filepath.Join(outside, "job.go"), "package job\n\nimport _ \"tmpapp/service/helper\"\n")
	setupHelperImportProject(t)
	if err := os.Symlink(outside, "linked"); err != nil {
		t.Fatal(err)
	}

	orphans, keptHelpers := findOrphanDirs(t, []*gen.ModelInfo{helperImportModel()}, nil, ggconfig.PruneConfig{})

	wantDir := filepath.Join("service", "helper")
	if !slices.ContainsFunc(orphans, func(orphan ggprune.OrphanDir) bool { return orphan.Path == wantDir }) {
		t.Fatalf("orphans = %#v, want %q among them", orphans, wantDir)
	}
	if len(keptHelpers) != 0 {
		t.Fatalf("code behind a symbolic link should keep nothing, got %#v", keptHelpers)
	}
}

// TestFindOrphanDirsKeepsNothingForAMissingImport pins that an import of a
// service package no directory on disk holds keeps nothing: there is nothing
// to keep.
func TestFindOrphanDirsKeepsNothingForAMissingImport(t *testing.T) {
	setupHelperImportProject(t)
	writeProjectFile(t, filepath.Join("cronjob", "cleanup.go"), "package cronjob\n\nimport _ \"tmpapp/service/missing\"\n")

	_, keptHelpers := findOrphanDirs(t, []*gen.ModelInfo{helperImportModel()}, nil, ggconfig.PruneConfig{})

	if len(keptHelpers) != 0 {
		t.Fatalf("keptHelpers = %#v, want none for an import of a missing directory", keptHelpers)
	}
}

// TestFindOrphanDirsJudgesEveryDirectoryOnItsOwn pins that prune judges a
// directory the go command or the project's Git ignore rules leave out like
// any other: a hidden, underscored, vendor or testdata directory, a nested
// module, and a Git ignored directory no model owns are orphans, even right
// below a directory orphan cleanup leaves alone, such as the testdata next to
// the tests of service/authz. Only prune.ignore would keep them. Below an
// orphan they are cleaned with it.
func TestFindOrphanDirsJudgesEveryDirectoryOnItsOwn(t *testing.T) {
	setupHelperImportProject(t)
	for path, content := range map[string]string{
		filepath.Join("service", "authz", "common_test.go"):                "package authz\n",
		filepath.Join("service", "authz", "testdata", "input.json"):        "{}\n",
		filepath.Join("service", ".cache", "state.json"):                   "{}\n",
		filepath.Join("service", "_old", "old.go"):                         "package old\n",
		filepath.Join("service", "vendor", "example.com", "lib", "lib.go"): "package lib\n",
		filepath.Join("service", "tools", "go.mod"):                        "module tmpapp/service/tools\n\ngo 1.26\n",
		filepath.Join("service", "tools", "main.go"):                       "package main\n",
		filepath.Join("service", "legacy", "legacy.go"):                    "package legacy\n",
		filepath.Join("service", "legacy", "testdata", "input.json"):       "{}\n",
		".gitignore": "service/scratch/\n",
		filepath.Join("service", "scratch", "try.go"): "package scratch\n",
	} {
		writeProjectFile(t, path, content)
	}

	orphans, _ := findOrphanDirs(t, []*gen.ModelInfo{helperImportModel()}, nil, ggconfig.PruneConfig{})

	want := []ggprune.OrphanDir{
		{Path: filepath.Join("service", ".cache"), Files: []string{filepath.Join("service", ".cache", "state.json")}},
		{Path: filepath.Join("service", "_old"), Files: []string{filepath.Join("service", "_old", "old.go")}},
		{Path: filepath.Join("service", "helper"), Files: []string{filepath.Join("service", "helper", "helper.go")}},
		{Path: filepath.Join("service", "legacy"), Files: []string{filepath.Join("service", "legacy", "legacy.go"), filepath.Join("service", "legacy", "testdata", "input.json")}},
		{Path: filepath.Join("service", "scratch"), Files: []string{filepath.Join("service", "scratch", "try.go")}},
		{Path: filepath.Join("service", "tools"), Files: []string{filepath.Join("service", "tools", "go.mod"), filepath.Join("service", "tools", "main.go")}},
		{Path: filepath.Join("service", "vendor"), Files: []string{filepath.Join("service", "vendor", "example.com", "lib", "lib.go")}},
		{Path: filepath.Join("service", "authz", "testdata"), Files: []string{filepath.Join("service", "authz", "testdata", "input.json")}},
	}
	if !reflect.DeepEqual(orphans, want) {
		t.Fatalf("orphans = %#v, want %#v", orphans, want)
	}
}

// TestFindOrphanDirsLeavesOutWhatPruneIgnoreCovers pins that a
// gst.yaml prune.ignore entry keeps what it covers out of orphan cleanup: a
// covered directory is no orphan, and a covered file inside an orphan stays
// out of the files cleaning the orphan deletes.
func TestFindOrphanDirsLeavesOutWhatPruneIgnoreCovers(t *testing.T) {
	setupOrphanPruneProject(t)

	writeProjectFile(t, filepath.Join("service", "authz", "role.go"), "package authz\n")
	writeProjectFile(t, filepath.Join("service", "kept", "kept.go"), "package kept\n")
	writeProjectFile(t, filepath.Join("service", "legacy", "helper.go"), "package legacy\n")
	writeProjectFile(t, filepath.Join("service", "legacy", "util.go"), "package legacy\n")
	protect := ggconfig.PruneConfig{Ignore: []string{"service/kept", "service/legacy/helper.go"}}

	orphans, _ := findOrphanDirs(t, []*gen.ModelInfo{orphanPruneModel()}, nil, protect)

	wantDir := filepath.Join("service", "legacy")
	if len(orphans) != 1 || orphans[0].Path != wantDir {
		t.Fatalf("orphans = %#v, want single dir %q", orphans, wantDir)
	}
	wantFile := filepath.Join(wantDir, "util.go")
	if len(orphans[0].Files) != 1 || orphans[0].Files[0] != wantFile {
		t.Fatalf("orphans[0].Files = %#v, want [%s]", orphans[0].Files, wantFile)
	}
}

// setupOrphanPruneProject moves the test into a temporary project root holding
// a go.mod.
func setupOrphanPruneProject(t *testing.T) {
	t.Helper()

	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeProjectFile(t, "go.mod", "module tmpapp\n\ngo 1.26\n")
}

// findOrphanDirs runs ggprune.FindOrphanDirs over the test project, whose
// module is tmpapp, reading it through the project's ignore rules as the test
// left them and failing the test on an error. deleting are the files prune
// deletes besides the orphans' own.
func findOrphanDirs(t *testing.T, models []*gen.ModelInfo, keptDirs map[string]bool, protect ggconfig.PruneConfig, deleting ...string) (orphans, keptHelpers []ggprune.OrphanDir) {
	t.Helper()

	orphans, keptHelpers, err := ggprune.FindOrphanDirs(models, keptDirs, deleting, "tmpapp", gghelper.NewProjectIgnore(), protect)
	if err != nil {
		t.Fatal(err)
	}
	return orphans, keptHelpers
}

// setupHelperImportProject moves the test into a project where the model of
// helperImportModel owns service/authz/role and no model owns service/helper,
// for the test to import it from somewhere.
func setupHelperImportProject(t *testing.T) {
	t.Helper()

	setupOrphanPruneProject(t)
	writeProjectFile(t, filepath.Join("service", "authz", "role", "role.go"), "package role\n")
	writeProjectFile(t, filepath.Join("service", "helper", "helper.go"), "package helper\n")
}

// lockPath takes every permission away from the file or directory at path,
// the way a volume a container owns looks to the user, and gives it back
// before the temporary project is removed.
func lockPath(t *testing.T, path string) {
	t.Helper()

	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(abs, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(abs, 0o755) })
}

// writeProjectFile writes content to path, creating its parent directories.
func writeProjectFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// orphanPruneModel returns a model whose only enabled action targets
// service/authz, mirroring a project-owned service directory.
func orphanPruneModel() *gen.ModelInfo {
	disabled := func(phase consts.Phase) *dsl.Action {
		return &dsl.Action{Phase: phase}
	}
	return &gen.ModelInfo{
		ModulePath:    "tmpapp",
		ModelPkgName:  "authz",
		ModelName:     "Role",
		ModelFileDir:  filepath.Join("model", "authz"),
		ModelFilePath: filepath.Join("model", "authz", "role.go"),
		Design: &dsl.Design{
			Enabled:  true,
			Endpoint: "authz/roles",
			Create: &dsl.Action{
				Enabled:  true,
				Service:  true,
				Filename: "role.go",
				Flatten:  true,
				Phase:    consts.PHASE_CREATE,
			},
			Delete:     disabled(consts.PHASE_DELETE),
			Update:     disabled(consts.PHASE_UPDATE),
			Patch:      disabled(consts.PHASE_PATCH),
			List:       disabled(consts.PHASE_LIST),
			Get:        disabled(consts.PHASE_GET),
			CreateMany: disabled(consts.PHASE_CREATE_MANY),
			DeleteMany: disabled(consts.PHASE_DELETE_MANY),
			UpdateMany: disabled(consts.PHASE_UPDATE_MANY),
			PatchMany:  disabled(consts.PHASE_PATCH_MANY),
			Import:     disabled(consts.PHASE_IMPORT),
			Export:     disabled(consts.PHASE_EXPORT),
			SSE:        disabled(consts.PHASE_SSE),
		},
	}
}

// helperImportModel returns the model of orphanPruneModel with its service
// file one level down, in service/authz/role, so that service/authz lies
// between the service root and the directory the model owns.
func helperImportModel() *gen.ModelInfo {
	m := orphanPruneModel()
	m.Design.Create.Flatten = false
	return m
}
