package main

import (
	"io"
	"os"
	"os/exec"

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
			clioutput.Warn("", "gg does not read %s: move its prune.ignore and prune.orphan_ignore entries into %s under prune.ignore, written as paths (directory prefixes, not regular expressions)", name, ggconfig.FileName)
			continue
		}
		clioutput.Warn("", "gg reads only %s, not %s: rename it, or merge it into %s when both exist", ggconfig.FileName, name, ggconfig.FileName)
	}
	return ggconfig.Load(".")
}

// writeGeneratedFile writes content to filename unless the file holds it
// already, creating the parent directory when needed, and prints CREATE,
// UPDATE or SKIP for it when log is set.
func writeGeneratedFile(filename string, content string, log bool) error {
	if gghelper.FileExists(filename) {
		oldData, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		if string(oldData) == content {
			if log {
				clioutput.Item("SKIP", "%s", filename)
			}
		} else {
			if log {
				clioutput.Status(clioutput.StyleWarn, clioutput.SymbolSuccess, "UPDATE", "%s", filename)
			}
			if err := os.WriteFile(filename, []byte(content), ggconst.FileModeGenerated); err != nil { //nolint:gosec // G703: the path is the project file the generator was asked to write.
				return err
			}
		}
	} else {
		if log {
			clioutput.Success("CREATE", "%s", filename)
		}
		if err := gghelper.EnsureParentDir(filename); err != nil {
			return err
		}
		if err := os.WriteFile(filename, []byte(content), ggconst.FileModeGenerated); err != nil { //nolint:gosec // G703: the path is the project file the generator was asked to write.
			return err
		}
	}
	return nil
}

// tidyModule tidies the module in the working directory, under a section of
// the output of its own, once gg new, gg gen or gg module copy has written
// its files, so that go.mod and go.sum hold what the written sources import
// (see goModTidy). In a Go workspace it runs no go mod tidy and says why:
// the command reads no go.work, so it would look up a module the workspace
// provides, or settle on a version the workspace does not build with, while
// the workspace resolves the versions itself and adds no requirement to any
// go.mod.
func tidyModule() error {
	clioutput.Section("Run Go Mod Tidy")
	inWorkspace, err := gghelper.InWorkspace()
	if err != nil {
		return err
	}
	if inWorkspace {
		clioutput.Warn("", "go mod tidy skipped: the project is in a Go workspace, whose go.work go mod tidy does not read; run go mod tidy outside the workspace before building there")
		return nil
	}
	if err = goModTidy(); err != nil {
		clioutput.Error("", "go mod tidy failed")
		return err
	}
	clioutput.Success("", "Dependencies tidied")
	return nil
}

// goModTidy runs go mod tidy in the working directory for tidyModule,
// printing only its errors. Tests replace it: those of gg new because
// tidying resolves the framework over the network, those of gg gen and gg
// module copy to see when it runs and to make it fail.
var goModTidy = func() error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
