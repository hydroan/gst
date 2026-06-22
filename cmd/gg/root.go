package main

import (
	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"
)

var (
	modelDir     = "model"
	serviceDir   = "service"
	routerDir    = "router"
	daoDir       = "dao"
	excludes     []string
	module       string
	debug        bool
	prune        bool
	cleanOrphans bool
)

var rootCmd = &cobra.Command{
	Use:               "gg",
	Short:             "gst code generator",
	Long:              "gst code generator",
	Version:           "1.0.0",
	PersistentPreRunE: rejectFrameworkRootCommand,
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "enable debug logging")
	rootCmd.PersistentFlags().BoolVar(&prune, "prune", false, "Prune disabled service action files with user confirmation")
	rootCmd.PersistentFlags().BoolVar(&cleanOrphans, "clean-orphans", false, "Delete unmanaged files in orphan service directories after pruning")

	rootCmd.AddCommand(
		genCmd,
		newCmd,
		astCmd,
		pruneCmd,
		checkCmd,
		lintCmd,
		routesCmd,
		routeTreeCmd,
		buildCmd,
		releaseCmd,
		configCmd,
		migrateCmd,
		moduleCmd,
	)
}

func rejectFrameworkRootCommand(cmd *cobra.Command, args []string) error {
	if isMetadataCommand(cmd) {
		return nil
	}
	if isGstFrameworkProject(".") {
		return errors.New("gg commands cannot run in the gst framework repository root")
	}
	return nil
}

func isMetadataCommand(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	return cmd.Name() == "help"
}
