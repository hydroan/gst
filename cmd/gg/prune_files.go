package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/codegen/gen"
)

// scanExistingServiceFiles scans existing service files in the service directory.
// It includes standard phase filenames (e.g., create.go, list.go) and any other .go file
// that embeds service.Base[...] (per-action handlers), such as DSL Filename("x") outputs.
func scanExistingServiceFiles(serviceDir string) []string {
	var files []string

	// Check if service directory exists
	if _, err := os.Stat(serviceDir); os.IsNotExist(err) {
		return files
	}

	validPhases := validServicePhaseFiles()

	// Walk through the service directory
	err := filepath.Walk(serviceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			fileName := filepath.Base(path)
			if strings.HasSuffix(fileName, "_test.go") {
				return nil
			}
			if validPhases[fileName] {
				files = append(files, path)
				return nil
			}
			if gen.IsActionServiceSource(path) {
				files = append(files, path)
			}
		}
		return nil
	})
	if err != nil {
		clioutput.Warn("", "failed to scan existing service files: %v", err)
	}
	return files
}

// filterIgnoredFiles filters out files that match any ignore pattern.
// Supports both string matching (contains) and regex matching.
// Returns filtered files and ignored files.
func filterIgnoredFiles(files []string, ignorePatterns []string) (filtered []string, ignored []string) {
	if len(ignorePatterns) == 0 {
		return files, []string{}
	}

	for _, file := range files {
		shouldIgnore := false

		for _, pattern := range ignorePatterns {
			if matchesPrunePattern(file, pattern) {
				shouldIgnore = true
				break
			}
		}

		if shouldIgnore {
			ignored = append(ignored, file)
		} else {
			filtered = append(filtered, file)
		}
	}

	return filtered, ignored
}

// pruneServiceFiles prunes disabled service files.
func pruneServiceFiles(oldServiceFiles []string, allModels []*gen.ModelInfo) {
	// Get list of service files that should currently exist
	currentFiles := currentServiceFiles(allModels)

	// Find files to delete (exist in old list but not in current list)
	filesToDelete := make([]string, 0)
	for _, oldFile := range oldServiceFiles {
		if !currentFiles[oldFile] {
			filesToDelete = append(filesToDelete, oldFile)
		}
	}

	// Apply ignore patterns from config
	ignorePatterns := getPruneIgnorePatterns()
	var ignoredFiles []string
	if len(ignorePatterns) > 0 {
		filesToDelete, ignoredFiles = filterIgnoredFiles(filesToDelete, ignorePatterns)
	}

	// Display ignored files if any
	if len(ignoredFiles) > 0 {
		clioutput.Section("Files Ignored By Config")
		for _, file := range ignoredFiles {
			clioutput.Item("", "ignore %s", file)
		}
	}

	if len(filesToDelete) == 0 {
		if len(ignoredFiles) > 0 {
			clioutput.Success("", "No disabled service files to prune (all files are ignored)")
		} else {
			clioutput.Success("", "No disabled service files to prune")
		}
		// Still check for empty directories even if no files to delete
		removeEmptyDirectories(serviceDir)
		handleOrphanServiceDirs(allModels)
		return
	}

	// Display list of files to be deleted
	clioutput.Section("Files To Be Deleted")
	for _, file := range filesToDelete {
		clioutput.Error("", "%s", file)
	}

	// Ask user for confirmation
	clioutput.Prompt("Do you want to delete these files? (y/N): ")
	var response string
	_, _ = fmt.Scanln(&response)

	response = strings.ToLower(strings.TrimSpace(response))
	if response != "y" && response != "yes" {
		clioutput.Item("", "Deletion canceled")
		return
	}

	// Execute deletion operation
	for _, file := range filesToDelete {
		if err := os.Remove(file); err != nil {
			clioutput.Error("", "Failed to delete %s: %v", file, err)
		} else {
			clioutput.Success("", "Deleted %s", file)
		}
	}

	// Remove empty directories after deleting files
	removeEmptyDirectories(serviceDir)
	handleOrphanServiceDirs(allModels)
}

func currentServiceFiles(allModels []*gen.ModelInfo) map[string]bool {
	current := make(map[string]bool)
	for _, m := range allModels {
		m.Design.Range(func(route string, act *dsl.Action) {
			if act.Enabled && act.Service {
				target := gen.ServiceTarget(m, act, modelDir, serviceDir)
				current[target.FilePath] = true
			}
		})
	}
	return current
}

// removeEmptyDirectories removes empty child directories below the given root directory.
func removeEmptyDirectories(rootDir string) {
	dirs := make([]string, 0)
	_ = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			//nolint:nilerr
			return nil // Continue walking even if there's an error
		}

		if path == rootDir || !info.IsDir() {
			return nil
		}

		dirs = append(dirs, path)
		return nil
	})

	sort.Slice(dirs, func(i, j int) bool {
		return directoryDepth(rootDir, dirs[i]) > directoryDepth(rootDir, dirs[j])
	})

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		if len(entries) == 0 {
			// #nosec G122 -- path is under known project root (rootDir); we only remove empty dirs in codegen
			if err := os.Remove(dir); err == nil {
				clioutput.Success("", "Removed empty directory %s", dir)
			}
		}
	}
}

func directoryDepth(rootDir, path string) int {
	rel, err := filepath.Rel(rootDir, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}
