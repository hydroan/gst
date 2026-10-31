package logger

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/stretchr/testify/require"
)

// TestStandardLibraryLogsLandInTheAppStream pins that after Init the lines
// a project writes with slog and with the standard log package land in the
// app stream: a slog line carries the request metadata and the trace id its
// context holds, a group and the attributes under WithGroup fold into one
// string field, and a log line arrives as an info entry.
func TestStandardLibraryLogsLandInTheAppStream(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(Clean)
	withLoggerInitConfig(t, dir, "global.log")
	restoreGlobalLoggers(t)
	require.NoError(t, Init())

	meta := requestctx.New(requestctx.Fields{Route: "/api/samples/:id", Path: "/api/samples/1", Method: http.MethodGet, UserID: "1"})
	ctx := execctx.WithTraceID(requestctx.WithMetadata(context.Background(), meta), "trace-slog")
	slog.InfoContext(ctx, "sample event", "count", 3, slog.Group("detail", "kind", "sample"))
	slog.Default().WithGroup("batch").With("size", 2).Warn("grouped event", "n", 1)
	log.Printf("plain %s", "line")
	Clean()

	content, err := os.ReadFile(filepath.Join(dir, "app.log"))
	require.NoError(t, err)
	entries := make(map[string]map[string]any)
	for line := range strings.SplitSeq(strings.TrimSpace(string(content)), "\n") {
		entry := make(map[string]any)
		require.NoError(t, json.Unmarshal([]byte(line), &entry), line)
		msg, _ := entry["msg"].(string)
		entries[msg] = entry
	}

	sample := entries["sample event"]
	require.NotNil(t, sample, "the slog line is missing from %s", content)
	require.Equal(t, "INFO", sample["level"])
	require.Equal(t, "trace-slog", sample[execctx.KeyTraceID])
	require.Equal(t, "/api/samples/:id", sample[requestctx.KeyRoute])
	require.Equal(t, "1", sample[requestctx.KeyUserID])
	require.InDelta(t, 3, sample["count"], 0)
	detail, _ := sample["detail"].(string)
	require.Contains(t, detail, `"kind":"sample"`, "a group folds into one string field")
	require.Contains(t, sample["caller"], "slog_internal_test.go")

	grouped := entries["grouped event"]
	require.NotNil(t, grouped, "the grouped slog line is missing from %s", content)
	require.Equal(t, "WARN", grouped["level"])
	batch, _ := grouped["batch"].(string)
	require.Contains(t, batch, `"size":2`, "the attributes bound under WithGroup fold into the group's field")
	require.Contains(t, batch, `"n":1`, "the attributes of the record fold into the open group")

	plain := entries["plain line"]
	require.NotNil(t, plain, "the log line is missing from %s", content)
	require.Equal(t, "INFO", plain["level"])
}
