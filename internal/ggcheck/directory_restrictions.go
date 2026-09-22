package ggcheck

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gitignore "github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/hydroan/gst/internal/gghelper"
)

// DirectoryRestrictions limits the top-level directories to the ones the
// framework layout names and the ones holding no Go package.
var DirectoryRestrictions = Check{
	Name: "Directory restrictions",
	Rule: "top-level directories must be ones the framework layout names, such as model, service, router and dao, or hold no Go packages, such as logs and deploy",
	run:  checkDirectoryRestrictions,
}

// checkDirectoryRestrictions checks if only allowed directories exist in the project
func checkDirectoryRestrictions(ignore gitignore.Matcher) []string {
	projectDir := "."
	var violations []string

	// Check if this is a gst framework project by reading go.mod
	if gghelper.IsFrameworkProject(projectDir) {
		// Skip directory restriction check for gst framework itself
		return violations
	}

	// Check if this project uses gst framework
	if !usesGstFramework(projectDir) {
		// Skip directory restriction check for projects not using gst framework
		return violations
	}

	// Define allowed directories for gst framework projects
	allowedDirs := map[string]bool{
		"model":      true,
		"module":     true,
		"service":    true,
		"router":     true,
		"dao":        true,
		"provider":   true,
		"middleware": true,
		"cronjob":    true,
		"leader":     true,
		"lock":       true,
		"component":  true,
		"configx":    true,
		"config":     true,
		"typesx":     true,
		"consts":     true,
		"constx":     true,
		"type":       true,
		"typex":      true,
		"helper":     true,
		"internal":   true,
		"cmd":        true,
		"errorx":     true,
		"testcode":   true,
		"testdata":   true,
		"test":       true,
		"docs":       true,
		"doc":        true,
	}

	// Directories that hold no Go packages: build and log output, and the
	// conventional homes of deployment manifests and Helm charts, operator
	// scripts and development scripts.
	whitelistDirs := map[string]bool{
		"tmp":       true,
		"logs":      true,
		"dist":      true,
		"generated": true,
		"deploy":    true,
		"charts":    true,
		"scripts":   true,
		"hack":      true,
	}

	// Read directory contents
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return violations
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()

		// Skip hidden directories and common project files
		if strings.HasPrefix(dirName, ".") {
			continue
		}
		if isIgnoredProjectPath(ignore, dirName, true) {
			continue
		}

		// Check if directory is allowed
		if !allowedDirs[dirName] && !whitelistDirs[dirName] {
			violations = append(violations, fmt.Sprintf("Directory '%s' is not allowed in project structure", dirName))
		}
	}

	return violations
}

// usesGstFramework checks if the project uses gst framework as a dependency
func usesGstFramework(projectDir string) bool {
	goModPath := filepath.Join(projectDir, "go.mod")
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return false
	}

	// Check if github.com/hydroan/gst is in dependencies
	return strings.Contains(string(content), "github.com/hydroan/gst")
}
