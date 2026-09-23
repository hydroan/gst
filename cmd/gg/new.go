package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/hydroan/gst/internal/clioutput"
	pkgnew "github.com/hydroan/gst/internal/codegen/new"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/spf13/cobra"
)

var newCmd = &cobra.Command{
	Use:   "new",
	Short: "new a project",
	Long:  "new a project",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := newProject(args[0]); err != nil {
			clioutput.Error("", "%v", err)
			os.Exit(1)
		}
	},
}

// newProject creates the project whose module path is projectName in a
// directory named after its last element, printing each step: it initializes
// the Go module, writes the files pkgnew.ProjectFiles lists, tidies the
// dependencies and initializes a Git repository.
func newProject(projectName string) error {
	projectDir := filepath.Base(projectName)

	clioutput.Section("Create Project Directory")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		clioutput.Error("", "failed to create project directory")
		return err
	}
	clioutput.Success("", "%s", projectDir)

	if err := os.Chdir(projectDir); err != nil {
		return err
	}

	clioutput.Section("Initialize Go Module")
	clioutput.Info("", "go mod init %s", projectName)
	cmd := exec.Command("go", "mod", "init", projectName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		clioutput.Error("", "go mod init failed")
		return err
	}
	clioutput.Success("", "Go module initialized")

	clioutput.Section("Generate Project Files")
	files, err := pkgnew.ProjectFiles(projectName)
	if err != nil {
		return err
	}
	for _, file := range files {
		writeErr := gghelper.EnsureParentDir(file.Path)
		if writeErr == nil {
			writeErr = os.WriteFile(file.Path, []byte(file.Content), ggconst.FileModeGenerated)
		}
		if writeErr != nil {
			clioutput.Error("", "Failed to create %s", file.Path)
			return writeErr
		}
		clioutput.Success("CREATE", "%s", file.Path)
	}

	clioutput.Section("Run Go Mod Tidy")
	cmd = exec.Command("go", "mod", "tidy")
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		clioutput.Error("", "go mod tidy failed")
		return err
	}
	clioutput.Success("", "Dependencies tidied")

	clioutput.Section("Initialize Git Repository")
	cmd = exec.Command("git", "init")
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		clioutput.Error("", "git init failed")
		return err
	}
	clioutput.Success("", "Git repository initialized")

	clioutput.Section("Project Initialization Completed")
	clioutput.Done("Project %s created successfully!", clioutput.Text(clioutput.StyleBold, "%s", projectDir))
	clioutput.Section("Next Steps")
	clioutput.Command("cd %s", projectDir)
	clioutput.Command("git add .")
	clioutput.Command("git commit -m \"Initial commit\"")

	return nil
}
