package main

import (
	"os"

	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/codegen"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
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
	if len(module) == 0 {
		var err error
		module, err = gen.GetModulePath()
		checkErr(err)
	}

	if !fileExists(modelDir) {
		clioutput.Error("", "model dir not found: %s", modelDir)
		os.Exit(1)
	}

	// Scan all models
	clioutput.Section("Scan Models")
	allModels, err := codegen.FindModels(module, modelDir, serviceDir, excludes)
	checkErr(err)
	if len(allModels) == 0 {
		clioutput.Item("", "No models found, pruning service files only")
	} else {
		clioutput.Success("", "%d models found", len(allModels))
	}

	// Align the prune view with gg gen: build hierarchical endpoints, then
	// apply project-level route ignores so ignored actions no longer count
	// as expected service files.
	buildHierarchicalEndpoints(allModels)
	propagateParentParams(allModels)
	projectCfg, err := ggconfig.Load(".")
	checkErr(err)
	reportUnmatchedRouteIgnores(applyRouteIgnores(allModels, projectCfg.Gen.Routes.Ignore))

	// Scan existing service files
	oldServiceFiles := scanExistingServiceFiles(serviceDir)

	// Prune disabled service files
	clioutput.Section("Prune Disabled Service Files")
	pruneServiceFiles(oldServiceFiles, allModels)

	clioutput.Done("Code pruning completed successfully!")
}
