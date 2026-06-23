package ggmodule

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadModuleCopyMetadataMissingFile(t *testing.T) {
	metadata, err := loadModuleCopyMetadata(t.TempDir())

	require.NoError(t, err)
	require.Empty(t, metadata.PostCopyNotes)
	require.Empty(t, metadata.IgnoreFiles)
}

func TestLoadModuleCopyMetadataReadsPostCopyNotes(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleCopyMetadataForTest(t, moduleDir, `{
		"postCopyNotes": [
			"Password-based MFA checks require servicemfa.SetUserAuthenticator(...).",
			"   ",
			"Create a project-owned adapter outside service/mfa.",
			"multi\nline"
		],
		"unknownFutureField": true
	}`)

	metadata, err := loadModuleCopyMetadata(moduleDir)

	require.NoError(t, err)
	require.Equal(t, []string{
		"Password-based MFA checks require servicemfa.SetUserAuthenticator(...).",
		"Create a project-owned adapter outside service/mfa.",
		"multi\nline",
	}, metadata.PostCopyNotes)
}

func TestLoadModuleCopyMetadataReadsIgnoreFiles(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleCopyMetadataForTest(t, moduleDir, `{
		"ignoreFiles": [
			" internal/model/authz/button.go ",
			"",
			"internal/model/authz/../authz/menu.go"
		]
	}`)

	metadata, err := loadModuleCopyMetadata(moduleDir)

	require.NoError(t, err)
	require.Equal(t, []string{
		"internal/model/authz/button.go",
		"internal/model/authz/menu.go",
	}, metadata.IgnoreFiles)
}

func TestLoadModuleCopyMetadataReadsMiddleware(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleCopyMetadataForTest(t, moduleDir, `{
		"middleware": [
			{
				"source": " middleware/authz.go ",
				"auth": true,
				"function": " Authz "
			}
		]
	}`)

	metadata, err := loadModuleCopyMetadata(moduleDir)

	require.NoError(t, err)
	require.Equal(t, []moduleCopyMiddlewareMetadata{
		{
			Source:   "middleware/authz.go",
			Auth:     true,
			Function: "Authz",
		},
	}, metadata.Middleware)
}

func TestLoadModuleCopyMetadataRejectsInvalidJSON(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleCopyMetadataForTest(t, moduleDir, `{`)

	_, err := loadModuleCopyMetadata(moduleDir)

	require.Error(t, err)
	require.Contains(t, err.Error(), moduleCopyMetadataFilename)
}

func TestLoadModuleCopyMetadataRejectsNonStringArrayNotes(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleCopyMetadataForTest(t, moduleDir, `{"postCopyNotes":"configure authenticator"}`)

	_, err := loadModuleCopyMetadata(moduleDir)

	require.Error(t, err)
	require.Contains(t, err.Error(), moduleCopyMetadataFilename)
}

func TestLoadModuleCopyMetadataRejectsUnsafeIgnoreFiles(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleCopyMetadataForTest(t, moduleDir, `{"ignoreFiles":["../internal/model/authz/button.go"]}`)

	_, err := loadModuleCopyMetadata(moduleDir)

	require.Error(t, err)
	require.Contains(t, err.Error(), moduleCopyMetadataFilename)
	require.Contains(t, err.Error(), "ignoreFiles")
}

func TestLoadModuleCopyMetadataRejectsUnsafeMiddlewareSource(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleCopyMetadataForTest(t, moduleDir, `{
		"middleware": [
			{"source": "../middleware/authz.go", "function": "Authz"}
		]
	}`)

	_, err := loadModuleCopyMetadata(moduleDir)

	require.Error(t, err)
	require.Contains(t, err.Error(), moduleCopyMetadataFilename)
	require.Contains(t, err.Error(), "middleware")
}

func TestLoadModuleCopyMetadataRejectsInvalidMiddlewareFunction(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleCopyMetadataForTest(t, moduleDir, `{
		"middleware": [
			{"source": "middleware/authz.go", "function": "Authz()"}
		]
	}`)

	_, err := loadModuleCopyMetadata(moduleDir)

	require.Error(t, err)
	require.Contains(t, err.Error(), moduleCopyMetadataFilename)
	require.Contains(t, err.Error(), "function")
}

func writeModuleCopyMetadataForTest(t *testing.T, moduleDir, content string) {
	t.Helper()

	err := os.WriteFile(filepath.Join(moduleDir, moduleCopyMetadataFilename), []byte(content), 0o600)
	require.NoError(t, err)
}
