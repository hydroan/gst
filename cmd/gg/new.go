package main

import (
	"go/build"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/clioutput"
	pkgnew "github.com/hydroan/gst/internal/codegen/new"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/spf13/cobra"
)

var newCmd = &cobra.Command{
	Use:   "new <module-path> [.]",
	Short: "create a project",
	Long: `Create a project whose module path is <module-path>: in a new directory named
after the last element of the module path, or, when "." follows it, in the
current directory, which must have that name. gg new refuses a directory
already holding any file or directory it creates, before it writes anything,
and initializes a Git repository unless the project lies inside one already.`,
	Example: `  gg new github.com/example/myapp
  gg new github.com/example/myapp .`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		inPlace := len(args) == 2
		if inPlace && args[1] != "." && args[1] != "./" {
			return errors.Newf(`gg new creates the project in a new directory, or with "." in the current one, not in %s`, args[1])
		}
		return newProject(args[0], inPlace)
	},
}

// newProject creates the project whose module path is projectName, printing
// each step: in a new directory named after the last element of projectName,
// or, when inPlace, in the working directory, which must have that name. It
// refuses a directory already holding an entry the project gets before it
// creates anything; then it initializes the Go module, writes the files
// pkgnew.ProjectFiles lists, tidies the dependencies and initializes a Git
// repository, unless the project lies inside one already.
func newProject(projectName string, inPlace bool) error {
	if build.IsLocalImport(projectName) || filepath.IsAbs(projectName) {
		return errors.Newf("%q is a directory, not a module path: to create the project in the current directory, run gg new <module-path> .", projectName)
	}
	name := filepath.Base(projectName)
	projectDir := name
	if inPlace {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		if filepath.Base(wd) != name {
			return errors.Newf("the module path %s names the project %s, but the current directory is %s: run gg new %s . in a directory named %s", projectName, name, filepath.Base(wd), projectName, name)
		}
		projectDir = "."
	}
	files, err := pkgnew.ProjectFiles(projectName)
	if err != nil {
		return err
	}
	existing, err := existingProjectEntries(projectDir, files)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return errors.Newf("%s already holds what gg new creates: %s; nothing was written", name, strings.Join(existing, ", "))
	}

	if inPlace {
		clioutput.Section("Use Current Directory")
		clioutput.Success("", "%s", name)
	} else {
		clioutput.Section("Create Project Directory")
		if err := os.MkdirAll(projectDir, 0o755); err != nil {
			clioutput.Error("", "failed to create project directory")
			return err
		}
		clioutput.Success("", "%s", projectDir)

		if err := os.Chdir(projectDir); err != nil {
			return err
		}
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
	if err := goModTidy(); err != nil {
		clioutput.Error("", "go mod tidy failed")
		return err
	}
	clioutput.Success("", "Dependencies tidied")

	clioutput.Section("Initialize Git Repository")
	// A project created inside a Git repository, a cloned one or a directory
	// of a larger one, belongs to it: git init would nest a second repository,
	// whose files the enclosing one never tracks. git rev-parse
	// --is-inside-work-tree prints true anywhere inside a work tree and fails
	// outside one.
	if out, err := exec.Command("git", "rev-parse", "--is-inside-work-tree").Output(); err == nil && strings.TrimSpace(string(out)) == "true" {
		clioutput.Info("", "Already inside a Git repository, git init skipped")
	} else {
		cmd = exec.Command("git", "init")
		cmd.Stdout = io.Discard
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			clioutput.Error("", "git init failed")
			return err
		}
		clioutput.Success("", "Git repository initialized")
	}

	clioutput.Section("Project Initialization Completed")
	clioutput.Done("Project %s created successfully!", clioutput.Text(clioutput.StyleBold, "%s", name))
	clioutput.Section("Next Steps")
	if !inPlace {
		clioutput.Command("cd %s", projectDir)
	}
	clioutput.Command("git add .")
	clioutput.Command("git commit -m \"Initial commit\"")

	return nil
}

// goModTidy tidies the module in the working directory, printing only its
// errors. Tidying resolves the framework over the network, so the tests of gg
// new replace it.
var goModTidy = func() error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// existingProjectEntries returns, sorted, the entries of dir that a new
// project gets and that already exist: go.mod, go.sum and the top-level entry
// of every file in files. For files holding main.go and model/model.gen.go, a
// dir holding go.mod and a model directory yields [go.mod model], and a dir
// that does not exist yields none. An entry counts even as a dangling symbolic
// link, since writing the file would follow it.
func existingProjectEntries(dir string, files []pkgnew.ProjectFile) ([]string, error) {
	entries := []string{"go.mod", "go.sum"}
	for _, file := range files {
		entry, _, _ := strings.Cut(file.Path, "/")
		entries = append(entries, entry)
	}
	slices.Sort(entries)

	var existing []string
	for _, entry := range slices.Compact(entries) {
		_, err := os.Lstat(filepath.Join(dir, entry))
		switch {
		case err == nil:
			existing = append(existing, entry)
		case !errors.Is(err, fs.ErrNotExist):
			return nil, err
		}
	}
	return existing, nil
}
