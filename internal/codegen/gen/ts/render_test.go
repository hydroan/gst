package ts

import (
	"strings"
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

func TestPropertyNameQuotesWhatIsNotAnIdentifier(t *testing.T) {
	tests := map[string]string{
		"title":      "title",
		"$ref":       "$ref",
		"trace_id":   "trace_id",
		"-":          `"-"`,
		"created at": `"created at"`,
		"2fa":        `"2fa"`,
	}
	for key, want := range tests {
		require.Equal(t, want, propertyName(key), "key %q", key)
	}
}

func TestQuoteStringLeavesTextReadable(t *testing.T) {
	// encoding/json escapes <, > and & by default, which would turn a readable
	// literal into escape sequences.
	require.Equal(t, `"a<b&c>d"`, quoteString("a<b&c>d"))
	require.Equal(t, `"quote\"inside"`, quoteString(`quote"inside`))
	require.Equal(t, `"line\nbreak"`, quoteString("line\nbreak"))
}

func TestArrayOfParenthesizesAUnion(t *testing.T) {
	require.Equal(t, "string[]", arrayOf("string"))
	require.Equal(t, "Record[]", arrayOf("Record"))
	require.Equal(t, `(Status | "")[]`, arrayOf(`Status | ""`))
	require.Equal(t, "(Record | null)[]", arrayOf("Record | null"))
}

func TestWriteDocEscapesWhatWouldEndOrRetagTheComment(t *testing.T) {
	var b strings.Builder
	writeDoc(&b, "", "ends with */ inside")
	require.Equal(t, "/** ends with *\\/ inside */\n", b.String())

	b.Reset()
	writeDoc(&b, "  ", "Title is the display title.\n\n@internal must not read as a tag")
	require.Equal(t, "  /**\n   * Title is the display title.\n   *\n   * \\@internal must not read as a tag\n   */\n", b.String())

	b.Reset()
	writeDoc(&b, "", "")
	require.Empty(t, b.String())
}

func TestGenerateReportsRootsItCannotFind(t *testing.T) {
	_, err := generateFixture(
		t,
		TypeRef{PkgPath: fixtureModule + "/model/sample", Name: "Missing"},
		TypeRef{PkgPath: "example.com/elsewhere", Name: "Sample"},
	)
	var diagnostics *DiagnosticsError
	require.ErrorAs(t, err, &diagnostics)

	require.Contains(t, diagnostics.Error(), "the package declares no type Missing")
	require.Contains(t, diagnostics.Error(), "the package is not part of module "+fixtureModule)
}
