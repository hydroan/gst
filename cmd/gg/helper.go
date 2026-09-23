package main

import (
	"os"

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
			clioutput.Warn("", "gg no longer reads %s: move its prune.ignore and prune.orphan_ignore entries into %s under prune.ignore, written as paths (directory prefixes, not regular expressions)", name, ggconfig.FileName)
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
			if err := os.WriteFile(filename, []byte(content), ggconst.FileModeGenerated); err != nil {
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
		if err := os.WriteFile(filename, []byte(content), ggconst.FileModeGenerated); err != nil {
			return err
		}
	}
	return nil
}
