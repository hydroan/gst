package main

import (
	"bufio"
	"os"
	"sort"
	"strings"

	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/ggmodule"
	"github.com/spf13/cobra"
)

type moduleCopyOptions struct {
	Force bool
	Yes   bool
}

var moduleCopyOpts moduleCopyOptions

var moduleCmd = &cobra.Command{
	Use:   "module",
	Short: "manage gst modules",
}

var moduleCopyCmd = &cobra.Command{
	Use:   "copy <name>",
	Short: "copy a framework module into the current project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runModuleCopy(args[0], moduleCopyOpts)
	},
}

func init() {
	moduleCopyCmd.Flags().BoolVar(&moduleCopyOpts.Force, "force", false, "overwrite copied module files when they differ")
	moduleCopyCmd.Flags().BoolVar(&moduleCopyOpts.Yes, "yes", false, "copy without prompting for confirmation")

	moduleCmd.AddCommand(moduleCopyCmd)
}

// runModuleCopy owns the command-level workflow only: build a checked plan,
// show the current-project files that will be touched, confirm, then execute.
// Source analysis and AST rewriting live in internal/ggmodule.
func runModuleCopy(name string, opts moduleCopyOptions) error {
	copyOpts := ggmodule.CopyOptions{Force: opts.Force}
	plan, err := ggmodule.BuildCopyPlan(name, copyOpts)
	if err != nil {
		return err
	}

	printModuleCopyPlan(plan)
	if !opts.Yes && !confirmModuleCopy(name) {
		clioutput.Item("", "Module copy canceled")
		return nil
	}

	exec := ggmodule.CopyExecution{Plan: plan, Options: copyOpts, RunGen: runModuleCopyGen}
	if err := exec.Run(); err != nil {
		if len(exec.WrittenFiles) > 0 {
			printModuleCopyCleanup(name)
		}
		return err
	}

	clioutput.Section("Done")
	clioutput.Done("Module copied successfully")
	printModuleCopyCleanup(name)
	return nil
}

// runModuleCopyGen reuses gg gen's generator path but suppresses generated-file
// logs so module copy output stays focused on local-source files.
func runModuleCopyGen() error {
	// Module copy needs gg gen to create the target service shell, but it must not
	// turn this copy operation into a prune/cleanup pass over user service files.
	oldPrune := prune
	oldCleanOrphans := cleanOrphans
	prune = false
	cleanOrphans = false
	defer func() {
		prune = oldPrune
		cleanOrphans = oldCleanOrphans
	}()

	return genRunWithOptions(genRunOptions{Quiet: true})
}

func confirmModuleCopy(name string) bool {
	clioutput.Prompt("Copy module %s into the current project? (y/N): ", name)

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil && len(response) == 0 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(response)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

func printModuleCopyPlan(plan *ggmodule.CopyPlan) {
	clioutput.Section("Module Copy Plan")
	printModuleCopyPlanGroup("Model files", plan.ModelTargets())
	printModuleCopyPlanGroup("Service files", plan.ServiceTargets())
	printModuleCopyPlanGroup("Helper files", plan.HelperTargets())
}

func printModuleCopyPlanGroup(title string, files []string) {
	if len(files) == 0 {
		return
	}
	sort.Strings(files)
	clioutput.Item("", "%s:", title)
	for _, file := range files {
		clioutput.Item("", "%s", file)
	}
}

func printModuleCopyCleanup(name string) {
	clioutput.Item("", "To remove copied module code, delete model/%s, then run: gg prune --clean-orphans", name)
}
