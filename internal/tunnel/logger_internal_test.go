package tunnel

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/logfield"
	"github.com/hydroan/gst/logger"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
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

// TestEachFrameIsLoggedOnce pins a frame's binary.log entry: the size and the
// bytes of the frame, base64 encoded, once.
func TestEachFrameIsLoggedOnce(t *testing.T) {
	dir := t.TempDir()
	// The file writers hold entries back; Clean flushes them before the
	// entry is read and stops them before the directory goes.
	t.Cleanup(logger.Clean)
	original := config.App
	config.App = new(config.Config)
	config.App.Logger.Dir = dir
	config.App.Logger.Format = "json"
	loggerOnce = sync.Once{}
	t.Cleanup(func() {
		config.App = original
		loggerOnce = sync.Once{}
		protocolLogger = nil
		binaryLogger = nil
	})

	local, remote := net.Pipe()
	t.Cleanup(func() {
		_ = local.Close()
		_ = remote.Close()
	})
	go func() { _, _ = io.Copy(io.Discard, remote) }()
	session := &Session{tcpconn: local, locker: new(connMapVal)}
	event := &Event{ID: "sample-id", Cmd: Ping}
	require.NoError(t, session.Write(event))
	frame, err := msgpack.Marshal(event)
	require.NoError(t, err)
	logger.Clean()

	data, err := os.ReadFile(filepath.Join(dir, "binary.log"))
	require.NoError(t, err)
	entry := make(map[string]any)
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(data), &entry))
	require.Equal(t, "Write", entry["msg"])
	fields := make([]string, 0, len(entry))
	for key := range entry {
		switch key {
		case "level", "ts", "caller", "msg", logfield.KeyInstance:
		default:
			fields = append(fields, key)
		}
	}
	require.ElementsMatch(t, []string{"size", "binary"}, fields)
	require.InDelta(t, len(frame), entry["size"], 0)
	require.Equal(t, base64.StdEncoding.EncodeToString(frame), entry["binary"])
}
