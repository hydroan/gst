package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTestedPackageReadsTheTestMainOffTheImportConfig pins how a link is
// mapped to the package it tests: the import config names the test main
// package as the import path with ".test" appended.
func TestTestedPackageReadsTheTestMainOffTheImportConfig(t *testing.T) {
	importcfg := filepath.Join(t.TempDir(), "importcfg.link")
	require.NoError(t, os.WriteFile(importcfg, []byte(`# import config
packagefile fmt=/cache/fmt.a
packagefile example.com/m/pkg=/cache/pkg.a
packagefile example.com/m/pkg.test=/work/b001/_pkg_.a
`), 0o600))

	tested, err := testedPackage(importcfg)
	require.NoError(t, err)
	require.Equal(t, "example.com/m/pkg", tested)

	require.NoError(t, os.WriteFile(importcfg, []byte("packagefile fmt=/cache/fmt.a\n"), 0o600))
	_, err = testedPackage(importcfg)
	require.ErrorContains(t, err, "names no test main package")
}

// TestFlagValueReadsTheLinkerFlags pins that a linker flag and its value
// arrive as two arguments, the way go passes them.
func TestFlagValueReadsTheLinkerFlags(t *testing.T) {
	args := []string{"-o", "/work/b001/pkg.test", "-importcfg", "/work/b001/importcfg.link", "-buildmode=exe", "/work/b001/_pkg_.a"}
	require.Equal(t, "/work/b001/pkg.test", flagValue(args, "-o"))
	require.Equal(t, "/work/b001/importcfg.link", flagValue(args, "-importcfg"))
	require.Empty(t, flagValue(args, "-buildmode"))
	require.Empty(t, flagValue([]string{"-o"}, "-o"))
}
