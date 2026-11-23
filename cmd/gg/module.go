package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/ggcheck"
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
	Short: "Manage the framework modules of the project",
	Args:  cobra.NoArgs,
	RunE:  showHelp,
}

var moduleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List framework modules",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runModuleList(cmd)
	},
}

var moduleAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Register a framework module in the current project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runModuleAdd(args[0])
	},
}

var moduleRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Unregister a framework module from the current project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runModuleRemove(args[0])
	},
}

var moduleCopyCmd = &cobra.Command{
	Use:   "copy <name>",
	Short: "Copy a framework module into the current project",
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
		clioutput.Change(clioutput.LabelSkip, "%s already registered in %s", result.Module.Name, result.Path)
	default:
		clioutput.Change(clioutput.LabelAdd, "%s registered in %s", result.Module.Name, result.Path)
	}
	return nil
}

func runModuleRemove(name string) error {
	result, err := ggmodule.RemoveModule(".", name)
	if err != nil {
		return err
	}

	clioutput.Change(clioutput.LabelRemove, "%s unregistered from %s", result.Module.Name, result.Path)
	return nil
}

// runModuleCopy owns the command-level workflow only: build a checked plan,
// show the current-project files that will be touched, confirm, execute, then
// tidy the module so that go.mod and go.sum hold what the copied sources
// import, and end on what the copy changed. Source analysis and AST rewriting
// live in internal/ggmodule.
func runModuleCopy(name string, opts moduleCopyOptions) error {
	started := time.Now()
	copyOpts := ggmodule.CopyOptions{Force: opts.Force}
	plan, err := ggmodule.BuildCopyPlan(name, copyOpts)
	if err != nil {
		return err
	}

	printModuleCopyPlan(plan)
	if !opts.Yes && !clioutput.Confirm("Copy module %s into the current project?", name) {
		clioutput.Item("Module copy canceled")
		return nil
	}

	// Snapshot current project check violations before any file is written, so
	// the copy-time gen run fails only on violations introduced by this copy.
	baseline := collectProjectCheckBaseline()
	log := new(changes)
	exec := ggmodule.CopyExecution{
		Plan:    plan,
		Options: copyOpts,
		RunGen: func() error {
			return runModuleCopyGen(baseline, plan.ServiceTargets(), nil)
		},
		OnSection: clioutput.Section,
		// Every file the copy went over is listed under the label of what
		// happened to it, the ones left as they are included: the list is
		// the plan above, carried out.
		OnFile: func(status ggmodule.CopyWriteStatus, path string) {
			log.record(moduleCopyLabels[status], path, "")
		},
	}
	if err := exec.Run(); err != nil {
		if len(exec.WrittenFiles) > 0 || len(exec.DeletedFiles) > 0 {
			printModuleCopyCleanup(name)
		}
		return err
	}

	if err := tidyModule(log); err != nil {
		printModuleCopyCleanup(name)
		return err
	}

	if summary := log.summary(); summary != "" {
		clioutput.Done("Copied module %s: %s (%s)", name, summary, elapsedSince(started))
	} else {
		clioutput.Done("Copied module %s: every file was up to date (%s)", name, elapsedSince(started))
	}
	printModuleCopyCleanup(name)
	printModuleCopyAssembly()
	for _, note := range plan.PostNotes {
		clioutput.Note("%s", note)
	}
	return nil
}

// moduleCopyLabels are the labels the files of a module copy are listed
// under, by what the copy did to each.
var moduleCopyLabels = map[ggmodule.CopyWriteStatus]clioutput.Label{
	ggmodule.CopyWriteSkip:   clioutput.LabelSkip,
	ggmodule.CopyWriteUpdate: clioutput.LabelUpdate,
	ggmodule.CopyWriteCreate: clioutput.LabelCreate,
	ggmodule.CopyWriteDelete: clioutput.LabelRemove,
}

// runModuleCopyGen reuses gg gen's generator path for the copy: the service
// files among targets, the ones the copy merges the module's source onto, are
// created as shells (see genRunOptions.MergeShells). What the run generates
// is recorded in log; gg module copy passes none, so that its output stays
// focused on the local-source files it copies, and gg upgrade passes the one
// it counts every file it changed in.
func runModuleCopyGen(baseline map[string]struct{}, targets []string, log *changes) error {
	// Module copy needs gg gen to create the target service shell. Project
	// checks stay enabled through genRunWithOptions(Quiet: true), scoped by
	// baseline to violations introduced by this copy: pre-existing project
	// violations must not block copying an unrelated module. The one check
	// the copy itself fails, Module assembly, is left out and printed once
	// the copy is done (see moduleCopyChecks). If copied module sources
	// introduce new violations, fix the framework module or the check rule
	// instead of bypassing validation here.
	shells := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		shells[filepath.Clean(target)] = struct{}{}
	}
	return genRunWithOptions(genRunOptions{Quiet: true, Log: log, BaselineViolations: baseline, Checks: moduleCopyChecks(), MergeShells: shells})
}

