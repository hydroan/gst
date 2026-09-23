package main

import (
	"os"

	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/codegen"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/spf13/cobra"
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "clean unused service files",
	Long:  "Clean unused service files that are no longer needed based on current model definitions",
	Run: func(cmd *cobra.Command, args []string) {
		pruneRun()
	},
}

func pruneRun() {
	ignore := gghelper.NewProjectIgnore()
	if len(module) == 0 {
		var err error
		module, err = gghelper.ModulePath()
		checkErr(err)
	}

	if !gghelper.FileExists(ggconst.DirModel) {
		clioutput.Error("", "model dir not found: %s", ggconst.DirModel)
		os.Exit(1)
	}

	// Scan all models
	clioutput.Section("Scan Models")
	allModels, err := codegen.FindModels(module, ggconst.DirModel, ignore)
	checkErr(err)
	if len(allModels) == 0 {
		clioutput.Item("", "No models found, pruning service files only")
	} else {
		clioutput.Success("", "%d models found", len(allModels))
	}

	// Route ignores from gst.yaml are intentionally not applied here: an
	// ignored action keeps its service file on disk, so leaving the action
	// enabled makes prune treat the file as expected without extra bookkeeping.

	// Scan existing service files
	oldServiceFiles := scanExistingServiceFiles(ggconst.DirService, ignore)

	// Prune disabled service files
	clioutput.Section("Prune Disabled Service Files")
	pruneServiceFiles(oldServiceFiles, allModels, nil, nil, ignore)

	clioutput.Done("Code pruning completed successfully!")
}
