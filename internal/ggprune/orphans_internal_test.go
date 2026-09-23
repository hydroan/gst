package ggprune

import (
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconst"
)

func TestCurrentServiceDirsUsesFlattenTarget(t *testing.T) {
	got := currentServiceDirs([]*gen.ModelInfo{flattenPruneModel()})
	wantDir := filepath.Clean(filepath.Join(ggconst.DirService, "authz"))
	oldDir := filepath.Clean(filepath.Join(ggconst.DirService, "authz", "role"))

	if len(got.ownedDirs) != 1 || got.ownedDirs[0] != wantDir {
		t.Fatalf("ownedDirs = %v, want [%s]", got.ownedDirs, wantDir)
	}
	if !got.knownDirs[wantDir] {
		t.Fatalf("knownDirs missing flattened dir %q", wantDir)
	}
	if got.knownDirs[oldDir] {
		t.Fatalf("knownDirs should not include old nested dir %q", oldDir)
	}
}
