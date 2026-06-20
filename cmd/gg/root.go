package main

import "github.com/spf13/cobra"

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
	Use:     "gg",
	Short:   "gst code generator",
	Long:    "gst code generator",
	Version: "1.0.0",
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
	)
}
