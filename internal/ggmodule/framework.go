package ggmodule

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
)

const frameworkModulePath = "github.com/hydroan/gst"

func findFrameworkRoot() (string, error) {
	candidates := []string{
		filepath.Join(".", "internal", "gst"),
		".",
	}
	for _, candidate := range candidates {
		if isFrameworkRoot(candidate) {
			return filepath.Clean(candidate), nil
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if isFrameworkRoot(wd) {
			return filepath.Clean(wd), nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	return "", errors.New("framework source not found; expected internal/gst/go.mod")
}

func isFrameworkRoot(candidate string) bool {
	modFile := filepath.Join(candidate, "go.mod")
	data, err := os.ReadFile(modFile)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "module "+frameworkModulePath)
}

func readProjectModulePath() (string, error) {
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
