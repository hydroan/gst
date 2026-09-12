package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

	// Overlay maps project file paths to replacement contents for this build
	// only: the compiler sees the replacements while the files on disk stay
	// untouched. Column inspection uses it to stub out previously generated
	// column files and to leave out the handwritten code that reads them.
	Overlay map[string]string
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
	modFile, err := writeTemporaryModFiles(tempDir)
	if err != nil {
		return err
	}

	stdout := p.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	args := []string{"run", "-mod=mod", "-modfile", modFile}
	if len(p.Overlay) > 0 {
		overlayFile, overlayErr := writeOverlayFile(tempDir, p.Overlay)
		if overlayErr != nil {
			return overlayErr
		}
		args = append(args, "-overlay", overlayFile)
	}
	// #nosec G204 -- every argument is either a literal flag or a path gg
	// itself created under the os.MkdirTemp-owned directory.
	runCmd := exec.Command("go", append(args, runnerFile)...)
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

// writeOverlayFile materializes the replacement contents under dir and
// returns a go build overlay file mapping the original paths to them.
func writeOverlayFile(dir string, overlay map[string]string) (string, error) {
	paths := make([]string, 0, len(overlay))
	for path := range overlay {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	replace := make(map[string]string, len(overlay))
	for i, path := range paths {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("failed to resolve overlay path %s: %w", path, err)
		}
		replacement := filepath.Join(dir, fmt.Sprintf("overlay_%d.go", i))
		// #nosec G703 -- the replacement is created under an os.MkdirTemp-owned directory.
		if err = os.WriteFile(replacement, []byte(overlay[path]), 0o600); err != nil {
			return "", fmt.Errorf("failed to write overlay replacement for %s: %w", path, err)
		}
		replace[abs] = replacement
	}

	encoded, err := json.Marshal(struct {
		Replace map[string]string `json:"Replace"`
	}{Replace: replace})
	if err != nil {
		return "", fmt.Errorf("failed to encode overlay file: %w", err)
	}
	overlayFile := filepath.Join(dir, "overlay.json")
	// #nosec G703 -- the overlay file is created under the same temp directory.
	if err = os.WriteFile(overlayFile, encoded, 0o600); err != nil {
		return "", fmt.Errorf("failed to write overlay file: %w", err)
	}
	return overlayFile, nil
}

// writeTemporaryModFiles copies the project's go.mod, and its go.sum when
// present, into dir as run.mod and run.sum and returns the path of run.mod. A
// go command pointed at it through -modfile resolves exactly the dependency
// versions the project does, while any requirement the command adds lands in
// the copies instead of the project's own files.
func writeTemporaryModFiles(dir string) (string, error) {
	goMod, err := os.ReadFile("go.mod")
	if err != nil {
		return "", fmt.Errorf("failed to read go.mod: %w", err)
	}
	modFile := filepath.Join(dir, "run.mod")
	// #nosec G703 -- modFile is created under an os.MkdirTemp-owned directory.
	if err = os.WriteFile(modFile, goMod, 0o600); err != nil {
		return "", fmt.Errorf("failed to write temporary run.mod: %w", err)
	}
	goSum, err := os.ReadFile("go.sum")
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to read go.sum: %w", err)
	}
	if goSum != nil {
		// #nosec G703 -- the sum file is created under the same temp directory.
		if err = os.WriteFile(filepath.Join(dir, "run.sum"), goSum, 0o600); err != nil {
			return "", fmt.Errorf("failed to write temporary run.sum: %w", err)
		}
	}
	return modFile, nil
}

// listedPackage is the part of the go command's report on a package that gg
// reads.
type listedPackage struct {
	ImportPath string   `json:"ImportPath"`
	Name       string   `json:"Name"`
	Dir        string   `json:"Dir"`
	GoFiles    []string `json:"GoFiles"`
	CgoFiles   []string `json:"CgoFiles"`
}

// listProjectPackages reports the packages behind import paths as the
// project's module resolves them. Like projectProgram, it points the go
// command at copies of the module files, so resolving a path the project does
// not require yet never edits the project's go.mod or go.sum. A path the go
// command cannot load comes back without a name.
func listProjectPackages(paths []string) (map[string]listedPackage, error) {
	listed := make(map[string]listedPackage, len(paths))
	if len(paths) == 0 {
		return listed, nil
	}
	tempDir, err := os.MkdirTemp("", "gg-list-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary list directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	modFile, err := writeTemporaryModFiles(tempDir)
	if err != nil {
		return nil, err
	}
	args := append([]string{"list", "-mod=mod", "-modfile", modFile, "-e", "-json=ImportPath,Name,Dir,GoFiles,CgoFiles"}, paths...)
	var stdout bytes.Buffer
	// #nosec G204 -- the flags are literals, the module file lives under the
	// os.MkdirTemp-owned directory, and the paths are import paths read from
	// project sources.
	listCmd := exec.Command("go", args...)
	listCmd.Stdout = &stdout
	listCmd.Stderr = os.Stderr
	if err = listCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to list packages: %w", err)
	}
	for decoder := json.NewDecoder(&stdout); decoder.More(); {
		var pkg listedPackage
		if err = decoder.Decode(&pkg); err != nil {
			return nil, fmt.Errorf("failed to decode the package list: %w", err)
		}
		listed[pkg.ImportPath] = pkg
	}
	return listed, nil
}
