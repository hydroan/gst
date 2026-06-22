package ggmodule

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadModuleCopyMetadataMissingFile(t *testing.T) {
	notes, err := loadModuleCopyMetadata(t.TempDir())

	require.NoError(t, err)
	require.Empty(t, notes)
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

	notes, err := loadModuleCopyMetadata(moduleDir)

	require.NoError(t, err)
	require.Equal(t, []string{
		"Password-based MFA checks require servicemfa.SetUserAuthenticator(...).",
		"Create a project-owned adapter outside service/mfa.",
		"multi\nline",
	}, notes)
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

func writeModuleCopyMetadataForTest(t *testing.T, moduleDir, content string) {
	t.Helper()

	err := os.WriteFile(filepath.Join(moduleDir, moduleCopyMetadataFilename), []byte(content), 0o600)
	require.NoError(t, err)
}
