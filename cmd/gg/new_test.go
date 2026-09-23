package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	pkgnew "github.com/hydroan/gst/internal/codegen/new"
	"github.com/stretchr/testify/require"
)

// TestNewRefusesADirectoryHoldingWhatItCreates pins that gg new checks the
// project directory before it writes anything, whether it names a directory
// that already exists or generates into the current one: every entry the
// project gets that is already there — a file, a directory, a dangling
// symbolic link writing would follow — is reported at once, and the directory
// is left as it was.
func TestNewRefusesADirectoryHoldingWhatItCreates(t *testing.T) {
	for _, tc := range []struct {
		name    string
		inPlace bool
	}{
		{name: "named directory"},
		{name: "current directory", inPlace: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project := filepath.Join(t.TempDir(), "sampleapp")
			writeProjectFile(t, filepath.Join(project, ".gitignore"), "keep\n")
			writeProjectFile(t, filepath.Join(project, "model", "record.go"), "package model\n")
			require.NoError(t, os.Symlink("missing.go", filepath.Join(project, "main.go")))
			dir, args := filepath.Dir(project), []string{"new", "example.com/sampleapp"}
			if tc.inPlace {
				dir, args = project, append(args, ".")
			}
			t.Chdir(dir)

			_, err := executeRootCommand(t, args...)

			require.EqualError(t, err, "sampleapp already holds what gg new creates: .gitignore, main.go, model; nothing was written")
			require.Equal(t, []string{".gitignore", "main.go", "model"}, dirNames(t, project))
			requireFileContent(t, filepath.Join(project, ".gitignore"), "keep\n")
		})
	}
}

// TestNewFillsAnExistingDirectoryWithoutConflicts pins that an existing
// project directory holding nothing the project gets, as a cloned repository
// holding only its README does, is filled like a new one: every file gg new
// writes lands in it, under the module the project was created with, and the
// README stays as it was.
func TestNewFillsAnExistingDirectoryWithoutConflicts(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	stubGoModTidy(t)
	project := filepath.Join(root, "sampleapp")
	writeProjectFile(t, filepath.Join(project, "README.md"), "# sampleapp\n")

	_, err := executeRootCommand(t, "new", "example.com/sampleapp")

	require.NoError(t, err)
	requireProjectFiles(t, project, "example.com/sampleapp")
	requireFileContent(t, filepath.Join(project, "README.md"), "# sampleapp\n")
}

// TestNewInPlaceFillsTheCurrentDirectory pins gg new <module-path> .: the
// project is generated into the current directory itself, named after the
// last element of the module path, "./" standing for "." alike, and what the
// directory held before stays as it was.
func TestNewInPlaceFillsTheCurrentDirectory(t *testing.T) {
	for _, tc := range []struct{ name, dot string }{
		{name: "dot", dot: "."},
		{name: "dot slash", dot: "./"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project := filepath.Join(t.TempDir(), "sampleapp")
			writeProjectFile(t, filepath.Join(project, "README.md"), "# sampleapp\n")
			t.Chdir(project)
			stubGoModTidy(t)

			_, err := executeRootCommand(t, "new", "example.com/sampleapp", tc.dot)

			require.NoError(t, err)
			requireProjectFiles(t, project, "example.com/sampleapp")
			requireFileContent(t, filepath.Join(project, "README.md"), "# sampleapp\n")
		})
	}
}

// TestNewInPlaceRequiresTheDirectoryNamedAfterTheModule pins that gg new
// <module-path> . generates only into a directory named after the last
// element of the module path, which stops it in a directory entered by
// mistake and on a mistyped module path; the directory is left as it was.
func TestNewInPlaceRequiresTheDirectoryNamedAfterTheModule(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "work")
	require.NoError(t, os.Mkdir(dir, 0o755))
	t.Chdir(dir)

	_, err := executeRootCommand(t, "new", "example.com/sampleapp", ".")

	require.EqualError(t, err, "the module path example.com/sampleapp names the project sampleapp, but the current directory is work: run gg new example.com/sampleapp . in a directory named sampleapp")
	require.Empty(t, dirNames(t, dir))
}

