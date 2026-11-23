package main

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/stretchr/testify/require"
)

// TestLinesAreShapedTheWayGGShapesItsOwn pins that the lines of this
// program are the ones gg prints through internal/clioutput, which a
// program built inside a project cannot import: the same symbols and
// layout, on the same stream, and on a terminal taking colors the same
// colors.
func TestLinesAreShapedTheWayGGShapesItsOwn(t *testing.T) {
	noColor := color.NoColor
	t.Cleanup(func() { color.NoColor, migrateColor = noColor, false })

	for _, colored := range []bool{false, true} {
		color.NoColor, migrateColor = !colored, colored
		for _, line := range []struct {
			name      string
			stream    **os.File
			program   func()
			clioutput func()
		}{
			{name: "section", stream: &os.Stdout, program: func() { section("Plan") }, clioutput: func() { clioutput.Section("Plan") }},
			{name: "item", stream: &os.Stdout, program: func() { item("Database: %s", "sqlite") }, clioutput: func() { clioutput.Item("Database: %s", "sqlite") }},
			{name: "success", stream: &os.Stdout, program: func() { success("Renamed") }, clioutput: func() { clioutput.Success("Renamed") }},
			{name: "done", stream: &os.Stdout, program: func() { done("Applied %s", "3 statements") }, clioutput: func() { clioutput.Done("Applied %s", "3 statements") }},
			{name: "note", stream: &os.Stdout, program: func() { note("To migrate a database file, set %s", "sqlite.path") }, clioutput: func() { clioutput.Note("To migrate a database file, set %s", "sqlite.path") }},
			{name: "progress", stream: &os.Stderr, program: func() { progress("Printing the %s schema", "sqlite") }, clioutput: func() { clioutput.Progress("Printing the %s schema", "sqlite") }},
		} {
			want := capture(t, line.stream, line.clioutput)
			require.NotEmpty(t, want, "%s, colored %t", line.name, colored)
			require.Equal(t, want, capture(t, line.stream, line.program), "%s, colored %t", line.name, colored)
		}
	}
}

// TestPlanSummary pins the examples planSummary gives: the statements
// counted by the words they open with, however they are spaced and cased,
// and one opening with none of them counted last.
func TestPlanSummary(t *testing.T) {
	var created []string
	for range 13 {
		created = append(created, "CREATE TABLE `samples` (\n  `id` text\n)")
	}
	for range 4 {
		created = append(created, "CREATE UNIQUE INDEX `uniq_samples_code` ON `samples` (`code`)")
	}
	for range 5 {
		created = append(created, "create index `idx_samples_kind` on `samples` (`kind`)")
	}
	require.Equal(t, "22 statements: 13 CREATE TABLE, 4 CREATE UNIQUE INDEX, 5 CREATE INDEX", planSummary(created))

	require.Equal(t, "3 statements: 2 ALTER TABLE, 1 other", planSummary([]string{
		"ALTER TABLE `samples` ADD COLUMN `kind` text",
		"PRAGMA foreign_keys = off",
		"ALTER  TABLE\n`samples` DROP INDEX `uniq_samples_code`",
	}))
	require.Equal(t, "1 statement: 1 DROP INDEX", planSummary([]string{"DROP INDEX `idx_samples_kind`"}))
}

// TestElapsedIsWrittenTheWayGGWritesIt pins that the time the statements
// took is written the way gg writes the time of its own commands.
func TestElapsedIsWrittenTheWayGGWritesIt(t *testing.T) {
	for _, d := range []time.Duration{0, 240*time.Millisecond + 400*time.Microsecond, 999600 * time.Microsecond, 1280 * time.Millisecond, 59 * time.Second, 72*time.Second + 400*time.Millisecond} {
		require.Equal(t, clioutput.Elapsed(d), elapsed(d), d.String())
	}
}

// capture returns what fn writes to stream, standard output or standard
// error.
func capture(t *testing.T, stream **os.File, fn func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	saved := *stream
	*stream = writer
	defer func() { *stream = saved }()
	fn()
	require.NoError(t, writer.Close())
	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	return string(output)
}
