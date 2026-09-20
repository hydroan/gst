package database

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/stretchr/testify/require"
)

// TestLongTransactionIsReported proves the entry a transaction that held its
// connection too long leaves behind. A transaction holds a connection of the
// pool and every row it wrote for its whole length, and the length is the one
// thing its caller cannot see: whatever it spent it on — a statement, a lock,
// a call to another system — the deployment feels the same connection gone.
func TestLongTransactionIsReported(t *testing.T) {
	withLongTransaction(t, 50*time.Millisecond)
	from := databaseLogSize(t)

	require.NoError(t, Transaction(context.Background(), func(context.Context) error {
		time.Sleep(80 * time.Millisecond)
		return nil
	}))

	entry := databaseLogEntry(t, from, "transaction held its connection for a long time")
	require.NotNil(t, entry, "a transaction past the threshold must leave an entry")
	require.Equal(t, "transaction", entry["phase"], "the entry names the operation it reports on")
	require.NotEmpty(t, entry["held"], "the entry carries how long the connection was held")
}

// TestShortTransactionIsNotReported pins the other half: the ordinary
// transaction, which is nearly every one of them, writes nothing.
func TestShortTransactionIsNotReported(t *testing.T) {
	withLongTransaction(t, time.Hour)
	from := databaseLogSize(t)

	require.NoError(t, Transaction(context.Background(), func(context.Context) error { return nil }))

	require.Nil(t, databaseLogEntry(t, from, "transaction held its connection for a long time"),
		"a transaction within the threshold must leave none")
}

// withLongTransaction shrinks — or stretches — the threshold for one test.
func withLongTransaction(t *testing.T, d time.Duration) {
	t.Helper()

	original := longTransaction
	longTransaction = d
	t.Cleanup(func() { longTransaction = original })
}

// databaseLogSize flushes the database stream and returns how much of its
// file is written already, so a test reads only what it wrote itself: the
// file is the whole test binary's, and every test before this one is in it.
func databaseLogSize(t *testing.T) int64 {
	t.Helper()

	pkgzap.Clean()
	info, err := os.Stat(databaseLogPath())
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(t, err)
	return info.Size()
}

// databaseLogEntry flushes the stream and returns the last entry written
// past from whose msg field is msg, or nil when there is none.
func databaseLogEntry(t *testing.T, from int64, msg string) map[string]any {
	t.Helper()

	pkgzap.Clean()
	content, err := os.ReadFile(databaseLogPath())
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	if int64(len(content)) <= from {
		return nil
	}

	var found map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(string(content[from:])), "\n") {
		entry := map[string]any{}
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		if entry["msg"] == msg {
			found = entry
		}
	}
	return found
}

// databaseLogPath is the file the database stream writes to under the test
// setup's log directory.
func databaseLogPath() string {
	return filepath.Join(config.App.Logger.Dir, "database.log")
}
