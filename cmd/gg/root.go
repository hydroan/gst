package main

import (
	"runtime/debug"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
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
		upgradeCmd,
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
// frameworkVersionMismatch); gg new, which starts a project, and gg upgrade,
// which moves a project to the version of gg, are held to no version.
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
	if cmd == newCmd || cmd == upgradeCmd {
		return nil
	}
	return frameworkVersionMismatch(cmd.Root().Version)
}

// showHelp is what a command that only groups others runs: called by its
// name alone, it prints its help. Beside cobra.NoArgs it makes the group
// refuse a command it does not have, where cobra, for a command that runs
// nothing, prints the help whatever follows its name and exits with 0.
func showHelp(cmd *cobra.Command, _ []string) error { return cmd.Help() }

// frameworkVersionMismatch refuses to run gg, built at the framework version
// built, in a project whose go.mod requires another one: gg generates code
// for its own version and builds its programs inside the project with the
// project's, so the two differing ends in generated code that does not
// compile or in a build error of a program the project never wrote. The
// error names both versions and the command for either way out, the second
// of them only for a gg newer than the project, the way gg upgrade moves one:
//
//	gg is built from github.com/hydroan/gst v1.3.0, and go.mod requires v1.2.0: what one version generates need not build with the other
//	  To install the gg the project requires: go install github.com/hydroan/gst/cmd/gg@v1.2.0
//	  To move the project to this gg: gg upgrade
//
// Nothing is compared where the project does not build with the version it
// requires, or gg names none (see namesVersion): a go.mod replacing the
// framework module; a Go workspace, which supplies the module; and a
// directory whose go.mod requires no framework.
func frameworkVersionMismatch(built string) error {
	if !namesVersion(built) {
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
	upgrade := ""
	if semver.Compare(built, required) > 0 {
		upgrade = "\n  To move the project to this gg: gg upgrade"
	}
	return errors.Newf("gg is built from %[1]s %[2]s, and go.mod requires %[3]s: what one version generates need not build with the other\n"+
		"  To install the gg the project requires: go install %[1]s/cmd/gg@%[3]s%[4]s",
		ggconst.ImportPathGst, built, required, upgrade)
}

// namesVersion reports whether built, the version gg records of itself,
// names a version of the framework: a gg built from a working copy records
// none, "(devel)", or one marked +dirty.
func namesVersion(built string) bool {
	return built != "" && built != "(devel)" && !strings.Contains(built, "+dirty")
}
