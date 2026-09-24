package main

import (
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
// what it keeps and the order it goes in. It deletes from service/ and
// middleware/ alone.
var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "clean what the models no longer need from service/ and middleware/",
	Long: "Clean what the current models no longer need, asking once before deleting: the service files of disabled actions with their test files, " +
		"the unmanaged files of service directories no model owns, the middleware of removed copied modules with their register calls, " +
		"and the directories this leaves empty. It touches service/ and middleware/ only, and gst.yaml's prune.ignore is the one way to keep a path there.",
	Run: func(cmd *cobra.Command, args []string) {
		if err := pruneRun(); err != nil {
			clioutput.Error("", "%v", err)
			os.Exit(1)
		}
	},
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
	ignore := gghelper.NewProjectIgnore()
	allModels, err := codegen.FindModels(module, ggconst.DirModel, ignore)
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

	pruneLeftovers(oldServiceFiles, allModels, nil, nil, ignore, projectCfg.Prune)

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

// pruneLeftovers deletes, after asking once, what the models leave behind in
// service/ and middleware/: the service files of disabled actions among
// oldServiceFiles, the unmanaged files of the service directories no model
// owns, the middleware module copy wrote for modules the project removed, with
// their register calls, and the directories all this leaves empty. It works
// everything out before it asks, so a helper directory only a disabled
// action's service file imports goes in the same run. Files in keptFiles
// belong to gst.yaml-ignored actions: they no longer appear in the generated
// registrations but must stay on disk, so they are never deletion candidates.
// keptDirs protects their directories from orphan cleanup; both may be nil.
// ignore, the project's ignore rules, decides which code counts as still
// using a service directory, as it decides what gg check and gg gen read; it
// protects nothing. The paths the gst.yaml prune.ignore entries in protect
// cover are never deleted: not as disabled files, not as orphans, not as
// empty directories. Of the ignore rules, only those keep a path (see package
// ggprune).
func pruneLeftovers(oldServiceFiles []string, allModels []*gen.ModelInfo, keptFiles, keptDirs map[string]bool, ignore gghelper.ProjectIgnore, protect ggconfig.PruneConfig) {
	clioutput.Section("Prune Leftovers")
	warnMissingPruneIgnore(protect)

	plan := ggprune.PlanFiles(oldServiceFiles, allModels, keptFiles, protect)
	orphans, keptHelpers, orphanMiddleware := findOrphans(allModels, keptDirs, plan.Delete, ignore, protect)
	nothingToDelete := len(plan.Delete) == 0 && len(orphans) == 0 && len(orphanMiddleware) == 0
	if nothingToDelete {
		clioutput.Success("", "Nothing to prune")
		removeEmptyServiceDirs(protect)
	}
	if len(plan.Ignored) > 0 {
		clioutput.Section("Files Ignored By Config")
		for _, file := range plan.Ignored {
			clioutput.Item("", "ignore %s", file)
		}
	}
	reportKeptServiceHelperDirs(keptHelpers)
	if nothingToDelete {
		return
	}

	if len(plan.Delete) > 0 {
		clioutput.Section("Disabled Service Files")
		for _, file := range plan.Delete {
			clioutput.Error("", "%s", file)
		}
	}
	reportOrphanServiceDirs(orphans)
	reportOrphanMiddleware(orphanMiddleware)
	remindUnreadPruneSettings()
	// Orphan middleware carries the ownership marker module copy wrote; only
	// an orphan directory holds files gg cannot vouch for.
	if len(orphans) > 0 {
		clioutput.Warn("", "This will delete unmanaged files that gg cannot prove it owns.")
	}
	clioutput.Prompt("Do you want to delete these files? (y/N): ")
	var response string
	_, _ = fmt.Scanln(&response)
	response = strings.ToLower(strings.TrimSpace(response))
	if response != "y" && response != "yes" {
		clioutput.Item("", "Deletion canceled")
		return
	}

	deleteLeftovers(plan.Delete, orphans, orphanMiddleware, protect)
}

// findOrphans works out the orphans prune deletes along with deleting, the
// service files it deletes anyway: the service directories no model owns,
// see ggprune.FindOrphanDirs for the ownership rules, and the middleware
// module copy wrote for modules the project removed, see
// orphanModuleMiddleware. It returns as well the helper directories kept
// because live code imports them. When the project's code cannot be read in
// full, it warns and finds no orphans.
func findOrphans(allModels []*gen.ModelInfo, keptDirs map[string]bool, deleting []string, ignore gghelper.ProjectIgnore, protect ggconfig.PruneConfig) (orphans, keptHelpers []ggprune.OrphanDir, orphanMiddleware []ggmodule.OrphanMiddleware) {
	orphanMiddleware, err := orphanModuleMiddleware(protect)
	if err != nil {
		clioutput.Warn("", "failed to read the middleware directory, so orphans are not checked: %v", err)
		return nil, nil, nil
	}
	deleting = slices.Clone(deleting)
	for _, file := range orphanMiddleware {
		deleting = append(deleting, file.Path)
	}
	orphans, keptHelpers, err = ggprune.FindOrphanDirs(allModels, keptDirs, deleting, module, ignore, protect)
	if err != nil {
		clioutput.Warn("", "failed to trace which service directories live code imports, so orphan service directories are not checked: %v", err)
		return nil, nil, nil
	}
	return orphans, keptHelpers, orphanMiddleware
}

// deleteLeftovers deletes what pruneLeftovers listed, in this order: the
// disabled service files, the orphan middleware with its register calls, the
// unmanaged files of the orphan service directories, and last the
// directories this leaves empty. The orphan directories are orphans because
// the files before them go, so when one of those cannot be deleted they stay.
func deleteLeftovers(disabledFiles []string, orphans []ggprune.OrphanDir, orphanMiddleware []ggmodule.OrphanMiddleware, protect ggconfig.PruneConfig) {
	disabledKept := false
	ggprune.RemoveFiles(disabledFiles, func(path string, err error) {
		disabledKept = disabledKept || err != nil
		reportRemoval(path, err)
	})
	if len(orphanMiddleware) > 0 {
		files := make([]string, 0, len(orphanMiddleware))
		for _, file := range orphanMiddleware {
			files = append(files, file.Path)
		}
		if err := cleanOrphanMiddleware(files); err != nil {
			clioutput.Error("", "Failed to delete orphan module middleware, so orphan service directories are kept: %v", err)
			orphans = nil
		}
	}
	if disabledKept && len(orphans) > 0 {
		clioutput.Warn("", "Some disabled service files were not deleted, so orphan service directories are kept")
		orphans = nil
	}
	var orphanFiles []string
	for _, orphan := range orphans {
		orphanFiles = append(orphanFiles, orphan.Files...)
	}
	ggprune.RemoveFiles(orphanFiles, reportRemoval)
	removeEmptyServiceDirs(protect)
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

// reportOrphanMiddleware lists the orphan middleware files; each goes together
// with its register calls.
func reportOrphanMiddleware(orphans []ggmodule.OrphanMiddleware) {
	if len(orphans) == 0 {
		return
	}
	clioutput.Section("Orphan Module Middleware Files")
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

// reportOrphanServiceDirs lists the orphan service directories, each with the
// unmanaged files cleaning it deletes.
func reportOrphanServiceDirs(orphans []ggprune.OrphanDir) {
	if len(orphans) == 0 {
		return
	}
	clioutput.Section("Unmanaged Orphan Service Directories")
	for _, orphan := range orphans {
		clioutput.Item("", "%s (no current model maps to this directory)", orphan.Path)
		for _, file := range orphan.Files {
			clioutput.Line(clioutput.StyleMuted, "    - %s", file)
		}
	}
}

// reportKeptServiceHelperDirs explains why unmanaged helper directories are
// no orphans: live project code still imports them.
func reportKeptServiceHelperDirs(keptHelpers []ggprune.OrphanDir) {
	if len(keptHelpers) == 0 {
		return
	}
	clioutput.Section("Service Helper Directories Kept")
	for _, helper := range keptHelpers {
		clioutput.Item("", "%s (imported by live project code)", helper.Path)
	}
}
