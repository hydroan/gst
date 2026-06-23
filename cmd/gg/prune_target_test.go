package main

import (
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/types/consts"
)

func TestCurrentServiceFilesUsesFlattenTarget(t *testing.T) {
	oldModelDir := modelDir
	oldServiceDir := serviceDir
	t.Cleanup(func() {
		modelDir = oldModelDir
		serviceDir = oldServiceDir
	})

	modelDir = filepath.Join("repo", "model")
	serviceDir = filepath.Join("repo", "service")

	models := []*gen.ModelInfo{flattenPruneModel()}
	got := currentServiceFiles(models)

	wantCurrent := filepath.Join(serviceDir, "authz", "role.go")
	wantOld := filepath.Join(serviceDir, "authz", "role", "role.go")
	if !got[wantCurrent] {
		t.Fatalf("currentServiceFiles missing flattened file %q", wantCurrent)
	}
	if got[wantOld] {
		t.Fatalf("currentServiceFiles should not include old nested file %q", wantOld)
	}
}

func TestCurrentServiceDirsUsesFlattenTarget(t *testing.T) {
	oldModelDir := modelDir
	oldServiceDir := serviceDir
	t.Cleanup(func() {
		modelDir = oldModelDir
		serviceDir = oldServiceDir
	})

	modelDir = filepath.Join("repo", "model")
	serviceDir = filepath.Join("repo", "service")

	got := currentServiceDirs([]*gen.ModelInfo{flattenPruneModel()})
	wantDir := filepath.Clean(filepath.Join(serviceDir, "authz"))
	oldDir := filepath.Clean(filepath.Join(serviceDir, "authz", "role"))

	if len(got.ModelDirs) != 1 || got.ModelDirs[0] != wantDir {
		t.Fatalf("ModelDirs = %v, want [%s]", got.ModelDirs, wantDir)
	}
	if !got.KnownDirs[wantDir] {
		t.Fatalf("KnownDirs missing flattened dir %q", wantDir)
	}
	if got.KnownDirs[oldDir] {
		t.Fatalf("KnownDirs should not include old nested dir %q", oldDir)
	}
}

func flattenPruneModel() *gen.ModelInfo {
	disabled := func(phase consts.Phase) *dsl.Action {
		return &dsl.Action{Phase: phase}
	}
	return &gen.ModelInfo{
		ModulePath:    "github.com/acme/app",
		ModelPkgName:  "authz",
		ModelName:     "Role",
		ModelFileDir:  filepath.Join("repo", "model", "authz"),
		ModelFilePath: filepath.Join("repo", "model", "authz", "role.go"),
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
