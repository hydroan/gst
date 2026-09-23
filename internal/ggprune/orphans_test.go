package ggprune_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/ggprune"
)

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
// test file.
func TestFindOrphanDirsKeepsHelperDirsImportedByLiveCode(t *testing.T) {
	importers := []struct {
		name    string
		path    string
		pkg     string
		protect []string
		extra   map[string]string
	}{
		{name: "cronjob", path: filepath.Join("cronjob", "cleanup.go"), pkg: "cronjob"},
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
			writeProjectFile(t, importer.path, "package "+importer.pkg+"\n\nimport _ \"tmpapp/service/helper\"\n")

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
// nothing: the code gg's walks over the project leave out, as gg check's do
// (testdata, vendor, hidden and Git ignored directories, nested modules); a
// .gen.go file, whose imports follow the models it was generated from, so a
// stale service.gen.go keeps nothing a deleted model left behind; and a
// directory no model owns, or a leftover would keep the helper it imports.
func TestFindOrphanDirsIgnoresImportsFromCodeThatIsNotLive(t *testing.T) {
	importers := []struct {
		name  string
		path  string
		pkg   string
		extra map[string]string
	}{
		{name: "testdata", path: filepath.Join("testdata", "fixture.go"), pkg: "fixture"},
		{name: "testdata in a model's service directory", path: filepath.Join("service", "authz", "role", "testdata", "fixture.go"), pkg: "fixture"},
		{name: "vendor", path: filepath.Join("vendor", "example.com", "lib", "lib.go"), pkg: "lib"},
		{name: "hidden directory", path: filepath.Join(".cache", "copy.go"), pkg: "cached"},
		{name: "Git ignored directory", path: filepath.Join("scratch", "try.go"), pkg: "scratch", extra: map[string]string{".gitignore": "scratch/\n"}},
		{name: "nested module", path: filepath.Join("tools", "main.go"), pkg: "main", extra: map[string]string{filepath.Join("tools", "go.mod"): "module tmpapp/tools\n\ngo 1.26\n"}},
		{name: "generated file", path: filepath.Join("service", "service.gen.go"), pkg: "service"},
		{name: "directory no model owns", path: filepath.Join("service", "leftover", "leftover.go"), pkg: "leftover"},
	}
	for _, importer := range importers {
		t.Run(importer.name, func(t *testing.T) {
			setupHelperImportProject(t)
			for path, content := range importer.extra {
				writeProjectFile(t, path, content)
			}
			writeProjectFile(t, importer.path, "package "+importer.pkg+"\n\nimport _ \"tmpapp/service/helper\"\n")

			orphans, keptHelpers := findOrphanDirs(t, []*gen.ModelInfo{helperImportModel()}, nil, ggconfig.PruneConfig{})

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

// TestFindOrphanDirsFailsOnAnUnreadableDirectory pins that orphan detection
// fails instead of guessing when part of the project cannot be read: an
// import in the unread part might be all that keeps a directory.
func TestFindOrphanDirsFailsOnAnUnreadableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a directory whatever its permissions")
	}
	setupHelperImportProject(t)
	writeProjectFile(t, filepath.Join("data", "job.go"), "package data\n\nimport _ \"tmpapp/service/helper\"\n")
	locked, err := filepath.Abs("data")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	// Give the permission back before the temporary directory is removed.
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	orphans, keptHelpers, err := ggprune.FindOrphanDirs([]*gen.ModelInfo{helperImportModel()}, nil, "tmpapp", ggconfig.PruneConfig{}, gghelper.NewProjectIgnore())

	if err == nil {
		t.Fatal("FindOrphanDirs() error = nil, want one for the unreadable data directory")
	}
	if len(orphans) != 0 || len(keptHelpers) != 0 {
		t.Fatalf("FindOrphanDirs() = %#v, %#v, want no directories alongside the error", orphans, keptHelpers)
	}
}

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
// module is tmpapp, failing the test on an error.
func findOrphanDirs(t *testing.T, models []*gen.ModelInfo, keptDirs map[string]bool, protect ggconfig.PruneConfig) (orphans, keptHelpers []ggprune.OrphanDir) {
	t.Helper()

	orphans, keptHelpers, err := ggprune.FindOrphanDirs(models, keptDirs, "tmpapp", protect, gghelper.NewProjectIgnore())
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
