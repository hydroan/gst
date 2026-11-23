package main

import (
	"go/build"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/ggnew"
	"github.com/spf13/cobra"
)

var newCmd = &cobra.Command{
	Use:   "new <module-path> [.]",
	Short: "Create a project",
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
// each file it creates: in a new directory named after the last element of
// projectName, or, when inPlace, in the working directory, which must have
// that name. It refuses a directory already holding an entry the project
// gets before it creates anything; then it initializes the Go module,
// writes the files ggnew.ProjectFiles lists, tidies the dependencies and
// initializes a Git repository, unless the project lies inside one already,
// and ends on what to do next.
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
	files, err := ggnew.ProjectFiles(projectName)
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

	started := time.Now()
	if !inPlace {
		if err := os.MkdirAll(projectDir, 0o755); err != nil {
			return errors.Wrap(err, "failed to create the project directory")
		}
		if err := os.Chdir(projectDir); err != nil {
			return err
		}
	}

	if err := runQuietly(exec.Command("go", "mod", "init", projectName)); err != nil {
		return errors.Wrap(err, "go mod init failed")
	}

	log := new(changes)
	for _, file := range files {
		writeErr := gghelper.EnsureParentDir(file.Path)
		if writeErr == nil {
			writeErr = os.WriteFile(file.Path, []byte(file.Content), ggconst.FileModeGenerated)
		}
		if writeErr != nil {
			return errors.Wrapf(writeErr, "failed to create %s", file.Path)
		}
		log.record(clioutput.LabelCreate, file.Path, "")
	}

	// Tidying completes the go.mod go mod init wrote and writes go.sum: the
	// two are listed as created once they hold what the project builds with.
	// It looks the framework up, over the network when it has to.
	clioutput.Progress("Resolving the dependencies with go mod tidy")
	if err := tidyModule(&changes{quiet: true}); err != nil {
		return err
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		if gghelper.FileExists(name) {
			log.record(clioutput.LabelCreate, name, "")
		}
	}

	// A project created inside a Git repository, a cloned one or a directory
	// of a larger one, belongs to it: git init would nest a second repository,
	// whose files the enclosing one never tracks. git rev-parse
	// --is-inside-work-tree prints true anywhere inside a work tree and fails
	// outside one.
	if out, err := exec.Command("git", "rev-parse", "--is-inside-work-tree").Output(); err == nil && strings.TrimSpace(string(out)) == "true" {
		clioutput.Item("Inside a Git repository already: git init skipped")
	} else {
		if err := runQuietly(exec.Command("git", "init")); err != nil {
			return errors.Wrap(err, "git init failed")
		}
		clioutput.Success("Git repository initialized")
	}

	clioutput.Done("Created %s (%s)", name, elapsedSince(started))
	clioutput.Section("Next Steps")
	if !inPlace {
		clioutput.Command("cd %s", projectDir)
	}
	clioutput.Item("Declare the first model in a file under %s/, its routes in Design()", ggconst.DirModel)
	clioutput.Command("gg gen")
	clioutput.Command("go run .")

	return nil
}

// existingProjectEntries returns, sorted, the entries of dir that a new
// project gets and that already exist: go.mod, go.sum and the top-level entry
// of every file in files. For files holding main.go and model/model.gen.go, a
// dir holding go.mod and a model directory yields [go.mod model], and a dir
// that does not exist yields none. An entry counts even as a dangling symbolic
// link, since writing the file would follow it.
func existingProjectEntries(dir string, files []ggnew.ProjectFile) ([]string, error) {
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
