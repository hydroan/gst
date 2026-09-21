package tunnel

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

// TestLoggersAreSelfContained proves the tunnel builds its own loggers on
// first use: the framework's logging setup knows nothing about protocol.log
// and binary.log, so only a process that actually runs a tunnel session gets
// the files.
func TestLoggersAreSelfContained(t *testing.T) {
	dir := t.TempDir()
	original := config.App
	config.App = new(config.Config)
	config.App.Logger.Dir = dir
	loggerOnce = sync.Once{}
	t.Cleanup(func() {
		config.App = original
		// Leave the lazy state untouched for other tests in this binary:
		// whoever uses the loggers next rebuilds them under its own config.
		loggerOnce = sync.Once{}
		protocolLogger = nil
		binaryLogger = nil
	})

	require.NotNil(t, protocolLog())
	require.NotNil(t, binaryLog())
	require.FileExists(t, filepath.Join(dir, "protocol.log"))
	require.FileExists(t, filepath.Join(dir, "binary.log"))
}
