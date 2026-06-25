package ggmodule

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadModuleManifestRequiresFile(t *testing.T) {
	_, err := loadModuleManifest(t.TempDir())

	require.Error(t, err)
	require.Contains(t, err.Error(), moduleManifestFilename)
}

func TestLoadModuleManifestReadsEmptyConfig(t *testing.T) {
	for name, content := range map[string]string{
		"empty root": `{}`,
		"empty copy": `{"copy":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			moduleDir := t.TempDir()
			writeModuleManifestForTest(t, moduleDir, content)

			manifest, err := loadModuleManifest(moduleDir)

			require.NoError(t, err)
			require.Empty(t, manifest.Copy.PostNotes)
			require.Empty(t, manifest.Copy.ExcludeSourceFiles)
			require.Empty(t, manifest.Copy.Middleware)
		})
	}
}

func TestLoadModuleManifestReadsPostNotes(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleManifestForTest(t, moduleDir, `{
		"copy": {
			"postNotes": [
				"Password-based MFA checks require servicemfa.SetAccountAuthenticator(...).",
				"   ",
				"Create a project-owned adapter outside service/mfa.",
				"multi\nline"
			],
			"unknownFutureField": true
		},
		"unknownFutureField": true
	}`)

	manifest, err := loadModuleManifest(moduleDir)

	require.NoError(t, err)
	require.Equal(t, []string{
		"Password-based MFA checks require servicemfa.SetAccountAuthenticator(...).",
		"Create a project-owned adapter outside service/mfa.",
		"multi\nline",
	}, manifest.Copy.PostNotes)
}

func TestLoadModuleManifestReadsExcludeSourceFiles(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleManifestForTest(t, moduleDir, `{
		"copy": {
			"excludeSourceFiles": [
				" internal/model/authz/button.go ",
				"",
				"internal/model/authz/../authz/menu.go"
			]
		}
	}`)

	manifest, err := loadModuleManifest(moduleDir)

	require.NoError(t, err)
	require.Equal(t, []string{
		"internal/model/authz/button.go",
		"internal/model/authz/menu.go",
	}, manifest.Copy.ExcludeSourceFiles)
}

func TestLoadModuleManifestReadsMiddleware(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleManifestForTest(t, moduleDir, `{
		"copy": {
			"middleware": [
				{
					"sourceFile": " middleware/authz.go ",
					"scope": " auth ",
					"handler": " Authz "
				}
			]
		}
	}`)

	manifest, err := loadModuleManifest(moduleDir)

	require.NoError(t, err)
	require.Equal(t, []moduleCopyMiddlewareManifest{
		{
			SourceFile: "middleware/authz.go",
			Scope:      moduleCopyMiddlewareScopeAuth,
			Handler:    "Authz",
		},
	}, manifest.Copy.Middleware)
}

func TestLoadModuleManifestRejectsInvalidJSON(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleManifestForTest(t, moduleDir, `{`)

	_, err := loadModuleManifest(moduleDir)

	require.Error(t, err)
	require.Contains(t, err.Error(), moduleManifestFilename)
}

func TestLoadModuleManifestRejectsNonStringArrayPostNotes(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleManifestForTest(t, moduleDir, `{"copy":{"postNotes":"configure authenticator"}}`)

	_, err := loadModuleManifest(moduleDir)

	require.Error(t, err)
	require.Contains(t, err.Error(), moduleManifestFilename)
}

func TestLoadModuleManifestRejectsUnsafeExcludeSourceFiles(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleManifestForTest(t, moduleDir, `{"copy":{"excludeSourceFiles":["../internal/model/authz/button.go"]}}`)

	_, err := loadModuleManifest(moduleDir)

	require.Error(t, err)
	require.Contains(t, err.Error(), moduleManifestFilename)
	require.Contains(t, err.Error(), "excludeSourceFiles")
}

func TestLoadModuleManifestRejectsUnsafeMiddlewareSourceFile(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleManifestForTest(t, moduleDir, `{
		"copy": {
			"middleware": [
				{"sourceFile": "../middleware/authz.go", "scope": "auth", "handler": "Authz"}
			]
		}
	}`)

	_, err := loadModuleManifest(moduleDir)

	require.Error(t, err)
	require.Contains(t, err.Error(), moduleManifestFilename)
	require.Contains(t, err.Error(), "middleware")
}

func TestLoadModuleManifestRejectsInvalidMiddlewareHandler(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleManifestForTest(t, moduleDir, `{
		"copy": {
			"middleware": [
				{"sourceFile": "middleware/authz.go", "scope": "auth", "handler": "Authz()"}
			]
		}
	}`)

	_, err := loadModuleManifest(moduleDir)

	require.Error(t, err)
	require.Contains(t, err.Error(), moduleManifestFilename)
	require.Contains(t, err.Error(), "handler")
}

func TestLoadModuleManifestRejectsInvalidMiddlewareScope(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleManifestForTest(t, moduleDir, `{
		"copy": {
			"middleware": [
				{"sourceFile": "middleware/authz.go", "scope": "admin", "handler": "Authz"}
			]
		}
	}`)

	_, err := loadModuleManifest(moduleDir)

	require.Error(t, err)
	require.Contains(t, err.Error(), moduleManifestFilename)
	require.Contains(t, err.Error(), "scope")
}

func writeModuleManifestForTest(t *testing.T, moduleDir, content string) {
	t.Helper()

	err := os.WriteFile(filepath.Join(moduleDir, moduleManifestFilename), []byte(content), 0o600)
	require.NoError(t, err)
}
