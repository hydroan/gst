package testlog_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/testutil/testlog"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestToTempDir brings the loggers up after ToTempDir the two ways a test
// process does, and follows an entry of the global stream into the scratch
// directory, which the release then removes.
func TestToTempDir(t *testing.T) {
	for _, tt := range []struct {
		name       string
		configInit bool
	}{
		{name: "through config.Init", configInit: true},
		{name: "without config.Init", configInit: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// t.Setenv restores each variable ToTempDir exports once the test
			// ends.
			for _, key := range []string{config.LOGGER_OUTPUT, config.LOGGER_DIR, config.LOGGER_FILE, config.LOGGER_CONSOLE} {
				t.Setenv(key, "")
			}
			original, originalLogger := config.App, config.App.Logger
			t.Cleanup(func() {
				config.App = original
				config.App.Logger = originalLogger
			})

			dir, release, err := testlog.ToTempDir()
			require.NoError(t, err)
			if tt.configInit {
				require.NoError(t, config.Init())
			}
			require.NoError(t, pkgzap.Init())
			zap.S().Info("sample entry")
			require.NoError(t, zap.L().Sync())

			data, err := os.ReadFile(filepath.Join(dir, "global.log"))
			require.NoError(t, err)
			require.Contains(t, string(data), "sample entry")

			require.NoError(t, release())
			require.NoDirExists(t, dir)
		})
	}
}