func printModuleCopyPlan(plan *ggmodule.CopyPlan) {
	clioutput.Section("Module Copy Plan")
	printModuleCopyPlanGroup("Model files", plan.ModelTargets())
	printModuleCopyPlanGroup("Service files", plan.ServiceTargets())
	printModuleCopyPlanGroup("Helper files", plan.HelperTargets())
	printModuleCopyPlanGroup("Middleware files", plan.MiddlewareTargets())
	printModuleCopyPlanGroup("Interceptor files", plan.InterceptorTargets())
	printModuleCopyStaleModelFiles(plan)
	printModuleCopyStaleServiceFiles(plan)
	printModuleCopyStaleMiddlewareFiles(plan)
	printModuleCopyStaleInterceptorFiles(plan)
}

func printModuleCopyPlanGroup(title string, files []string) {
	if len(files) == 0 {
		return
	}
	sort.Strings(files)
	clioutput.Item("%s", title)
	for _, file := range files {
		clioutput.Detail("%s", file)
	}
}

func printModuleCopyCleanup(name string) {
	clioutput.Note("To remove copied module code, delete model/%s, then run: gg gen and gg prune", name)
}

// printModuleCopyStaleModelFiles previews the stale model files the copy
// execution will delete. It prints before the confirmation prompt, so
// answering yes consents to exactly this list; test files and generated files
// never appear here because the plan exempts them from pruning.
func printModuleCopyStaleModelFiles(plan *ggmodule.CopyPlan) {
	staleModelFiles := plan.StaleModelTargets()
	if len(staleModelFiles) == 0 {
		return
	}

	clioutput.Section("Stale Target Model Files")
	clioutput.Warn("The target model directory contains files no longer present in the framework source")
	for _, file := range staleModelFiles {
		clioutput.Item("%s", file)
	}
	clioutput.Item("These files will be deleted to keep the copied module in sync with the framework source")
}

// printModuleCopyStaleServiceFiles previews the stale service files the copy
// execution will delete, under the same consent contract as
// printModuleCopyStaleModelFiles.
func printModuleCopyStaleServiceFiles(plan *ggmodule.CopyPlan) {
	staleServiceFiles := plan.StaleServiceTargets()
	if len(staleServiceFiles) == 0 {
		return
	}

	clioutput.Section("Stale Target Service Files")
	clioutput.Warn("The target service directory contains files no longer produced by this module copy plan")
	for _, file := range staleServiceFiles {
		clioutput.Item("%s", file)
	}
	clioutput.Item("These files will be deleted to keep the copied module in sync with the framework source")
}

// printModuleCopyStaleMiddlewareFiles previews the module-owned middleware
// files the copy execution will delete along with their register calls and,
// once unused, the framework middleware import, under the same consent
// contract as printModuleCopyStaleModelFiles.
func printModuleCopyStaleMiddlewareFiles(plan *ggmodule.CopyPlan) {
	staleMiddlewareFiles := plan.StaleMiddlewareTargets()
	if len(staleMiddlewareFiles) == 0 {
		return
	}

	clioutput.Section("Stale Target Middleware Files")
	clioutput.Warn("The middleware directory contains files owned by this module's copy but no longer declared by the module manifest")
	for _, file := range staleMiddlewareFiles {
		clioutput.Item("%s", file)
	}
	clioutput.Item("These files will be deleted together with their register calls in middleware/middleware.go, and the framework middleware import once nothing there uses it")
}

// printModuleCopyStaleInterceptorFiles previews the module-owned interceptor
// files the copy execution will delete along with their register calls and,
// once unused, the framework interceptor import, under the same consent
// contract as printModuleCopyStaleModelFiles: the ones the manifest no
// longer declares, or all of them once the project no longer serves gRPC.
func printModuleCopyStaleInterceptorFiles(plan *ggmodule.CopyPlan) {
	staleInterceptorFiles := plan.StaleInterceptorTargets()
	if len(staleInterceptorFiles) == 0 {
		return
	}

	clioutput.Section("Stale Target Interceptor Files")
	clioutput.Warn("The interceptor directory contains files owned by this module's copy but no longer written by it: not declared by the module manifest, or the project no longer serves gRPC")
	for _, file := range staleInterceptorFiles {
		clioutput.Item("%s", file)
	}
	clioutput.Item("These files will be deleted together with their register calls in interceptor/interceptor.go, and the framework interceptor import once nothing there uses it")
}

// printModuleCopyAssembly prints the assembly calls the copied modules
// declare and the project has yet to make, each as gg check reports it: the
// copy left them out of its own checks (see moduleCopyChecks), so they are
// the first thing to do once it is done. A project that makes them all
// prints nothing.
func printModuleCopyAssembly() {
	for _, result := range ggcheck.Run([]ggcheck.Check{ggcheck.ModuleAssembly}) {
		for _, violation := range result.Violations {
			clioutput.Warn("Assembly left to the project: %s", violation)
		}
	}
}
