package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCheckImportAlias runs the check over a fixture module whose
// .golangci.yml lists one alias, fxotel for example.com/module/otel. Two
// packages are named logger, example.com/module/logger and
// example.com/module/sink/logger, and the second is imported under an alias
// all over the module. Reported are the aliases no other import of their
// file calls for: in a package importing the one logger alone (spare), in a
// test file doing the same (sparetest), in a package itself named logger
// (selfref), whose own name shadows nothing, and beside an identifier named
// logger, which is renamed instead: a struct field (field), a package-level
// variable declared in another file of the package (ident) and a local
// variable (local). Reported once for the package is the second spelling the
// module gives it (drift's slogger beside sinklogger). Left alone are the
// alias beside the other logger (needed), the listed alias without any clash
// (listed), and a package name spelt out because it differs from the last
// element of its path (explicit's namedimpl).
func TestCheckImportAlias(t *testing.T) {
	root, pkgs := loadFixture(t, "testdata/importalias/module")
	violations, err := checkImportAlias(root, pkgs)
	require.NoError(t, err)
	spare := func(file string, line int) violation {
		return violation{
			File:    file,
			Message: fmt.Sprintf("Import 'example.com/module/sink/logger' at %s:%d is aliased sinklogger though no other import of the file is named logger: import it under its own name", file, line),
		}
	}
	require.Equal(t, []violation{
		{
			File:    "drift/drift.go",
			Message: "Package 'example.com/module/sink/logger' is aliased sinklogger (field/field.go:3) and slogger (drift/drift.go:5): use one alias for it",
		},
		spare("field/field.go", 3),
		spare("ident/use.go", 3),
		spare("local/local.go", 3),
		spare("selfref/logger/logger.go", 3),
		spare("spare/spare.go", 3),
		spare("sparetest/sparetest_test.go", 6),
	}, violations)
}

// TestCheckImportAliasReadsTheAliasesFromTheLintConfiguration pins where the
// listed aliases come from: the importas section of the .golangci.yml at the
// checked root, which must enable importas and require its aliases, so that
// golangci-lint holds the spelling of every listed alias while this check
// holds every other alias to a clash. A root without the file, one whose
// configuration does not enable importas, and one not requiring the aliases
// each stop the check.
func TestCheckImportAliasReadsTheAliasesFromTheLintConfiguration(t *testing.T) {
	_, pkgs := loadFixture(t, "testdata/importalias/module")
	for name, tt := range map[string]struct {
		config string
		want   string
	}{
		"without the file":        {want: lintConfigFile},
		"without importas":        {config: "version: \"2\"\nlinters:\n  enable:\n    - govet\n", want: "importas is not enabled"},
		"not requiring the alias": {config: "version: \"2\"\nlinters:\n  enable:\n    - importas\n  settings:\n    importas:\n      alias:\n        - pkg: example.com/module/otel\n          alias: fxotel\n", want: "no-unaliased"},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if tt.config != "" {
				require.NoError(t, os.WriteFile(filepath.Join(root, lintConfigFile), []byte(tt.config), 0o600))
			}
			_, err := checkImportAlias(root, pkgs)
			require.ErrorContains(t, err, tt.want)
		})
	}
}
