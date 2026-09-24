// Package gghelper holds what more than one of gg's packages needs about the
// project gg runs in: the module path its go.mod declares, the paths gg leaves
// out of it, the ones its Git ignore rules exclude and the ones the go command
// leaves out, programs and package listings run against its module, and the
// small file and path helpers the commands share. Code a single package uses
// stays in that package.
package gghelper

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/ggconst"
	"golang.org/x/mod/modfile"
)

// ModulePath returns the module path the go.mod of the working directory
// declares: example.com/app for a go.mod starting with module example.com/app,
// and for one whose module directive carries the deprecation comment go
// documents or a quoted path, both of which go itself accepts.
func ModulePath() (string, error) {
	content, err := os.ReadFile("go.mod")
	if err != nil {
		return "", errors.Wrap(err, "failed to read go.mod")
	}

	modulePath := modfile.ModulePath(content)
	if modulePath == "" {
		return "", errors.New("module name not found in go.mod")
	}
	return modulePath, nil
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
	path := filepath.Join(projectDir, "go.mod")
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Wrap(err, "failed to read go.mod")
	}
	file, err := modfile.ParseLax(path, content, nil)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse go.mod")
	}
	return slices.ContainsFunc(file.Require, func(require *modfile.Require) bool {
		return require.Mod.Path == ggconst.ImportPathGst
	}), nil
}
