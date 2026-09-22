// Package gghelper holds what more than one gg command package needs about
// the project gg runs in: the module path its go.mod declares, and the small
// file and path helpers the commands share. Code a single command package
// uses stays in that package.
package gghelper

import (
	"fmt"
	"os"
	"path/filepath"

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
		return "", fmt.Errorf("failed to read go.mod: %w", err)
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
