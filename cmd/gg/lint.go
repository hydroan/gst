package main

import (
	"os"
	"os/exec"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/spf13/cobra"
)

// golangciLintPackage and golangciLintVersion name the golangci-lint gg lint
// runs. The version is the one the framework lints itself with, the
// golangci-lint module go.mod requires, which a test keeps it equal to; the
// configuration gg new writes is written against it.
const (
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

// lintRun runs the pinned golangci-lint through go run, which builds it apart
// from the project's module and caches the executable: nothing is installed,
// and whatever golangci-lint PATH holds plays no part. The first run builds
// it, later ones reuse the cached executable. golangci-lint finds the project's
// .golangci.yml by walking up from the working directory it runs in.
func lintRun() {
	target := golangciLintPackage + "@" + golangciLintVersion
	clioutput.Section("Run golangci-lint")
	clioutput.Command("go run %s run ./...", target)

	cmd := exec.Command("go", "run", target, "run", "./...")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		clioutput.Error("", "%v", errors.Wrap(err, "golangci-lint failed"))
		os.Exit(1)
	}
}
