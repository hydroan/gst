package logger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/instance"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestInitInstallsFallbackForOptionalProviderLoggers(t *testing.T) {
	dir := t.TempDir()
	withLoggerInitConfig(t, dir, "global.log")
	restoreGlobalLoggers(t)

	require.NoError(t, Init())

	// Optional provider loggers stay usable but file-less until the provider
	// stage starts and the lifecycle registry binds dedicated loggers for the
	// providers actually compiled in; only that binding may create their log
	// files.
	optional := map[string]types.Logger{
		"cassandra": Cassandra,
		"elastic":   Elastic,
		"etcd":      Etcd,
		"influxdb":  Influxdb,
		"kafka":     Kafka,
		"ldap":      Ldap,
		"minio":     Minio,
		"mongo":     Mongo,
		"mqtt":      Mqtt,
		"nats":      Nats,
		"rethinkdb": RethinkDB,
		"rocketmq":  RocketMQ,
		"scylla":    Scylla,
	}
	for name, optionalLogger := range optional {
		require.NotNil(t, optionalLogger, "optional provider logger %s must fall back, not stay nil", name)
		require.NoFileExists(t, filepath.Join(dir, name+".log"),
			"optional provider %s must not own a log file before the provider stage starts", name)
	}

	// Core loggers keep their dedicated, precreated files. The distributed
	// cache is a framework capability like the in-process cache, not a
	// provider, so its logger belongs to this core group.
	require.FileExists(t, filepath.Join(dir, "service.log"))
	require.FileExists(t, filepath.Join(dir, "app.log"))
	require.FileExists(t, filepath.Join(dir, "dcache.log"))

	// A fallback entry routes to the global sink instead of vanishing.
	Kafka.Infow("fallback routed to the global sink")
	Clean()
	data, err := os.ReadFile(filepath.Join(dir, "global.log"))
	require.NoError(t, err)
	require.Contains(t, string(data), "fallback routed to the global sink")
}

