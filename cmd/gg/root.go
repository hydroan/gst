package main

import (
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/spf13/cobra"
)

var (
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
	// main prints a failed command's error, once and in gg's own style;
	// cobra prints none of it.
	SilenceErrors:     true,
	PersistentPreRunE: startCommand,
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

// startCommand runs before every command, once cobra has parsed the command
// line. An error from here on comes from running the command, which its usage
// would not explain, so cobra prints the usage only for a command line gg
// cannot run. Project commands stay out of the framework repository root;
// help, which only describes commands, runs anywhere.
func startCommand(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true
	if cmd.Name() != "help" && gghelper.IsFrameworkProject(".") {
		return errors.New("gg commands cannot run in the gst framework repository root")
	}
	return nil
}
