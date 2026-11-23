package clioutput_test

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/stretchr/testify/require"
)

// TestLinesGoToTheStreamTheyBelongTo pins every line shape of the package
// doc and the stream it is written to: what a command reports on standard
// output, what it says to the person running it on standard error.
func TestLinesGoToTheStreamTheyBelongTo(t *testing.T) {
	tests := []struct {
		name    string
		print   func()
		wantOut string
		wantErr string
	}{
		{name: "a section title", print: func() { clioutput.Section("Plan") }, wantOut: "\n▶ Plan\n"},
		{name: "an entry that went well", print: func() { clioutput.Success("%d checks passed", 27) }, wantOut: "  ✔ 27 checks passed\n"},
		{name: "a plain entry", print: func() { clioutput.Item("Database: %s", "sqlite") }, wantOut: "  → Database: sqlite\n"},
		{name: "an entry under a symbol of the caller's", print: func() {
			clioutput.Status(clioutput.StyleError, clioutput.SymbolError, "%s (%d)", "Column types", 2)
		}, wantOut: "  ✘ Column types (2)\n"},
		{name: "a line under an entry", print: func() { clioutput.Detail("model/sample.go:%d: no column", 9) }, wantOut: "    model/sample.go:9: no column\n"},
		{name: "a file written", print: func() { clioutput.Change(clioutput.LabelCreate, "%s", "service/sample/create.go") }, wantOut: "  CREATE  service/sample/create.go\n"},
		{name: "a label shorter than the widest", print: func() { clioutput.Change(clioutput.LabelFix, "model/sample.go: numbered %s", "Sample.Name") }, wantOut: "  FIX     model/sample.go: numbered Sample.Name\n"},
		{name: "a command to run next", print: func() { clioutput.Command("cd %s", "sample") }, wantOut: "  $ cd sample\n"},
		{name: "the line a command that succeeded ends on", print: func() { clioutput.Done("Generated %d files in %s", 3, "1.6s") }, wantOut: "\n✔ Generated 3 files in 1.6s\n"},
		{name: "what to do next", print: func() { clioutput.Note("Next: fill in %s", "service/sample/create.go") }, wantOut: "  Next: fill in service/sample/create.go\n"},
		{name: "a step taking its time", print: func() { clioutput.Progress("Building the %s program", "migration") }, wantErr: "  → Building the migration program\n"},
		{name: "a warning", print: func() { clioutput.Warn("%s names no file", "gst.yaml") }, wantErr: "  ⚠ gst.yaml names no file\n"},
		{name: "an error the command goes on from", print: func() { clioutput.Error("Failed to delete %s", "service/sample") }, wantErr: "  ✘ Failed to delete service/sample\n"},
		{name: "the line a command that failed ends on", print: func() { clioutput.Fail("%d violations in 1 of 35 checks", 12) }, wantErr: "\n✘ 12 violations in 1 of 35 checks\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := captureStreams(t, tt.print)
			require.Equal(t, tt.wantOut, out)
			require.Equal(t, tt.wantErr, err)
		})
	}
}

// TestConfirmReadsAYesOrANo pins the question Confirm asks, on standard
// error with the line ended after an answer that came through a pipe, and
// the answers it takes for a yes: y and yes in either case, nothing else.
func TestConfirmReadsAYesOrANo(t *testing.T) {
	tests := []struct {
		answer string
		want   bool
	}{
		{answer: "y\n", want: true},
		{answer: "YES\n", want: true},
		{answer: " yes \n", want: true},
		{answer: "n\n", want: false},
		{answer: "\n", want: false},
		{answer: "yep\n", want: false},
		{answer: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.answer, func(t *testing.T) {
			reader, writer, err := os.Pipe()
			require.NoError(t, err)
			_, err = writer.WriteString(tt.answer)
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			previous := os.Stdin
			os.Stdin = reader
			t.Cleanup(func() { os.Stdin = previous })

			var got bool
			out, errOut := captureStreams(t, func() { got = clioutput.Confirm("Delete %d files?", 2) })
			require.Equal(t, tt.want, got)
			require.Empty(t, out)
			require.Equal(t, "\n? Delete 2 files? (y/N): \n", errOut)
		})
	}
}

// TestConfirmLeavesTheAnswersTypedAheadToTheQuestionsAfterIt pins that the
// questions of one run read their answers off one standard input in turn:
// the answers piped in for the questions to come are not taken with the
// first.
func TestConfirmLeavesTheAnswersTypedAheadToTheQuestionsAfterIt(t *testing.T) {
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	_, err = writer.WriteString("y\nyes\nn\n")
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	previous := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() { os.Stdin = previous })

	var answers []bool
	captureStreams(t, func() {
		for range 4 {
			answers = append(answers, clioutput.Confirm("Go on?"))
		}
	})
	require.Equal(t, []bool{true, true, false, false}, answers)
}

// TestColorsFollowEachStream pins that a line is colored by the stream it
// goes to: with standard output taking colors, a line of standard error is
// colored only when standard error is a terminal too, which the pipe the
// test reads it through is not.
func TestColorsFollowEachStream(t *testing.T) {
	previous := color.NoColor
	color.NoColor = false
	t.Cleanup(func() { color.NoColor = previous })

	out, err := captureStreams(t, func() {
		clioutput.Success("written")
		clioutput.Warn("left as it is")
	})
	require.Equal(t, "  \x1b[92m✔\x1b[0m written\n", out)
	require.Equal(t, "  ⚠ left as it is\n", err)
	require.Equal(t, "\x1b[90mdim\x1b[0m", clioutput.Text(clioutput.StyleMuted, "dim"))
	require.Equal(t, "plain", clioutput.Text(clioutput.StylePlain, "plain"))

	color.NoColor = true
	out, _ = captureStreams(t, func() { clioutput.Success("written") })
	require.Equal(t, "  ✔ written\n", out)
}

// TestElapsed pins how the time a command took is written: to the
// millisecond under a second, to the tenth of a second under a minute, to
// the second from there on.
func TestElapsed(t *testing.T) {
	tests := []struct {
		elapsed time.Duration
		want    string
	}{
		{elapsed: 240*time.Millisecond + 400*time.Microsecond, want: "240ms"},
		{elapsed: 1280 * time.Millisecond, want: "1.3s"},
		{elapsed: 59 * time.Second, want: "59s"},
		{elapsed: 72*time.Second + 400*time.Millisecond, want: "1m12s"},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, clioutput.Elapsed(tt.elapsed))
	}
}

// captureStreams runs fn with standard output and standard error each
// replaced by a pipe, and returns what fn wrote to them.
func captureStreams(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	outReader, outWriter, err := os.Pipe()
	require.NoError(t, err)
	errReader, errWriter, err := os.Pipe()
	require.NoError(t, err)

	previousOut, previousErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outWriter, errWriter
	defer func() { os.Stdout, os.Stderr = previousOut, previousErr }()
	fn()
	require.NoError(t, outWriter.Close())
	require.NoError(t, errWriter.Close())

	out, err := io.ReadAll(outReader)
	require.NoError(t, err)
	errOut, err := io.ReadAll(errReader)
	require.NoError(t, err)
	return string(out), string(errOut)
}
