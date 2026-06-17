package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/spf13/cobra"
)

const (
	golangciLintBinary  = "golangci-lint"
	golangciLintPackage = "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"
	golangciLintVersion = "v2.12.2"
)

var lintCmd = &cobra.Command{
	Use:   "lint",
	Short: "run golangci-lint",
	Long:  "Run golangci-lint for the current project, installing it first when it is missing.",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		lintRun()
	},
}

type golangciLintRunner interface {
	LookPath(file string) (string, error)
	Run(name string, args ...string) error
	Env(name string) string
}

type osGolangciLintRunner struct{}

func (osGolangciLintRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (osGolangciLintRunner) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if cwd, err := os.Getwd(); err == nil {
		cmd.Dir = cwd
		cmd.Env = envWithPWD(cwd)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (osGolangciLintRunner) Env(name string) string {
	if value := os.Getenv(name); len(value) > 0 {
		return value
	}
	if name != "GOBIN" && name != "GOPATH" {
		return ""
	}

	output, err := exec.Command("go", "env", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func lintRun() {
	if err := runGolangciLint(osGolangciLintRunner{}); err != nil {
		clioutput.Error("", "%v", err)
		os.Exit(1)
	}
}

func runGolangciLint(runner golangciLintRunner) error {
	path, err := runner.LookPath(golangciLintBinary)
	if err != nil {
		if err = installGolangciLint(runner); err != nil {
			return err
		}
		path = installedGolangciLintPath(runner)
	}

	clioutput.Section("Run golangci-lint")
	clioutput.Command("%s run ./...", golangciLintBinary)
	if err = runner.Run(path, "run", "./..."); err != nil {
		return errors.Wrap(err, "golangci-lint failed")
	}
	return nil
}

func installGolangciLint(runner golangciLintRunner) error {
	clioutput.Section("Install golangci-lint")
	clioutput.Command("go install %s", golangciLintInstallTarget())
	if err := runner.Run("go", "install", golangciLintInstallTarget()); err != nil {
		return errors.Wrap(err, "failed to install golangci-lint")
	}
	return nil
}

func golangciLintInstallTarget() string {
	return golangciLintPackage + "@" + golangciLintVersion
}

func installedGolangciLintPath(runner golangciLintRunner) string {
	if gobin := runner.Env("GOBIN"); len(gobin) > 0 {
		return filepath.Join(gobin, golangciLintBinary)
	}
	if gopath := runner.Env("GOPATH"); len(gopath) > 0 {
		return filepath.Join(gopath, "bin", golangciLintBinary)
	}
	return golangciLintBinary
}

func envWithPWD(cwd string) []string {
	env := os.Environ()
	pwd := "PWD=" + cwd
	for i, value := range env {
		if strings.HasPrefix(value, "PWD=") {
			env[i] = pwd
			return env
		}
	}
	return append(env, pwd)
}
