package ts

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOutputPathsAreRootedAtTheModelTree(t *testing.T) {
	g := &generator{cfg: Config{ModulePath: "example.com/app", RootPath: "example.com/app/model"}}
	tests := map[string]struct{ file, alias string }{
		"example.com/app/model/sample":      {"sample.ts", "$sample"},
		"example.com/app/model/iam/account": {"iam/account.ts", "$iam$account"},
		"example.com/app/model":             {"model.ts", "$model"},
		"example.com/app/pkg/notifier":      {"pkg/notifier.ts", "$pkg$notifier"},
		"example.com/app/pkg/http-client":   {"pkg/http-client.ts", "$pkg$http_client"},
		"example.com/app":                   {"index.ts", "$index"},
	}
	for pkgPath, want := range tests {
		require.Equal(t, want.file, g.filePath(pkgPath), pkgPath)
		require.Equal(t, want.alias, g.importAlias(pkgPath), pkgPath)
	}
}

func TestRelativeImportClimbsOutOfTheImportingDirectory(t *testing.T) {
	tests := []struct{ from, to, want string }{
		{"sample.ts", "record.ts", "./record.js"},
		{"sample.ts", "pkg/notifier.ts", "./pkg/notifier.js"},
		{"iam/account.ts", "record.ts", "../record.js"},
		{"iam/account.ts", "iam/session.ts", "./session.js"},
		{"iam/admin/user.ts", "pkg/notifier.ts", "../../pkg/notifier.js"},
		{"pkg/notifier.ts", "model.ts", "../model.js"},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, relativeImport(tt.from, tt.to), "%s imports %s", tt.from, tt.to)
	}
}

func TestPreludeFileNameFallsBackToTheFrameworkName(t *testing.T) {
	tests := map[string]string{
		"":                 "gst.ts",
		"   ":              "gst.ts",
		"../":              "gst.ts",
		"shop":             "shop.ts",
		"sample-api":       "sample-api.ts",
		"Sample Shop / v2": "Sample_Shop___v2.ts",
	}
	for appName, want := range tests {
		require.Equal(t, want, preludeFileName(appName), "application name %q", appName)
	}
}
