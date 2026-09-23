package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/codegen"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/ggprune"
	"github.com/spf13/cobra"
)

// pruneCmd is gg prune; PRUNE.md next to this file lays out what it deletes,
// what it keeps and the order it goes in.
var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "clean unused service files",
	Long:  "Clean unused service files that are no longer needed based on current model definitions",
	Run: func(cmd *cobra.Command, args []string) {
		pruneRun()
	},
}

func pruneRun() {
	ignore := gghelper.NewProjectIgnore()
	if len(module) == 0 {
		var err error
		module, err = gghelper.ModulePath()
		checkErr(err)
	}

	if !gghelper.FileExists(ggconst.DirModel) {
		clioutput.Error("", "model dir not found: %s", ggconst.DirModel)
		os.Exit(1)
	}

	projectCfg, err := loadProjectConfig()
	checkErr(err)

	// Scan all models
	clioutput.Section("Scan Models")
	allModels, err := codegen.FindModels(module, ggconst.DirModel, ignore)
	checkErr(err)
	if len(allModels) == 0 {
		clioutput.Item("", "No models found, pruning service files only")
	} else {
		clioutput.Success("", "%d models found", len(allModels))
	}

	// Route ignores from gst.yaml are intentionally not applied here: an
	// ignored action keeps its service file on disk, so leaving the action
	// enabled makes prune treat the file as expected without extra bookkeeping.

	// Scan existing service files
	oldServiceFiles := existingServiceFiles(ignore)

	// Prune disabled service files
	clioutput.Section("Prune Disabled Service Files")
	pruneServiceFiles(oldServiceFiles, allModels, nil, nil, projectCfg.Prune, ignore)

	clioutput.Done("Code pruning completed successfully!")
}

// existingServiceFiles lists the service files prune works from, warning
// about a scan that ended early and going on with what it found.
func existingServiceFiles(ignore gghelper.ProjectIgnore) []string {
	files, err := ggprune.ScanServiceFiles(ggconst.DirService, ignore)
	if err != nil {
		clioutput.Warn("", "failed to scan existing service files: %v", err)
	}
	return files
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
		if ggconfig.IsLegacyPruneSettings(name) {
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

	plan := ggprune.PlanFiles(oldServiceFiles, allModels, keptFiles, protect)
	filesToDelete, ignoredFiles := plan.Delete, plan.Ignored

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
		removeEmptyServiceDirs(protect, ignore)
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
	ggprune.RemoveFiles(filesToDelete, reportRemoval)

	// Remove empty directories after deleting files
	removeEmptyServiceDirs(protect, ignore)
	handleOrphanServiceDirs(allModels, keptDirs, module, protect, ignore)
}

// reportRemoval prints how deleting one file went.
func reportRemoval(path string, err error) {
	if err != nil {
		clioutput.Error("", "Failed to delete %s: %v", path, err)
		return
	}
	clioutput.Success("", "Deleted %s", path)
}

// removeEmptyServiceDirs removes the empty directories below the service
// directory that prune.ignore does not cover, printing each one.
func removeEmptyServiceDirs(protect ggconfig.PruneConfig, ignore gghelper.ProjectIgnore) {
	ggprune.RemoveEmptyDirs(ggconst.DirService, protect, ignore, func(dir string) {
		clioutput.Success("", "Removed empty directory %s", dir)
	})
}

// handleOrphanServiceDirs reports or cleans service directories no model
// owns; see ggprune.FindOrphanDirs for the ownership rules. When the project
// cannot be read in full, it warns and leaves every directory alone.
func handleOrphanServiceDirs(allModels []*gen.ModelInfo, keptDirs map[string]bool, modulePath string, protect ggconfig.PruneConfig, ignore gghelper.ProjectIgnore) {
	orphans, keptHelpers, err := ggprune.FindOrphanDirs(allModels, keptDirs, modulePath, protect, ignore)
	if err != nil {
		clioutput.Warn("", "failed to trace which service directories live code imports, so orphan service directories are not checked: %v", err)
		return
	}
	reportKeptServiceHelperDirs(keptHelpers)
	if len(orphans) == 0 {
		return
	}

	if cleanOrphans {
		reportOrphanServiceDirs("Unmanaged Orphan Service Directories", orphans)
		remindUnreadPruneSettings()
		if !confirmCleanOrphanServiceDirs() {
			clioutput.Item("", "Orphan service directory cleanup canceled")
			return
		}
		cleanOrphanServiceDirs(orphans, protect, ignore)
		return
	}

	reportOrphanServiceDirs("Unmanaged Orphan Service Directories Kept", orphans)
}

func reportOrphanServiceDirs(section string, orphans []ggprune.OrphanDir) {
	clioutput.Section(section)
	for _, orphan := range orphans {
		clioutput.Item("", "%s (no current model maps to this directory)", orphan.Path)
		for _, file := range orphan.Files {
			clioutput.Line(clioutput.StyleMuted, "    - %s", file)
		}
	}
}

// reportKeptServiceHelperDirs explains why unmanaged helper directories
// survived orphan cleanup: live project code still imports them.
func reportKeptServiceHelperDirs(keptHelpers []ggprune.OrphanDir) {
	if len(keptHelpers) == 0 {
		return
	}
	clioutput.Section("Service Helper Directories Kept")
	for _, helper := range keptHelpers {
		clioutput.Item("", "%s (imported by live project code)", helper.Path)
	}
}

const cleanOrphansConfirmation = "delete orphan service leftovers"

func confirmCleanOrphanServiceDirs() bool {
	clioutput.Warn("", "This will delete unmanaged files that gg cannot prove it owns.")
	clioutput.Prompt("Type %q to continue: ", cleanOrphansConfirmation)

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil && len(response) == 0 {
		return false
	}
	return strings.TrimSpace(response) == cleanOrphansConfirmation
}

func cleanOrphanServiceDirs(orphans []ggprune.OrphanDir, protect ggconfig.PruneConfig, ignore gghelper.ProjectIgnore) {
	var files []string
	for _, orphan := range orphans {
		files = append(files, orphan.Files...)
	}
	ggprune.RemoveFiles(files, reportRemoval)
	removeEmptyServiceDirs(protect, ignore)
}
