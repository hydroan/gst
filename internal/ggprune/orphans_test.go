package ggprune_test

import (
	"os"
	"path/filepath"
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

	orphans, keptHelpers := ggprune.FindOrphanDirs([]*gen.ModelInfo{orphanPruneModel()}, nil, "tmpapp", ggconfig.PruneConfig{}, gghelper.NewProjectIgnore())

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

	orphans, keptHelpers := ggprune.FindOrphanDirs([]*gen.ModelInfo{orphanPruneModel()}, nil, "tmpapp", ggconfig.PruneConfig{}, gghelper.NewProjectIgnore())

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
	orphans, keptHelpers := ggprune.FindOrphanDirs(nil, keptDirs, "tmpapp", ggconfig.PruneConfig{}, gghelper.NewProjectIgnore())

	if len(orphans) != 0 {
		t.Fatalf("helper dir imported by kept service files should not be an orphan, got %#v", orphans)
	}
	wantDir := filepath.Join("service", "iam", "adminauth")
	if len(keptHelpers) != 1 || keptHelpers[0].Path != wantDir {
		t.Fatalf("keptHelpers = %#v, want single dir %q", keptHelpers, wantDir)
	}
}

func TestFindOrphanDirsFlagsUnreferencedDirs(t *testing.T) {
	setupOrphanPruneProject(t)

	writeProjectFile(t, filepath.Join("service", "authz", "role.go"), `package authz
`)
	writeProjectFile(t, filepath.Join("service", "leftover", "leftover.go"), `package leftover
`)

	orphans, keptHelpers := ggprune.FindOrphanDirs([]*gen.ModelInfo{orphanPruneModel()}, nil, "tmpapp", ggconfig.PruneConfig{}, gghelper.NewProjectIgnore())

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

	orphans, _ := ggprune.FindOrphanDirs([]*gen.ModelInfo{orphanPruneModel()}, nil, "tmpapp", protect, gghelper.NewProjectIgnore())

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
