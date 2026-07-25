package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// projectProgram is a temporary Go program compiled against the project's own
// module. Running inside the project module is what lets it import the
// project's packages and observe what they register at runtime, which no
// amount of static analysis in gg itself can reproduce.
type projectProgram struct {
	// Content is the program source.
	Content string

	// Stdout receives the program's standard output. A nil value discards it,
	// which is the right choice for a program whose result travels through a
	// file: framework initialization also writes to stdout, so stdout is not a
	// reliable data channel.
	Stdout io.Writer

	// Interactive connects the program to the terminal's standard input, for a
	// program that prompts the user for confirmation.
	Interactive bool
}

// Run compiles and runs the program. Stderr always goes to the terminal, so
// build and runtime failures stay visible regardless of how stdout is wired.
func (p projectProgram) Run() error {
	tempDir, err := os.MkdirTemp("", "gg-run-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary program directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	runnerFile := filepath.Join(tempDir, "main.go")
	if err = os.WriteFile(runnerFile, []byte(p.Content), 0o600); err != nil {
		return fmt.Errorf("failed to write temporary program: %w", err)
	}

	// The program is built against a copy of the project's module files, so it
	// resolves the same dependency versions the project does without the
	// temporary directory becoming part of the project module.
	goMod, err := os.ReadFile("go.mod")
	if err != nil {
		return fmt.Errorf("failed to read go.mod: %w", err)
	}
	modFile := filepath.Join(tempDir, "run.mod")
	// #nosec G703 -- modFile is created under an os.MkdirTemp-owned directory.
	if err = os.WriteFile(modFile, goMod, 0o600); err != nil {
		return fmt.Errorf("failed to write temporary run.mod: %w", err)
	}
	goSum, err := os.ReadFile("go.sum")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read go.sum: %w", err)
	}
	if goSum != nil {
		// #nosec G703 -- the sum file is created under the same temp directory.
		if err = os.WriteFile(filepath.Join(tempDir, "run.sum"), goSum, 0o600); err != nil {
			return fmt.Errorf("failed to write temporary run.sum: %w", err)
		}
	}

	stdout := p.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	runCmd := exec.Command("go", "run", "-mod=mod", "-modfile", modFile, runnerFile)
	runCmd.Stdout = stdout
	runCmd.Stderr = os.Stderr
	if p.Interactive {
		runCmd.Stdin = os.Stdin
	}
	if err = runCmd.Run(); err != nil {
		return fmt.Errorf("failed to run generated program: %w", err)
	}
	return nil
}
