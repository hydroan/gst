// Package gghelper holds what more than one gg command package reads about
// the project gg runs in, such as the module path its go.mod declares. Code a
// single command package uses stays in that package.
package gghelper

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/ggconst"
)

// ModulePath returns the module path the go.mod of the working directory
// declares: example.com/app for a go.mod starting with module example.com/app.
func ModulePath() (string, error) {
	content, err := os.ReadFile("go.mod")
	if err != nil {
		return "", fmt.Errorf("failed to read go.mod: %w", err)
	}

	lines := strings.SplitSeq(string(content), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module")), nil
		}
	}

	return "", errors.New("module name not found in go.mod")
}

// IsFrameworkProject reports whether the go.mod in projectDir declares the gst
// framework module itself, the repository gg commands refuse to run in.
func IsFrameworkProject(projectDir string) bool {
	content, err := os.ReadFile(filepath.Join(projectDir, "go.mod"))
	if err != nil {
		return false
	}

	lines := strings.SplitSeq(string(content), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			moduleName := strings.TrimSpace(strings.TrimPrefix(line, "module"))
			return moduleName == ggconst.ImportPathGst
		}
	}
	return false
}
