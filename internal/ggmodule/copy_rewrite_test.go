package ggmodule_test

import (
	"testing"

	"github.com/hydroan/gst/internal/ggmodule"
	"github.com/stretchr/testify/require"
)

// TestCopiedImportPathRewritesThePackagesOfTheCopiedTrees pins the example on
// CopiedImportPath: a package of the module's model or service tree, the tree
// root included, is imported from the project's copy of it, while a package
// outside the trees, a sibling module's among them, is imported from the
// framework.
func TestCopiedImportPathRewritesThePackagesOfTheCopiedTrees(t *testing.T) {
	cases := []struct {
		importPath string
		want       string
		ok         bool
	}{
		{"github.com/hydroan/gst/internal/service/iam/account", "tmpapp/service/iam/account", true},
		{"github.com/hydroan/gst/internal/service/iam", "tmpapp/service/iam", true},
		{"github.com/hydroan/gst/internal/model/iam/user", "tmpapp/model/iam/user", true},
		{"github.com/hydroan/gst/internal/service/iamx/account", "", false},
		{"github.com/hydroan/gst/authn", "", false},
	}
	for _, c := range cases {
		got, ok := ggmodule.CopiedImportPath(c.importPath, "iam", "tmpapp")
		require.Equal(t, c.ok, ok, c.importPath)
		require.Equal(t, c.want, got, c.importPath)
	}
}
