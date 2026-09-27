package main

import (
	"os"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/spf13/cobra"
)

// golangciLintModule, golangciLintPackage and golangciLintVersion name the
// golangci-lint gg lint runs. The version is the one the framework lints
// itself with, the golangci-lint module go.mod requires, which a test keeps
// it equal to; the configuration gg new writes is written against it.
const (
	golangciLintModule  = "github.com/golangci/golangci-lint/v2"
	golangciLintPackage = "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"
	golangciLintVersion = "v2.13.1"
)

var lintCmd = &cobra.Command{
	Use:   "lint",
	Short: "run golangci-lint",
	Long:  "Run golangci-lint over the current project, at the version the framework pins.",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		lintRun()
	},
}

// lintRun runs the pinned golangci-lint through gghelper.PinnedCommand,
// which builds it apart from the project's module and caches the executable:
// nothing is installed, whatever golangci-lint PATH holds plays no part, the
// first run downloads and builds it and later ones need neither the network
// nor a build. golangci-lint runs in the project directory, where it finds
// the project's .golangci.yml.
func lintRun() {
	clioutput.Section("Run golangci-lint " + golangciLintVersion)
	clioutput.Command("golangci-lint run ./...")

	cmd, err := gghelper.PinnedCommand(golangciLintModule, golangciLintVersion, golangciLintPackage, "run", "./...")
	if err != nil {
		clioutput.Error("", "%v", err)
		os.Exit(1)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		clioutput.Error("", "%v", errors.Wrap(err, "golangci-lint failed"))
		os.Exit(1)
	}
}