// TestNewRejectsADirectoryAsTheModulePath pins that gg new takes a module
// path first, never a directory: a relative or absolute directory there is
// reported with the form that generates into the current directory, before
// anything is created.
func TestNewRejectsADirectoryAsTheModulePath(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	require.NoError(t, os.Mkdir(work, 0o755))
	for _, tc := range []struct{ name, arg string }{
		{name: "current directory", arg: "."},
		{name: "subdirectory", arg: "./sampleapp"},
		{name: "sibling directory", arg: "../sampleapp"},
		{name: "absolute path", arg: filepath.Join(root, "sampleapp")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(work)

			_, err := executeRootCommand(t, "new", tc.arg)

			require.EqualError(t, err, fmt.Sprintf("%q is a directory, not a module path: to create the project in the current directory, run gg new <module-path> .", tc.arg))
			require.Equal(t, []string{"work"}, dirNames(t, root))
			require.Empty(t, dirNames(t, work))
		})
	}
}

// TestNewAcceptsOnlyADotAfterTheModulePath pins that the one argument gg new
// takes after the module path is ".", for the current directory: any other
// directory is reported before anything is created.
func TestNewAcceptsOnlyADotAfterTheModulePath(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	_, err := executeRootCommand(t, "new", "example.com/sampleapp", "sampleapp")

	require.EqualError(t, err, `gg new creates the project in a new directory, or with "." in the current one, not in sampleapp`)
	require.Empty(t, dirNames(t, root))
}

// TestNewInitializesGitOnlyOutsideARepository pins how gg new ends, in either
// form: a project created outside any Git repository gets one of its own,
// while one created inside a repository stays part of it instead of nesting a
// second one.
func TestNewInitializesGitOnlyOutsideARepository(t *testing.T) {
	for _, tc := range []struct {
		name         string
		inRepository bool
		inPlace      bool
	}{
		{name: "new directory outside a repository"},
		{name: "new directory inside a repository", inRepository: true},
		{name: "current directory outside a repository", inPlace: true},
		{name: "current directory inside a repository", inRepository: true, inPlace: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			// Git looks for a repository no further up than root, so none
			// enclosing the temporary directory can decide the case.
			t.Setenv("GIT_CEILING_DIRECTORIES", root)
			parent := filepath.Join(root, "work")
			require.NoError(t, os.Mkdir(parent, 0o755))
			if tc.inRepository {
				runGit(t, parent, "init")
			}
			project := filepath.Join(parent, "sampleapp")
			dir, args := parent, []string{"new", "example.com/sampleapp"}
			if tc.inPlace {
				require.NoError(t, os.Mkdir(project, 0o755))
				dir, args = project, append(args, ".")
			}
			t.Chdir(dir)
			stubGoModTidy(t)

			_, err = executeRootCommand(t, args...)

			require.NoError(t, err)
			repository := project
			if tc.inRepository {
				repository = parent
			}
			require.Equal(t, repository, runGit(t, project, "rev-parse", "--show-toplevel"))
		})
	}
}

// stubGoModTidy makes tidying succeed without running it for the rest of the
// test: it resolves the framework over the network, and these tests are about
// the files gg new writes.
func stubGoModTidy(t *testing.T) {
	t.Helper()

	tidy := goModTidy
	goModTidy = func() error { return nil }
	t.Cleanup(func() { goModTidy = tidy })
}

// requireProjectFiles fails the test unless dir holds the project gg new
// creates for projectName: go.mod declaring the module and every file
// pkgnew.ProjectFiles lists, with its content.
func requireProjectFiles(t *testing.T, dir, projectName string) {
	t.Helper()

	goMod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(goMod), "module "+projectName+"\n"), "go.mod does not declare module %s:\n%s", projectName, goMod)

	files, err := pkgnew.ProjectFiles(projectName)
	require.NoError(t, err)
	for _, file := range files {
		requireFileContent(t, filepath.Join(dir, file.Path), file.Content)
	}
}

// requireFileContent fails the test unless the file at path holds content.
func requireFileContent(t *testing.T, path, content string) {
	t.Helper()

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, content, string(got), "content of %s", path)
}

// dirNames returns the names of the entries of dir, sorted.
func dirNames(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// runGit runs git with args in dir and returns what it printed, trimmed.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	require.NoError(t, err, "git %s", strings.Join(args, " "))
	return strings.TrimSpace(string(out))
}
