// Package gghelper holds what more than one of gg's packages needs about the
// project gg runs in: the module path its go.mod declares, the paths gg leaves
// out of it, the ones its Git ignore rules exclude and the ones the go command
// leaves out, programs and package listings run against its module, and the
// small file and path helpers the commands share. It also holds the printing
// path of the Go files gg prints itself (see FormatFile). Code a single
// package uses stays in that package.
package gghelper

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/ggconst"
	"golang.org/x/mod/modfile"
)

// ModulePath returns the module path the go.mod in dir declares:
// example.com/app for a go.mod starting with module example.com/app, and for
// one whose module directive carries the deprecation comment go documents or
// a quoted path, both of which go itself accepts. gg runs in the project, so
// its commands pass ".".
//
// A dir holding no go.mod is not a project, which the error says with what
// creates one: "no go.mod in the current directory: gg runs at the root of
// a gst project, which gg new creates".
func ModulePath(dir string) (string, error) {
	content, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if errors.Is(err, fs.ErrNotExist) {
		where := dir
		if dir == "." {
			where = "the current directory"
		}
		return "", errors.Newf("no go.mod in %s: gg runs at the root of a gst project, which gg new creates", where)
	}
	if err != nil {
		return "", errors.Wrap(err, "failed to read go.mod")
	}

	modulePath := modfile.ModulePath(content)
	if modulePath == "" {
		return "", errors.New("module name not found in go.mod")
	}
	return modulePath, nil
}

// InWorkspace reports whether the go command runs in workspace mode in the
// working directory: a go.work file above it, or the one GOWORK names, puts
// it there, and GOWORK=off takes it out. The answer is read from go env
// GOWORK rather than by looking for the file, so it is the go command's own,
// whichever way the workspace was named.
func InWorkspace() (bool, error) {
	out, err := exec.Command("go", "env", "GOWORK").Output()
	if err != nil {
		return false, errors.Wrap(err, "failed to read go env GOWORK")
	}
	workspace := strings.TrimSpace(string(out))
	return workspace != "" && workspace != "off", nil
}

// IsFrameworkProject reports whether the go.mod in projectDir declares the gst
// framework module itself, the repository gg commands refuse to run in.
func IsFrameworkProject(projectDir string) bool {
	content, err := os.ReadFile(filepath.Join(projectDir, "go.mod"))
	if err != nil {
		return false
	}
	return modfile.ModulePath(content) == ggconst.ImportPathGst
}

// RequiresFramework reports whether the go.mod in projectDir requires the gst
// framework module. "require github.com/hydroan/gst v1.0.0" does; a comment
// naming the framework, a module path extending it such as
// github.com/hydroan/gst-demo, and a requirement on such a module do not. A
// project without go.mod requires nothing. A directive newer than this build
// knows is skipped, so a newer Go release cannot break gg here; a statement it
// knows but cannot read is an error.
func RequiresFramework(projectDir string) (bool, error) {
	version, _, err := FrameworkRequirement(projectDir)
	return version != "", err
}

// FrameworkRequirement returns the version of the gst framework module the
// go.mod in projectDir requires, read the way RequiresFramework reads it and
// "" where that reports false, and whether the go.mod replaces the
// framework module: the project then builds with what the replacement
// names, a local copy as a rule, whatever version the requirement states.
// For a go.mod holding
//
//	require github.com/hydroan/gst v1.2.0
//	replace github.com/hydroan/gst => ../gst
//
// it returns v1.2.0 and true.
func FrameworkRequirement(projectDir string) (version string, replaced bool, err error) {
	path := filepath.Join(projectDir, "go.mod")
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, errors.Wrap(err, "failed to read go.mod")
	}
	file, err := modfile.ParseLax(path, content, nil)
	if err != nil {
		return "", false, errors.Wrap(err, "failed to parse go.mod")
	}
	for _, require := range file.Require {
		if require.Mod.Path == ggconst.ImportPathGst {
			version = require.Mod.Version
		}
	}
	// The lax parse reads no replace directive, which only the main module's
	// go.mod carries, so the directives are read off the syntax it keeps: a
	// line "replace path ..." or a line "path ..." of a replace block, the
	// path written bare or quoted.
	replaces := func(tokens []string) bool {
		return len(tokens) > 0 && strings.Trim(tokens[0], "\"`") == ggconst.ImportPathGst
	}
	for _, stmt := range file.Syntax.Stmt {
		switch stmt := stmt.(type) {
		case *modfile.Line:
			replaced = replaced || len(stmt.Token) > 1 && stmt.Token[0] == "replace" && replaces(stmt.Token[1:])
		case *modfile.LineBlock:
			if len(stmt.Token) == 1 && stmt.Token[0] == "replace" {
				replaced = replaced || slices.ContainsFunc(stmt.Line, func(line *modfile.Line) bool { return replaces(line.Token) })
			}
		}
	}
	return version, replaced, nil
}
