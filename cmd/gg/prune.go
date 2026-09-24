package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/codegen"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/ggmodule"
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
		if err := pruneRun(); err != nil {
			clioutput.Error("", "%v", err)
			os.Exit(1)
		}
	},
}

// The pruning flags. gg gen takes both: --prune prunes once the code is
// generated. gg prune always prunes and takes --clean-orphans alone, which on
// either command also deletes the orphan leftovers.
var (
	prune        bool
	cleanOrphans bool
)

func init() {
	pruneCmd.Flags().BoolVar(&cleanOrphans, "clean-orphans", false, "Delete unmanaged files in orphan service directories and middleware left by removed copied modules")
}

// pruneRun prunes the project in the working directory. The errors it
// returns stop it before it deletes anything: the module path or gst.yaml
// could not be read, the model directory is missing, or a model file fails
// to parse.
func pruneRun() error {
	if len(module) == 0 {
		var err error
		module, err = gghelper.ModulePath()
		if err != nil {
			return err
		}
	}

	if !gghelper.FileExists(ggconst.DirModel) {
		return errors.Newf("model dir not found: %s", ggconst.DirModel)
	}

	projectCfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	// Scan all models the way gg gen does: which models the project declares
	// is gen's to say, and prune only works out what they leave behind.
	clioutput.Section("Scan Models")
	allModels, err := codegen.FindModels(module, ggconst.DirModel, gghelper.NewProjectIgnore())
	if err != nil {
		return err
	}
	if len(allModels) == 0 {
		clioutput.Item("", "No models found, pruning service files only")
	} else {
		clioutput.Success("", "%d models found", len(allModels))
	}

	// Route ignores from gst.yaml are intentionally not applied here: an
	// ignored action keeps its service file on disk, so leaving the action
	// enabled makes prune treat the file as expected without extra bookkeeping.

	// Scan existing service files
	oldServiceFiles := existingServiceFiles()

	// Prune disabled service files
	clioutput.Section("Prune Disabled Service Files")
	pruneServiceFiles(oldServiceFiles, allModels, nil, nil, projectCfg.Prune)

	clioutput.Done("Code pruning completed successfully!")
	return nil
}

