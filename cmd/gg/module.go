package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
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

var moduleListCmd = &cobra.Command{
	Use:   "list",
	Short: "list framework modules",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runModuleList(cmd)
	},
}

var moduleAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "register a framework module in the current project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runModuleAdd(args[0])
	},
}

var moduleRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "unregister a framework module from the current project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runModuleRemove(args[0])
	},
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

	moduleCmd.AddCommand(moduleListCmd, moduleAddCmd, moduleRemoveCmd, moduleCopyCmd)
}

func runModuleList(cmd *cobra.Command) error {
	modules, err := ggmodule.ListModules()
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if _, writeErr := fmt.Fprintf(w, "\n%s %s\n\n", clioutput.Text(clioutput.StyleInfo, "%s", clioutput.SymbolSection), clioutput.Text(clioutput.StyleBold, "Framework Modules")); writeErr != nil {
		return writeErr
	}
	rows := make([][]string, 0, len(modules))
	for _, module := range modules {
		addable := "yes"
		if !module.Addable {
			addable = "no"
		}
		copyable := "yes"
		if !module.Copyable {
			copyable = "no"
		}
		rows = append(rows, []string{module.Name, module.PackageName, addable, copyable, module.ImportPath})
	}
	tableWriter := table.New().
		Border(lipgloss.NormalBorder()).
		StyleFunc(func(_, _ int) lipgloss.Style {
			return lipgloss.NewStyle().Padding(0, 1)
		}).
		Headers("NAME", "PACKAGE", "ADD", "COPY", "IMPORT").
		Rows(rows...)
	_, err = fmt.Fprintln(w, tableWriter)
	return err
}

func runModuleAdd(name string) error {
	result, err := ggmodule.AddModule(".", name)
	if err != nil {
		return err
	}

	switch result.Status {
	case ggmodule.ChangeSkipped:
		clioutput.Item("SKIP", "%s already registered in %s", result.Module.Name, result.Path)
	default:
		clioutput.Success("ADD", "%s registered in %s", result.Module.Name, result.Path)
	}
	return nil
}

func runModuleRemove(name string) error {
	result, err := ggmodule.RemoveModule(".", name)
	if err != nil {
		return err
	}

	clioutput.Success("REMOVE", "%s unregistered from %s", result.Module.Name, result.Path)
	return nil
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
	printModuleCopyExtraModelReminder(plan)
	printModuleCopyExtraServiceReminder(plan)
	printModuleCopyPostNotes(plan.PostNotes)
	return nil
}

// runModuleCopyGen reuses gg gen's generator path but suppresses generated-file
// logs so module copy output stays focused on local-source files.
func runModuleCopyGen() error {
	// Module copy needs gg gen to create the target service shell, but it must not
	// turn this copy operation into a prune/cleanup pass over user service files.
	// Keep gg check enabled through genRunWithOptions(Quiet: true). If copied
	// module sources fail project checks, fix the framework module or the check
	// rule instead of bypassing validation here.
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
	printModuleCopyExtraModelReminder(plan)
	printModuleCopyExtraServiceReminder(plan)
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
	clioutput.Item("", "To remove copied module code, delete model/%s, then run: gg gen --prune --clean-orphans", name)
}

func printModuleCopyExtraModelReminder(plan *ggmodule.CopyPlan) {
	extraModelFiles := plan.ExtraModelTargets()
	if len(extraModelFiles) == 0 {
		return
	}

	clioutput.Section("Extra Target Model Files")
	clioutput.Warn("", "The target model directory contains files not present in the framework source")
	for _, file := range extraModelFiles {
		clioutput.Item("", "%s", file)
	}
	clioutput.Item("", "These files are not deleted automatically; review them before deleting")
}

func printModuleCopyExtraServiceReminder(plan *ggmodule.CopyPlan) {
	extraServiceFiles := plan.ExtraServiceTargets()
	if len(extraServiceFiles) == 0 {
		return
	}

	clioutput.Section("Extra Target Service Files")
	clioutput.Warn("", "The target service directory contains files not produced by this module copy plan")
	for _, file := range extraServiceFiles {
		clioutput.Item("", "%s", file)
	}
	clioutput.Item("", "These files are not deleted automatically; review them before deleting")
}

func printModuleCopyPostNotes(notes []string) {
	if len(notes) == 0 {
		return
	}
	fmt.Println()
	for _, note := range notes {
		fmt.Println(note)
	}
}
