package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/modfile"
)

// TestGolangciLintVersionMatchesTheFramework keeps the golangci-lint gg lint
// runs at the version the framework lints itself with: the version go.mod
// requires of the module that provides the golangci-lint package. Upgrading
// one without the other fails here, so the configuration gg new writes and
// the framework's own lint run stay on one version.
func TestGolangciLintVersionMatchesTheFramework(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	require.NoError(t, err)
	file, err := modfile.Parse("go.mod", data, nil)
	require.NoError(t, err)

	// The module providing a package is the required one with the longest
	// path the package path starts with.
	packagePath := golangciLintPackage + "/"
	var provider *modfile.Require
	for _, req := range file.Require {
		if strings.HasPrefix(packagePath, req.Mod.Path+"/") &&
			(provider == nil || len(req.Mod.Path) > len(provider.Mod.Path)) {
			provider = req
		}
	}
	require.NotNil(t, provider, "go.mod requires no module providing %s", golangciLintPackage)
	require.Equal(t, golangciLintVersion, provider.Mod.Version)
}
