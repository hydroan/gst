package ggprune

import (
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconst"
)

func TestCurrentServiceFilesUsesFlattenTarget(t *testing.T) {
	models := []*gen.ModelInfo{flattenPruneModel()}
	got := currentServiceFiles(models)

	wantCurrent := filepath.Join(ggconst.DirService, "authz", "role.go")
	wantOld := filepath.Join(ggconst.DirService, "authz", "role", "role.go")
	if !got[wantCurrent] {
		t.Fatalf("currentServiceFiles missing flattened file %q", wantCurrent)
	}
	if got[wantOld] {
		t.Fatalf("currentServiceFiles should not include old nested file %q", wantOld)
	}
}

// flattenPruneModel returns a model whose only enabled action flattens its
// service file into service/authz/role.go.
func flattenPruneModel() *gen.ModelInfo {
	disabled := func(phase consts.Phase) *dsl.Action {
		return &dsl.Action{Phase: phase}
	}
	return &gen.ModelInfo{
		ModulePath:    "github.com/acme/app",
		ModelPkgName:  "authz",
		ModelName:     "Role",
		ModelFileDir:  filepath.Join(ggconst.DirModel, "authz"),
		ModelFilePath: filepath.Join(ggconst.DirModel, "authz", "role.go"),
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
