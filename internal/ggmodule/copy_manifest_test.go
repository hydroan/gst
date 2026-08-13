package ggmodule

import (
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
				"Copytest checks require servicecopytest.SetAdapter(...).",
				"   ",
				"Create a project-owned adapter outside service/copytest.",
				"multi\nline"
			],
			"unknownFutureField": true
		},
		"unknownFutureField": true
	}`)

	manifest, err := loadModuleManifest(moduleDir)

	require.NoError(t, err)
	require.Equal(t, []string{
		"Copytest checks require servicecopytest.SetAdapter(...).",
		"Create a project-owned adapter outside service/copytest.",
		"multi\nline",
	}, manifest.Copy.PostNotes)
}

func TestLoadModuleManifestReadsExcludeSourceFiles(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleManifestForTest(t, moduleDir, `{
		"copy": {
			"excludeSourceFiles": [
				" internal/model/copytest/ignored.go ",
				"",
				"internal/model/copytest/../copytest/helper.go"
			]
		}
	}`)

	manifest, err := loadModuleManifest(moduleDir)

	require.NoError(t, err)
	require.Equal(t, []string{
		"internal/model/copytest/ignored.go",
		"internal/model/copytest/helper.go",
	}, manifest.Copy.ExcludeSourceFiles)
}

func TestLoadModuleManifestReadsIncludeSourceFiles(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleManifestForTest(t, moduleDir, `{
		"copy": {
			"includeSourceFiles": [
				" internal/service/copytest/standalone.go ",
				"",
				"internal/service/copytest/../copytest/orphan.go"
			]
		}
	}`)

	manifest, err := loadModuleManifest(moduleDir)

	require.NoError(t, err)
	require.Equal(t, []string{
		"internal/service/copytest/standalone.go",
		"internal/service/copytest/orphan.go",
	}, manifest.Copy.IncludeSourceFiles)
}

func TestLoadModuleManifestRejectsUnsafeIncludeSourceFiles(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleManifestForTest(t, moduleDir, `{"copy":{"includeSourceFiles":["../internal/service/copytest/standalone.go"]}}`)

	_, err := loadModuleManifest(moduleDir)

	require.Error(t, err)
	require.Contains(t, err.Error(), moduleManifestFilename)
	require.Contains(t, err.Error(), "includeSourceFiles")
}

func TestLoadModuleManifestReadsMiddleware(t *testing.T) {
	moduleDir := t.TempDir()
	writeModuleManifestForTest(t, moduleDir, `{
		"copy": {
			"middleware": [
				{
					"sourceFile": " middleware/copy_auth.go ",
					"scope": " auth ",
					"handler": " CopyAuth "
				}
			]
		}
	}`)

	manifest, err := loadModuleManifest(moduleDir)

	require.NoError(t, err)
	require.Equal(t, []moduleCopyMiddlewareManifest{
		{
			SourceFile: "middleware/copy_auth.go",
			Scope:      moduleCopyMiddlewareScopeAuth,
			Handler:    "CopyAuth",
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
	writeModuleManifestForTest(t, moduleDir, `{"copy":{"excludeSourceFiles":["../internal/model/copytest/ignored.go"]}}`)

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
				{"sourceFile": "../middleware/copy_auth.go", "scope": "auth", "handler": "CopyAuth"}
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
				{"sourceFile": "middleware/copy_auth.go", "scope": "auth", "handler": "CopyAuth()"}
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
				{"sourceFile": "middleware/copy_auth.go", "scope": "admin", "handler": "CopyAuth"}
			]
		}
	}`)

	_, err := loadModuleManifest(moduleDir)

	require.Error(t, err)
	require.Contains(t, err.Error(), moduleManifestFilename)
	require.Contains(t, err.Error(), "scope")
}
