package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/types/consts"
)

func TestCollectOrphanServiceDirsKeepsImportedHelperDirs(t *testing.T) {
	setupOrphanPruneProject(t)

	writeOrphanPruneFile(t, filepath.Join("service", "authz", "role.go"), `package authz

import _ "tmpapp/service/adminauth"
`)
	writeOrphanPruneFile(t, filepath.Join("service", "adminauth", "adminauth.go"), `package adminauth
`)

	orphans, keptHelpers := collectOrphanServiceDirs([]*gen.ModelInfo{orphanPruneModel()}, nil)

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

func TestCollectOrphanServiceDirsKeepsTransitiveHelperImports(t *testing.T) {
	setupOrphanPruneProject(t)

	writeOrphanPruneFile(t, filepath.Join("service", "authz", "role.go"), `package authz

import _ "tmpapp/service/helpera"
`)
	writeOrphanPruneFile(t, filepath.Join("service", "helpera", "helpera.go"), `package helpera

import _ "tmpapp/service/helperb"
`)
	writeOrphanPruneFile(t, filepath.Join("service", "helperb", "helperb.go"), `package helperb
`)

	orphans, keptHelpers := collectOrphanServiceDirs([]*gen.ModelInfo{orphanPruneModel()}, nil)

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

func TestCollectOrphanServiceDirsKeepsHelperDirsImportedByKeptDirs(t *testing.T) {
	setupOrphanPruneProject(t)

	// The service file belongs to a gst.yaml-ignored action: no model action
	// maps to it, but keptDirs marks its directory as owned.
	writeOrphanPruneFile(t, filepath.Join("service", "iam", "user", "list.go"), `package user

import _ "tmpapp/service/iam/adminauth"
`)
	writeOrphanPruneFile(t, filepath.Join("service", "iam", "adminauth", "adminauth.go"), `package adminauth
`)

	keptDirs := map[string]bool{filepath.Join("service", "iam", "user"): true}
	orphans, keptHelpers := collectOrphanServiceDirs(nil, keptDirs)

	if len(orphans) != 0 {
		t.Fatalf("helper dir imported by kept service files should not be an orphan, got %#v", orphans)
	}
	wantDir := filepath.Join("service", "iam", "adminauth")
	if len(keptHelpers) != 1 || keptHelpers[0].Path != wantDir {
		t.Fatalf("keptHelpers = %#v, want single dir %q", keptHelpers, wantDir)
	}
}

func TestCollectOrphanServiceDirsFlagsUnreferencedDirs(t *testing.T) {
	setupOrphanPruneProject(t)

	writeOrphanPruneFile(t, filepath.Join("service", "authz", "role.go"), `package authz
`)
	writeOrphanPruneFile(t, filepath.Join("service", "leftover", "leftover.go"), `package leftover
`)

	orphans, keptHelpers := collectOrphanServiceDirs([]*gen.ModelInfo{orphanPruneModel()}, nil)

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

// setupOrphanPruneProject prepares a temporary project root with go.mod and
// points the modelDir/serviceDir globals at it.
func setupOrphanPruneProject(t *testing.T) {
	t.Helper()

	oldModelDir := modelDir
	oldServiceDir := serviceDir
	t.Cleanup(func() {
		modelDir = oldModelDir
		serviceDir = oldServiceDir
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	modelDir = "model"
	serviceDir = "service"

	writeOrphanPruneFile(t, "go.mod", "module tmpapp\n\ngo 1.26\n")
}

func writeOrphanPruneFile(t *testing.T, path string, content string) {
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
		},
	}
}
