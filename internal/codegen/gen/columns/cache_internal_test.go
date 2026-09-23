package columns

import (
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/stretchr/testify/require"
)

// TestColumnsCacheKeyCoversTheModelCode pins which files the cache key
// hashes: every model source a walk over the project's code reads, a model
// package named generated among them, and none it leaves out, such as a
// testdata directory.
func TestColumnsCacheKeyCoversTheModelCode(t *testing.T) {
	t.Chdir(t.TempDir())
	writeProjectFile(t, "go.mod", "module tmpapp\n\ngo 1.26\n")
	writeProjectFile(t, filepath.Join(ggconst.DirModel, "record.go"), "package model\n")
	generated := filepath.Join(ggconst.DirModel, "generated", "sample.go")
	fixture := filepath.Join(ggconst.DirModel, "testdata", "fixture.go")
	writeProjectFile(t, generated, "package generated\n")
	writeProjectFile(t, fixture, "package fixture\n")
	key := func() string {
		t.Helper()
		k, err := columnsCacheKey("program", ggconst.DirModel, gghelper.NewProjectIgnore())
		require.NoError(t, err)
		return k
	}

	before := key()
	writeProjectFile(t, generated, "package generated\n\ntype Sample struct{}\n")
	require.NotEqual(t, before, key(), "a change to the model package named generated must change the key")

	before = key()
	writeProjectFile(t, fixture, "package fixture\n\ntype Fixture struct{}\n")
	require.Equal(t, before, key(), "a change under testdata must leave the key alone")
}
