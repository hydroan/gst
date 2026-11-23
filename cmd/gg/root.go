package main

import (
	"runtime/debug"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/spf13/cobra"
)

var module string

var rootCmd = &cobra.Command{
	Use:   "gg",
	Short: "Generate, check and migrate a gst project",
	Long: `gg works on the gst project in the current directory: it generates the
code the models declare, checks the project against the framework's
conventions, and migrates the database to the models.`,
	// main prints a failed command's error, once and in gg's own style, and
	// points a command line gg cannot run to the help; cobra prints neither
	// the error nor the usage.
	SilenceErrors:     true,
	SilenceUsage:      true,
	PersistentPreRunE: startCommand,
}

func init() {
	// gg reports the module version Go recorded when it was built: the tag it
	// was installed at, a pseudo-version for an untagged commit, "(devel)" for
	// a build that recorded none.
	if info, ok := debug.ReadBuildInfo(); ok {
		rootCmd.Version = info.Main.Version
	}

	rootCmd.AddCommand(
		genCmd,
		newCmd,
		pruneCmd,
		checkCmd,
		lintCmd,
		routesCmd,
		configCmd,
		migrateCmd,
		moduleCmd,
	)
}

// commandStarted is set once cobra has accepted the command line and the
// command is about to run: an error before that is a command line gg cannot
// run, an error after it a command that failed (see run).
var commandStarted bool

// startCommand runs before every command, once cobra has parsed the command
// line. The commands that describe gg, help and completion, run anywhere. A
// project command stays out of the framework repository root, and runs only
// with the gg the project's framework version comes with (see
// frameworkVersionMismatch); gg new, which starts a project, is held to no
// version.
func startCommand(cmd *cobra.Command, _ []string) error {
	commandStarted = true
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "help", "completion", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
			return nil
		}
	}
	if gghelper.IsFrameworkProject(".") {
		return errors.New("gg commands cannot run in the gst framework repository root")
	}
	if cmd == newCmd {
		return nil
	}
	return frameworkVersionMismatch(cmd.Root().Version)
}

// frameworkVersionMismatch refuses to run gg, built at the framework version
// built, in a project whose go.mod requires another one: gg generates code
// for its own version and builds its programs inside the project with the
// project's, so the two differing ends in generated code that does not
// compile or in a build error of a program the project never wrote. The
// error names both versions and the command for either way out:
//
//	gg is built from github.com/hydroan/gst v1.3.0, and go.mod requires v1.2.0: what one version generates need not build with the other
//	  To install the gg the project requires: go install github.com/hydroan/gst/cmd/gg@v1.2.0
//	  To move the project to this gg: go get github.com/hydroan/gst@v1.3.0
//
// Nothing is compared where the project does not build with the version it
// requires, or gg states none: a gg built from a working copy, which
// records no version or one marked +dirty; a go.mod replacing the framework
// module; a Go workspace, which supplies the module; and a directory whose
// go.mod requires no framework.
func frameworkVersionMismatch(built string) error {
	if built == "" || built == "(devel)" || strings.Contains(built, "+dirty") {
		return nil
	}
	required, replaced, err := gghelper.FrameworkRequirement(".")
	if err != nil || required == "" || replaced || required == built {
		return err
	}
	inWorkspace, err := gghelper.InWorkspace()
	if err != nil || inWorkspace {
		return err
	}
	return errors.Newf("gg is built from %[1]s %[2]s, and go.mod requires %[3]s: what one version generates need not build with the other\n"+
		"  To install the gg the project requires: go install %[1]s/cmd/gg@%[3]s\n"+
		"  To move the project to this gg: go get %[1]s@%[2]s",
		ggconst.ImportPathGst, built, required)
}
