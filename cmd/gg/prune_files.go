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
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

// scanExistingServiceFiles scans existing service files in the service directory.
// It includes standard phase filenames (e.g., create.go, list.go) and any other .go file
// that embeds service.Base[...] (per-action handlers), such as DSL Filename("x") outputs.
func scanExistingServiceFiles(serviceDir string, ignore gghelper.ProjectIgnore) []string {
	var files []string

	// Check if service directory exists
	if _, err := os.Stat(serviceDir); os.IsNotExist(err) {
		return files
	}

	validPhases := validServicePhaseFiles()

	// Walk through the service directory
	err := ignore.Walk(serviceDir, func(path string, info os.FileInfo) error {
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

// filterIgnoredFiles splits files into the ones prune may delete and the ones
// a gst.yaml prune.ignore entry in protect covers.
func filterIgnoredFiles(files []string, protect ggconfig.PruneConfig) (filtered []string, ignored []string) {
	for _, file := range files {
		if protect.Ignores(file) {
			ignored = append(ignored, file)
		} else {
			filtered = append(filtered, file)
		}
	}
	return filtered, ignored
}

// warnMissingPruneIgnore warns about the gst.yaml prune.ignore entries naming
// no file or directory, usually left behind by a rename; like the other
// gst.yaml warnings, it does not stop the run.
func warnMissingPruneIgnore(protect ggconfig.PruneConfig) {
	for _, entry := range protect.Ignore {
		if _, err := os.Stat(entry); os.IsNotExist(err) {
			clioutput.Warn("", "gst.yaml prune.ignore entry %q names no file or directory", entry)
		}
	}
}

// remindUnreadPruneSettings repeats, right before prune asks to delete, that
// an old .gg.yaml protects nothing: the warning printed when gst.yaml was
// read may have scrolled away by then.
func remindUnreadPruneSettings() {
	for _, name := range ggconfig.UnreadFiles(".") {
		if isLegacyPruneSettings(name) {
			clioutput.Warn("", "%s is not read, so the paths it lists are not protected here; move them into %s under prune.ignore", name, ggconfig.FileName)
		}
	}
}

// pruneServiceFiles prunes disabled service files. Files in keptFiles belong
// to gst.yaml-ignored actions: they no longer appear in the generated
// registrations but must stay on disk, so they are never deletion candidates.
// keptDirs protects their directories from orphan cleanup; both may be nil.
// The paths the gst.yaml prune.ignore entries in protect cover are never
// deleted: not as disabled files, not as orphans, not as empty directories.
func pruneServiceFiles(oldServiceFiles []string, allModels []*gen.ModelInfo, keptFiles, keptDirs map[string]bool, protect ggconfig.PruneConfig, ignore gghelper.ProjectIgnore) {
	warnMissingPruneIgnore(protect)

	// Get list of service files that should currently exist
	currentFiles := currentServiceFiles(allModels)

	// Find files to delete (exist in old list but not in current list)
	filesToDelete := make([]string, 0)
	for _, oldFile := range oldServiceFiles {
		if !currentFiles[oldFile] && !keptFiles[oldFile] {
			filesToDelete = append(filesToDelete, oldFile)
		}
	}

	// Keep the files gst.yaml prune.ignore protects
	filesToDelete, ignoredFiles := filterIgnoredFiles(filesToDelete, protect)

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
		removeEmptyDirectories(ggconst.DirService, protect, ignore)
		handleOrphanServiceDirs(allModels, keptDirs, module, protect, ignore)
		return
	}

	// Display list of files to be deleted
	clioutput.Section("Files To Be Deleted")
	for _, file := range filesToDelete {
		clioutput.Error("", "%s", file)
	}

	// Ask user for confirmation
	remindUnreadPruneSettings()
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
	removeEmptyDirectories(ggconst.DirService, protect, ignore)
	handleOrphanServiceDirs(allModels, keptDirs, module, protect, ignore)
}

func currentServiceFiles(allModels []*gen.ModelInfo) map[string]bool {
	current := make(map[string]bool)
	for _, m := range allModels {
		m.Design.Range(func(route string, act *dsl.Action) {
			if act.Enabled && act.Service {
				target := gen.ServiceTarget(m, act, ggconst.DirModel, ggconst.DirService)
				current[target.FilePath] = true
			}
		})
	}
	return current
}

// removeEmptyDirectories removes the empty directories below rootDir, deepest
// first, keeping those a gst.yaml prune.ignore entry in protect covers.
func removeEmptyDirectories(rootDir string, protect ggconfig.PruneConfig, ignore gghelper.ProjectIgnore) {
	dirs := make([]string, 0)
	_ = ignore.Walk(rootDir, func(path string, info os.FileInfo) error {
		if path == rootDir || !info.IsDir() || protect.Ignores(path) {
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
