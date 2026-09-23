package config

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSetBuildInfoTakesTheModuleVersion pins where a build gets its version:
// the module version Go records in the binary, a tag or a pseudo-version, of
// which only a release version also names the tag.
func TestSetBuildInfoTakesTheModuleVersion(t *testing.T) {
	tests := []struct {
		name    string
		module  string // the main module version Go recorded
		version string
		tag     string
	}{
		{name: "release version", module: "v1.2.3", version: "v1.2.3", tag: "v1.2.3"},
		{name: "release version with local changes", module: "v1.2.3+dirty", version: "v1.2.3+dirty", tag: "v1.2.3"},
		{name: "pseudo-version", module: "v0.0.0-20260923111514-7e3248505af4", version: "v0.0.0-20260923111514-7e3248505af4"},
		{name: "pseudo-version with local changes", module: "v0.0.0-20260923111514-7e3248505af4+dirty", version: "v0.0.0-20260923111514-7e3248505af4+dirty"},
		{name: "build outside version control", module: "(devel)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &AppInfo{}
			a.setBuildInfo(&debug.BuildInfo{Main: debug.Module{Version: tt.module}})

			require.Equal(t, tt.version, a.Version)
			require.Equal(t, tt.tag, a.GitTag)
		})
	}
}