func TestCleanFlushesBufferedFileSink(t *testing.T) {
	dir := t.TempDir()
	withLogWriterConfig(t, dir, "clean.log")

	log := New("clean.log")
	log.Infoz("flush through clean")
	Clean()

	data, err := os.ReadFile(filepath.Join(dir, "clean.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "flush through clean") {
		t.Fatalf("expected flushed log file to contain message, got %q", string(data))
	}
}

// TestCleanLeavesARemovedLogDirectoryRemoved pins what a test harness relies
// on when it removes its log directory after Clean: the stopped writers put
// nothing more on disk, so an entry logged afterwards cannot recreate the
// directory the way a writer opening its file on its first write does.
func TestCleanLeavesARemovedLogDirectoryRemoved(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	withLogWriterConfig(t, dir, "sample.log")

	log := New("sample.log")
	log.Infoz("logged before clean")
	Clean()
	require.NoError(t, os.RemoveAll(dir))

	log.Infoz("logged after clean")
	// A running writer would have written the entry out by now.
	time.Sleep(defaultLogFlushInterval + 200*time.Millisecond)
	require.NoDirExists(t, dir)
}

// TestEveryLoggerStampsTheProcessIdentity proves every entry, whichever
// constructor built its logger, carries the instance field naming this
// process, so the entries of replicas can be told apart wherever they are
// collected together.
func TestEveryLoggerStampsTheProcessIdentity(t *testing.T) {
	dir := t.TempDir()
	withLoggerInitConfig(t, dir, "global.log")
	restoreGlobalLoggers(t)
	require.NoError(t, Init())

	zap.S().Info("global")
	New("typed.log").Infoz("typed")
	NewGin("gin.log").Info("gin")
	NewZap("plain.log").Info("plain")
	NewSugared("sugared.log").Info("sugared")
	Clean()

	for _, file := range []string{"global.log", "typed.log", "gin.log", "plain.log", "sugared.log"} {
		data, err := os.ReadFile(filepath.Join(dir, file))
		require.NoError(t, err)
		var entry map[string]any
		require.NoError(t, json.Unmarshal(data, &entry), "%s: %q", file, data)
		require.Equal(t, instance.ID(), entry["instance"], "%s must carry the process identity", file)
		require.NotContains(t, entry, "logger", "%s: in file mode the file names the stream, the entry does not", file)
	}
}

// TestInitBuildsTheListenerLoggers pins the loggers Init builds for the two
// listeners, with their files created up front for a collector to tail: the
// HTTP access log and body log, the gRPC access log, and the recovery log
// both listeners' panics go to. The recovery log keeps the level and the
// message of an entry: a panic entry is its message, the request, the panic
// and the stack, which the access-log encoder would drop.
func TestInitBuildsTheListenerLoggers(t *testing.T) {
	dir := t.TempDir()
	withLoggerInitConfig(t, dir, "global.log")
	restoreGlobalLoggers(t)
	require.NoError(t, Init())

	require.NotNil(t, Gin)
	require.NotNil(t, HTTPBody)
	require.NotNil(t, GRPC)
	require.NotNil(t, Recovery)
	for _, file := range []string{"access.log", "http_body.log", "grpc.log", "recovery.log"} {
		require.FileExists(t, filepath.Join(dir, file))
	}

	Recovery.Error("[recovery] panic recovered: boom", zap.String(consts.TRACE_ID, "trace-panic"))
	Clean()

	data, err := os.ReadFile(filepath.Join(dir, "recovery.log"))
	require.NoError(t, err)
	var entry map[string]any
	require.NoError(t, json.Unmarshal(data, &entry), "%q", data)
	require.Equal(t, "ERROR", entry["level"])
	require.Equal(t, "[recovery] panic recovered: boom", entry["msg"])
	require.Equal(t, "trace-panic", entry[consts.TRACE_ID])
}

// TestInitRejectsAnUnknownOutput proves a mistyped output fails logger
// initialization instead of sending the logs somewhere nobody collects them.
func TestInitRejectsAnUnknownOutput(t *testing.T) {
	withLoggerInitConfig(t, t.TempDir(), "sample.log")
	config.App.Logger.Output = "files"
	restoreGlobalLoggers(t)

	require.ErrorContains(t, Init(), `"files"`)
}

// withLoggerInitConfig points config.App at a scratch file-mode logger setup
// for tests that run Init, which reads every logger setting from config.App.
func withLoggerInitConfig(t *testing.T, dir, file string) {
	t.Helper()

	original := config.App
	config.App = new(config.Config)
	config.App.Logger.Output = config.LoggerOutputFile
	config.App.Logger.Dir = dir
	config.App.Logger.File = file
	config.App.Logger.Level = "info"
	config.App.Logger.Format = "json"
	config.App.Logger.MaxAge = 30
	config.App.Logger.MaxSize = 100
	config.App.Logger.MaxBackups = 1

	t.Cleanup(func() {
		config.App = original
	})
}

// restoreGlobalLoggers snapshots every logger package global plus the zap
// global and restores them on cleanup, so a test that runs Init cannot leak
// loggers pointing at its scratch directory into later tests.
func restoreGlobalLoggers(t *testing.T) {
	t.Helper()

	savedTyped := map[*types.Logger]types.Logger{}
	for _, ref := range []*types.Logger{
		&App,
		&Controller, &Service, &Database,
		&Cache, &Dcache, &Redis,
		&Authz, &OTEL, &Cassandra, &Elastic,
		&Etcd, &Influxdb, &Kafka, &Ldap,
		&Minio, &Mongo, &Mqtt, &Nats,
		&RethinkDB, &RocketMQ, &Scylla,
	} {
		savedTyped[ref] = *ref
	}
	savedGin, savedHTTPBody, savedGorm := Gin, HTTPBody, Gorm
	savedZap := zap.L()

	t.Cleanup(func() {
		for ref, saved := range savedTyped {
			*ref = saved
		}
		Gin, HTTPBody, Gorm = savedGin, savedHTTPBody, savedGorm
		zap.ReplaceGlobals(savedZap)
	})
}
