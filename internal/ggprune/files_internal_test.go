package ggprune

import (
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/modelinfo"
)

func TestCurrentServiceFilesUsesFlattenTarget(t *testing.T) {
	models := []*modelinfo.Model{flattenPruneModel()}
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
func flattenPruneModel() *modelinfo.Model {
	disabled := func(phase apiaction.Phase) *dsl.Action {
		return &dsl.Action{Phase: phase}
	}
	return &modelinfo.Model{
		ModulePath:    "github.com/acme/app",
		ModelPkgName:  "authz",
		ModelName:     "Role",
		ModelFileDir:  filepath.Join(ggconst.DirModel, "authz"),
		ModelFilePath: filepath.Join(ggconst.DirModel, "authz", "role.go"),
		Design: &dsl.Design{
			Endpoint: "authz/roles",
			Create: &dsl.Action{
				Service:     true,
				ServiceName: "role",
				Flatten:     true,
				Phase:       apiaction.Create,
			},
			Delete:     disabled(apiaction.Delete),
			Update:     disabled(apiaction.Update),
			Patch:      disabled(apiaction.Patch),
			List:       disabled(apiaction.List),
			Get:        disabled(apiaction.Get),
			CreateMany: disabled(apiaction.CreateMany),
			DeleteMany: disabled(apiaction.DeleteMany),
			UpdateMany: disabled(apiaction.UpdateMany),
			PatchMany:  disabled(apiaction.PatchMany),
			Import:     disabled(apiaction.Import),
			Export:     disabled(apiaction.Export),
			SSE:        disabled(apiaction.SSE),
			Stream:     disabled(apiaction.Stream),
		},
	}
}
