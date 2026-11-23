package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

// loadProjectConfig reads gst.yaml from the project in the working directory,
// after warning about each file next to it that looks like gg configuration
// but is not read, so a setting written into one of them is not lost without
// a word.
func loadProjectConfig() (*ggconfig.Config, error) {
	for _, name := range ggconfig.UnreadFiles(".") {
		if ggconfig.IsLegacyPruneSettings(name) {
			clioutput.Warn("gg does not read %s: move its prune.ignore and prune.orphan_ignore entries into %s under prune.ignore, written as paths (directory prefixes, not regular expressions)", name, ggconfig.FileName)
			continue
		}
		clioutput.Warn("gg reads only %s, not %s: rename it, or merge it into %s when both exist", ggconfig.FileName, name, ggconfig.FileName)
	}
	return ggconfig.Load(".")
}

// elapsedSince tells how long the command has taken since started, the way
// the line it ends on states it, the time it waited for answers aside.
func elapsedSince(started time.Time) string {
	return clioutput.Elapsed(time.Since(started) - clioutput.Waited())
}

// counted writes a number of things the way a sentence does: "1 route",
// "54 routes".
func counted(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// changes prints what a run does to the files of the project, a line each,
// and counts the files it creates, updates and removes for the line the run
// ends on. A quiet one prints nothing and counts all the same: gg module
// copy runs gg gen that way, its own report covering what the copy wrote.
type changes struct {
	quiet                     bool
	created, updated, removed map[string]struct{}
}

// record prints what the run did to the file at path under label, followed
// by the detail when there is one, and counts the file once whatever the
// number of lines about it: under the files created for LabelCreate, removed
// for LabelRemove and updated for LabelUpdate and LabelFix; a file under
// another label, one left as it is, is printed and not counted. A file the
// run created and then wrote again is a file created, and one it updated
// and then wrote again is updated once: the second write, an update with no
// detail to add, is neither printed nor counted. The line of
//
//	record(clioutput.LabelRemove, "pb/sample.pb.go", " (its model is no longer served over gRPC)")
//
// is
//
//	REMOVE  pb/sample.pb.go (its model is no longer served over gRPC)
func (c *changes) record(label clioutput.Label, path string, detail string, args ...any) {
	var counted *map[string]struct{}
	switch label {
	case clioutput.LabelCreate:
		counted = &c.created
	case clioutput.LabelUpdate, clioutput.LabelFix:
		_, created := c.created[path]
		_, updated := c.updated[path]
		if label == clioutput.LabelUpdate && (created || updated && detail == "") {
			return
		}
		counted = &c.updated
	case clioutput.LabelRemove:
		counted = &c.removed
	}
	if counted != nil {
		if *counted == nil {
			*counted = make(map[string]struct{})
		}
		(*counted)[path] = struct{}{}
	}
	if !c.quiet {
		clioutput.Change(label, "%s%s", path, fmt.Sprintf(detail, args...))
	}
}

// summary tells what the run changed, the way the line it ends on states
// it: "2 created, 1 updated, 3 removed", the kinds it changed nothing of
// left out, and "" for a run that changed nothing.
func (c *changes) summary() string {
	var parts []string
	for _, kind := range []struct {
		files map[string]struct{}
		name  string
	}{{c.created, "created"}, {c.updated, "updated"}, {c.removed, "removed"}} {
		if len(kind.files) > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", len(kind.files), kind.name))
		}
	}
	return strings.Join(parts, ", ")
}

// writeGeneratedFile writes content to filename unless the file holds it
// already, creating the parent directory when needed, and records in log
// the file it wrote, as created or updated.
func writeGeneratedFile(filename string, content string, log *changes) error {
	label := clioutput.LabelCreate
	if gghelper.FileExists(filename) {
		oldData, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		if string(oldData) == content {
			return nil
		}
		label = clioutput.LabelUpdate
	} else if err := gghelper.EnsureParentDir(filename); err != nil {
		return err
	}
	if err := os.WriteFile(filename, []byte(content), ggconst.FileModeGenerated); err != nil { //nolint:gosec // G703: the path is the project file the generator was asked to write.
		return err
	}
	log.record(label, filename, "")
	return nil
}

// tidyModule tidies the module in the working directory once gg new, gg gen,
// gg module copy or gg upgrade has written its files, so that go.mod and
// go.sum hold what the written sources import (see goModTidy), and records
// in log which of the two it changed. In a Go workspace it runs no go mod tidy and says
// why: the command reads no go.work, so it would look up a module the
// workspace provides, or settle on a version the workspace does not build
// with, while the workspace resolves the versions itself and adds no
// requirement to any go.mod.
func tidyModule(log *changes) error {
	inWorkspace, err := gghelper.InWorkspace()
	if err != nil {
		return err
	}
	if inWorkspace {
		clioutput.Warn("go mod tidy skipped: the project is in a Go workspace, whose go.work go mod tidy does not read; run go mod tidy outside the workspace before building there")
		return nil
	}
	return recordModuleFiles(log, func() error {
		return errors.Wrap(goModTidy(), "go mod tidy failed")
	})
}

// recordModuleFiles runs goCommand, a go command that may rewrite go.mod and
// go.sum in the working directory, and records in log which of the two it
// changed.
func recordModuleFiles(log *changes, goCommand func() error) error {
	moduleFiles := []string{"go.mod", "go.sum"}
	before := make(map[string][]byte, len(moduleFiles))
	for _, name := range moduleFiles {
		// A file that cannot be read counts as missing: the go command reports
		// what is wrong with it.
		if content, readErr := os.ReadFile(name); readErr == nil {
			before[name] = content
		}
	}
	if err := goCommand(); err != nil {
		return err
	}
	for _, name := range moduleFiles {
		content, readErr := os.ReadFile(name)
		old, existed := before[name]
		switch {
		case readErr != nil || string(content) == string(old):
		case existed:
			log.record(clioutput.LabelUpdate, name, "")
		default:
			log.record(clioutput.LabelCreate, name, "")
		}
	}
	return nil
}

// goModTidy runs go mod tidy in the working directory for tidyModule,
// printing what it says only when it fails. Tests replace it: those of gg
// new because tidying resolves the framework over the network, those of gg
// gen and gg module copy to see when it runs and to make it fail.
var goModTidy = func() error {
	return runQuietly(exec.Command("go", "mod", "tidy"))
}

// runQuietly runs cmd and keeps what it prints to itself unless it fails:
// the output of a command that went well, the modules go mod tidy looked up
// or the hints of git init, says nothing the lines of gg do not.
func runQuietly(cmd *exec.Cmd) error {
	output, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = os.Stderr.Write(output)
	}
	return err
}
