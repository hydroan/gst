package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/stretchr/testify/require"
)

// TestChangesRecordPrintsEachChangeAndCountsEachFileOnce pins the example
// record gives and what the log of a run counts: a file created, updated —
// however many declarations were fixed in it — or removed once each, a file
// the run created and wrote again as one created and a file it updated
// twice as one updated, with no line for the second write, and a file under
// another label printed and not counted. A quiet log counts the same and
// prints nothing; the summary leaves out the kinds nothing was changed of,
// and is empty for a run that changed nothing.
func TestChangesRecordPrintsEachChangeAndCountsEachFileOnce(t *testing.T) {
	record := func(log *changes) {
		log.record(clioutput.LabelCreate, "service/sample/create.go", "")
		log.record(clioutput.LabelUpdate, "service/sample/create.go", "")
		log.record(clioutput.LabelUpdate, "router/router.gen.go", "")
		log.record(clioutput.LabelUpdate, "router/router.gen.go", "")
		log.record(clioutput.LabelFix, "model/sample.go", ": numbered %s.%s pb:%q", "Sample", "Name", "1")
		log.record(clioutput.LabelFix, "model/sample.go", ": numbered %s.%s pb:%q", "Sample", "Code", "2")
		log.record(clioutput.LabelRemove, "pb/sample.pb.go", " (its model is no longer served over gRPC)")
		log.record(clioutput.LabelSkip, "service/sample/list.go", "")
	}

	log := new(changes)
	require.Equal(t, `  CREATE  service/sample/create.go
  UPDATE  router/router.gen.go
  FIX     model/sample.go: numbered Sample.Name pb:"1"
  FIX     model/sample.go: numbered Sample.Code pb:"2"
  REMOVE  pb/sample.pb.go (its model is no longer served over gRPC)
  SKIP    service/sample/list.go
`, captureStdout(t, func() { record(log) }))
	require.Equal(t, "1 created, 2 updated, 1 removed", log.summary())

	quiet := &changes{quiet: true}
	require.Empty(t, captureStdout(t, func() { record(quiet) }))
	require.Equal(t, "1 created, 2 updated, 1 removed", quiet.summary())

	removed := new(changes)
	captureStdout(t, func() { removed.record(clioutput.LabelRemove, "service/sample", " (an empty directory)") })
	require.Equal(t, "1 removed", removed.summary())
	require.Empty(t, new(changes).summary())
}

// TestCounted pins the examples counted gives.
func TestCounted(t *testing.T) {
	require.Equal(t, "1 route", counted(1, "route"))
	require.Equal(t, "54 routes", counted(54, "route"))
	require.Equal(t, "0 routes", counted(0, "route"))
}

// TestWriteGeneratedFileRecordsWhatItWrote pins that a generated file is
// written and recorded when it is new or holds something else, its
// directory created with it, and neither written nor mentioned when it
// holds the content already.
func TestWriteGeneratedFileRecordsWhatItWrote(t *testing.T) {
	t.Chdir(t.TempDir())
	path := filepath.Join("service", "sample", "create.go")
	log := new(changes)

	output := captureStdout(t, func() {
		require.NoError(t, writeGeneratedFile(path, "package sample\n", log))
		require.NoError(t, writeGeneratedFile(path, "package sample\n", log))
	})
	require.Equal(t, "  CREATE  "+path+"\n", output)

	other := new(changes)
	output = captureStdout(t, func() {
		require.NoError(t, writeGeneratedFile(path, "package sample\n\nvar Changed bool\n", other))
	})
	require.Equal(t, "  UPDATE  "+path+"\n", output)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "package sample\n\nvar Changed bool\n", string(content))
	require.Equal(t, "1 created", log.summary())
	require.Equal(t, "1 updated", other.summary())
}

// TestTidyModuleRecordsTheModuleFilesItChanged pins what tidying reports:
// go.mod and go.sum each as created or updated when go mod tidy left it
// other than it found it, nothing for a file it left alone, and the failure
// of go mod tidy as the failure of the step.
func TestTidyModuleRecordsTheModuleFilesItChanged(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("go.mod", []byte("module tmpapp\n\ngo 1.26\n"), 0o600))
	tidy := goModTidy
	t.Cleanup(func() { goModTidy = tidy })

	goModTidy = func() error {
		if err := os.WriteFile("go.mod", []byte("module tmpapp\n\ngo 1.26\n\nrequire example.com/sample v1.0.0\n"), 0o600); err != nil {
			return err
		}
		return os.WriteFile("go.sum", []byte("example.com/sample v1.0.0 h1:sample\n"), 0o600)
	}
	log := new(changes)
	output := captureStdout(t, func() { require.NoError(t, tidyModule(log)) })
	require.Equal(t, "  UPDATE  go.mod\n  CREATE  go.sum\n", output)

	goModTidy = func() error { return nil }
	output = captureStdout(t, func() { require.NoError(t, tidyModule(log)) })
	require.Empty(t, output)

	goModTidy = func() error { return errors.New("no network") }
	require.EqualError(t, tidyModule(log), "go mod tidy failed: no network")
}
