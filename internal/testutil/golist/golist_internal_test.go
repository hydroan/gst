package golist

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPackagesStatsTheDirectoriesAChangeMoves pins what the package stats,
// which is what go test keys a cached result on: Packages the directory each
// pattern starts from, and GoFiles the package's directory and those above it
// up to that start, nothing beyond it and nothing for a package whose files
// the test does not ask for.
func TestPackagesStatsTheDirectoriesAChangeMoves(t *testing.T) {
	var stated []string
	original := stat
	stat = func(name string) (os.FileInfo, error) {
		stated = append(stated, name)
		return original(name)
	}
	t.Cleanup(func() { stat = original })
	own, err := os.Getwd()
	require.NoError(t, err)
	testutil := filepath.Dir(own)

	pkgs := Packages(t, "github.com/hydroan/gst/internal/testutil/...")
	require.Equal(t, []string{testutil}, stated)
	stated = nil
	for _, pkg := range pkgs {
		if pkg.Dir == own {
			pkg.GoFiles(t)
		}
	}
	require.Equal(t, []string{own, testutil}, stated)

	stated = nil
	pkgs = Packages(t, ".")
	require.Equal(t, []string{own}, stated)
	stated = nil
	pkgs[0].GoFiles(t)
	require.Equal(t, []string{own}, stated)
}
