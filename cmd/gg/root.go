package main

import (
	rtdebug "runtime/debug"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/spf13/cobra"
)

var (
	module string
	debug  bool
)

var rootCmd = &cobra.Command{
	Use:   "gg",
	Short: "gst code generator",
	Long:  "gst code generator",
	// main prints a failed command's error, once and in gg's own style;
	// cobra prints none of it.
	SilenceErrors:     true,
	PersistentPreRunE: startCommand,
}

func init() {
	// gg reports the module version Go recorded when it was built: the tag it
	// was installed at, a pseudo-version for an untagged commit, "(devel)" for
	// a build that recorded none.
	if info, ok := rtdebug.ReadBuildInfo(); ok {
		rootCmd.Version = info.Main.Version
	}

	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "enable debug logging")

	rootCmd.AddCommand(
		genCmd,
		newCmd,
		pruneCmd,
		checkCmd,
		lintCmd,
		routesCmd,
		routeTreeCmd,
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