// existingServiceFiles lists the service files prune works from, warning
// about a scan that ended early and going on with what it found.
func existingServiceFiles() []string {
	files, err := ggprune.ScanServiceFiles(ggconst.DirService)
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
// Of the ignore rules, prune follows these alone: the project's Git ignore
// rules and the go command's ignores keep nothing (see package ggprune).
func pruneServiceFiles(oldServiceFiles []string, allModels []*gen.ModelInfo, keptFiles, keptDirs map[string]bool, protect ggconfig.PruneConfig) {
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
		removeEmptyServiceDirs(protect)
		handleOrphans(allModels, keptDirs, module, protect)
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
	removeEmptyServiceDirs(protect)
	handleOrphans(allModels, keptDirs, module, protect)
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
func removeEmptyServiceDirs(protect ggconfig.PruneConfig) {
	ggprune.RemoveEmptyDirs(ggconst.DirService, protect, func(dir string) {
		clioutput.Success("", "Removed empty directory %s", dir)
	})
}

// handleOrphans reports or cleans what the project left behind: the service
// directories no model owns, see ggprune.FindOrphanDirs for the ownership
// rules, and the middleware module copy wrote for modules the project removed,
// see orphanModuleMiddleware. When the project cannot be read in full, it
// warns and leaves everything alone.
func handleOrphans(allModels []*gen.ModelInfo, keptDirs map[string]bool, modulePath string, protect ggconfig.PruneConfig) {
	orphanMiddleware, err := orphanModuleMiddleware(protect)
	if err != nil {
		clioutput.Warn("", "failed to read the middleware directory, so orphans are not checked: %v", err)
		return
	}
	orphanFiles := make([]string, 0, len(orphanMiddleware))
	for _, file := range orphanMiddleware {
		orphanFiles = append(orphanFiles, file.Path)
	}
	orphans, keptHelpers, err := ggprune.FindOrphanDirs(allModels, keptDirs, orphanFiles, modulePath, protect)
	if err != nil {
		clioutput.Warn("", "failed to trace which service directories live code imports, so orphan service directories are not checked: %v", err)
		return
	}
	reportKeptServiceHelperDirs(keptHelpers)
	if len(orphans) == 0 && len(orphanMiddleware) == 0 {
		return
	}

	if cleanOrphans {
		reportOrphanServiceDirs("Unmanaged Orphan Service Directories", orphans)
		reportOrphanMiddleware("Orphan Module Middleware Files", orphanMiddleware)
		remindUnreadPruneSettings()
		// Orphan middleware carries the ownership marker module copy wrote;
		// only an orphan directory holds files gg cannot vouch for.
		if len(orphans) > 0 {
			clioutput.Warn("", "This will delete unmanaged files that gg cannot prove it owns.")
		}
		if !confirmCleanOrphans() {
			clioutput.Item("", "Orphan cleanup canceled")
			return
		}
		// The middleware goes first: the directories it imports are orphans
		// only because it is deleted, so they stay if it cannot be.
		if err := cleanOrphanMiddleware(orphanFiles); err != nil {
			clioutput.Error("", "Failed to delete orphan module middleware, so orphan service directories are kept: %v", err)
			return
		}
		cleanOrphanServiceDirs(orphans, protect)
		return
	}

	reportOrphanServiceDirs("Unmanaged Orphan Service Directories Kept", orphans)
	reportOrphanMiddleware("Orphan Module Middleware Files Kept", orphanMiddleware)
}

// orphanModuleMiddleware lists the middleware module copy wrote for modules the
// project removed (see ggmodule.OrphanMiddlewareFiles), but for the ones a
// gst.yaml prune.ignore entry in protect covers, which stay live code; no
// other rule keeps a file, a Git ignored one included.
func orphanModuleMiddleware(protect ggconfig.PruneConfig) ([]ggmodule.OrphanMiddleware, error) {
	orphans, err := ggmodule.OrphanMiddlewareFiles(ggconst.DirMiddleware, ggconst.DirModel)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(orphans, func(orphan ggmodule.OrphanMiddleware) bool {
		return protect.Ignores(orphan.Path)
	}), nil
}

// reportOrphanMiddleware lists the orphan middleware files under section; each
// goes together with its register calls.
func reportOrphanMiddleware(section string, orphans []ggmodule.OrphanMiddleware) {
	if len(orphans) == 0 {
		return
	}
	clioutput.Section(section)
	for _, orphan := range orphans {
		clioutput.Item("", "%s (copied with module %s, whose %s is gone; its register calls go with it)", orphan.Path, orphan.Module, filepath.Join(ggconst.DirModel, orphan.Module))
	}
}

// cleanOrphanMiddleware deletes the orphan middleware files and their register
// calls, printing each file it changes.
func cleanOrphanMiddleware(files []string) error {
	return ggmodule.RemoveMiddlewareFiles(ggconst.DirMiddleware, files, func(status ggmodule.CopyWriteStatus, path string) {
		if status == ggmodule.CopyWriteDelete {
			clioutput.Success("", "Deleted %s", path)
			return
		}
		clioutput.Success("", "Removed their register calls from %s", path)
	})
}

// reportOrphanServiceDirs lists the orphan service directories under section,
// each with the unmanaged files cleaning it deletes.
func reportOrphanServiceDirs(section string, orphans []ggprune.OrphanDir) {
	if len(orphans) == 0 {
		return
	}
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

// cleanOrphansConfirmation is the phrase --clean-orphans has the user type
// before it deletes anything.
const cleanOrphansConfirmation = "delete orphan leftovers"

// confirmCleanOrphans asks the user to type cleanOrphansConfirmation and
// reports whether they did.
func confirmCleanOrphans() bool {
	clioutput.Prompt("Type %q to continue: ", cleanOrphansConfirmation)

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil && len(response) == 0 {
		return false
	}
	return strings.TrimSpace(response) == cleanOrphansConfirmation
}

// cleanOrphanServiceDirs deletes the unmanaged files of the orphan service
// directories, and then the directories this leaves empty.
func cleanOrphanServiceDirs(orphans []ggprune.OrphanDir, protect ggconfig.PruneConfig) {
	var files []string
	for _, orphan := range orphans {
		files = append(files, orphan.Files...)
	}
	ggprune.RemoveFiles(files, reportRemoval)
	removeEmptyServiceDirs(protect)
}
