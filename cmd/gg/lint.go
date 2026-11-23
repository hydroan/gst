package main

import (
	"os"
	"os/exec"

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
	golangciLintVersion = "v2.15.0"
)

var lintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Run golangci-lint",
	Long:  "Run golangci-lint over the current project, at the version the framework pins.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return lintRun()
	},
}

// lintRun runs the pinned golangci-lint through gghelper.PinnedCommand,
// which builds it apart from the project's module and keeps the program:
// nothing is installed, whatever golangci-lint PATH holds plays no part, the
// first run downloads and builds it and later ones find it built, network or
// not. golangci-lint runs in the project directory, where it finds the
// project's .golangci.yml, with the environment gg lint itself got. What
// golangci-lint prints is the output of the command: the issues it found,
// which fail the command without a line of gg's after them, or that it found
// none.
func lintRun() error {
	clioutput.Command("golangci-lint run ./... (%s)", golangciLintVersion)

	cmd, err := gghelper.PinnedCommand(golangciLintModule, golangciLintVersion, golangciLintPackage, "run", "./...")
	if err != nil {
		return err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	// golangci-lint exits with 1 for issues found, which it has listed and
	// counted, and with another code for a run it could not make.
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == golangciLintIssuesFound {
		return errReported
	}
	return errors.Wrap(err, "golangci-lint failed")
}

// golangciLintIssuesFound is the code golangci-lint exits with when it found
// issues.
const golangciLintIssuesFound = 1
