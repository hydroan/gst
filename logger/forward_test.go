package logger_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	gstsqlite "github.com/hydroan/gst/database/sqlite"
	"github.com/hydroan/gst/internal/gst"
	internallogger "github.com/hydroan/gst/internal/logger"
	"github.com/hydroan/gst/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestTheStreamFunctionsReturnTheFrameworksStreams pins that the public
// package holds no stream of its own: each function returns the stream the
// framework itself logs to, as it stands at the call, so a stream the
// framework replaces, a provider's once its stage starts, is the one a
// project gets from then on.
func TestTheStreamFunctionsReturnTheFrameworksStreams(t *testing.T) {
	streamsInto(t)

	for name, stream := range map[string]struct {
		forwarded func() gst.Logger
		held      *gst.Logger
	}{
		"App":       {logger.App, &internallogger.App},
		"Cassandra": {logger.Cassandra, &internallogger.Cassandra},
		"Elastic":   {logger.Elastic, &internallogger.Elastic},
		"Etcd":      {logger.Etcd, &internallogger.Etcd},
		"Influxdb":  {logger.Influxdb, &internallogger.Influxdb},
		"Kafka":     {logger.Kafka, &internallogger.Kafka},
		"Ldap":      {logger.Ldap, &internallogger.Ldap},
		"Minio":     {logger.Minio, &internallogger.Minio},
		"Mongo":     {logger.Mongo, &internallogger.Mongo},
		"Mqtt":      {logger.Mqtt, &internallogger.Mqtt},
		"Nats":      {logger.Nats, &internallogger.Nats},
		"RethinkDB": {logger.RethinkDB, &internallogger.RethinkDB},
		"RocketMQ":  {logger.RocketMQ, &internallogger.RocketMQ},
		"Scylla":    {logger.Scylla, &internallogger.Scylla},
	} {
		opened := *stream.held
		require.NotNil(t, opened, name)
		require.Same(t, opened, stream.forwarded(), "%s returns the stream Init opened", name)

		replacement := logger.Fallback("replacement")
		*stream.held = replacement
		require.Same(t, replacement, stream.forwarded(), "%s follows the stream the framework replaced", name)
		*stream.held = opened
	}
}

// TestAnEntryNamesTheLineThatLoggedIt pins what forwarding must not disturb.
// A stream function returns the logger itself, with no function of this
// package between a project's code and it, so the caller an entry records is
// the line that logged it, and the stack recorded with an error starts where
// the error was made.
func TestAnEntryNamesTheLineThatLoggedIt(t *testing.T) {
	dir := streamsInto(t)

	_, _, line, ok := runtime.Caller(0)
	require.True(t, ok)
	logger.App().Infow("sample entry")
	failure := errors.New("sample failure")
	logger.App().Errorz("sample failure entry", zap.Error(failure))
	logger.Clean()

	entries := readEntries(t, filepath.Join(dir, "app.log"))
	require.Len(t, entries, 2)
	require.Equal(t, "logger/forward_test.go:"+strconv.Itoa(line+2), entries[0]["caller"])
	require.Equal(t, "logger/forward_test.go:"+strconv.Itoa(line+4), entries[1]["caller"])
	stack, _ := entries[1]["error_stack"].(string)
	origin, _, _ := strings.Cut(stack, "\n")
	require.Contains(t, origin, "TestAnEntryNamesTheLineThatLoggedIt", "the stack starts in the function that made the error: %q", stack)
	require.Contains(t, stack, "forward_test.go:"+strconv.Itoa(line+3), "the stack names the line that made the error")
}

// TestASQLEntryNamesTheLineThatIssuedTheStatement pins the caller of the SQL
// log end to end: the frames of gorm and of the framework's own logging are
// skipped, wherever the logging package lives, and the entry names the line
// that issued the statement.
func TestASQLEntryNamesTheLineThatIssuedTheStatement(t *testing.T) {
	dir := streamsInto(t)
	db, err := gstsqlite.New(config.Sqlite{IsMemory: true})
	require.NoError(t, err)

	_, _, line, ok := runtime.Caller(0)
	require.True(t, ok)
	require.Error(t, db.Exec("SELECT * FROM no_such_table").Error)
	logger.Clean()

	entries := readEntries(t, filepath.Join(dir, "gorm.log"))
	require.NotEmpty(t, entries)
	require.Equal(t, "logger/forward_test.go:"+strconv.Itoa(line+2), entries[len(entries)-1]["caller"])
}

// streamsInto opens the streams as files under a directory of the test's own
// and returns the directory; the streams are flushed and the configuration
// restored when the test ends.
func streamsInto(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	original := config.App
	config.App = new(config.Config)
	config.App.Logger.Output = config.LoggerOutputFile
	config.App.Logger.Dir = dir
	config.App.Logger.Level = "info"
	config.App.Logger.Format = "json"
	config.App.Logger.MaxAge = 30
	config.App.Logger.MaxSize = 100
	config.App.Logger.MaxBackups = 1
	t.Cleanup(func() {
		logger.Clean()
		config.App = original
	})
	require.NoError(t, logger.Init())
	return dir
}

// readEntries returns the entries of the log file at path, one JSON object a
// line.
func readEntries(t *testing.T, path string) []map[string]any {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var entries []map[string]any
	for line := range bytes.Lines(data) {
		var entry map[string]any
		require.NoError(t, json.Unmarshal(line, &entry), "%q", line)
		entries = append(entries, entry)
	}
	return entries
}
