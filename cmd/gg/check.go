package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/ggcheck"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "check the project against the framework's conventions",
	Long:  projectCheckHelp(),
	Run: func(cmd *cobra.Command, args []string) {
		checkRun()
	},
}

// projectChecks lists the checks gg check runs, in the order it runs them,
// prints their results and numbers them in its help. gg gen and module copy
// run the same list.
var projectChecks = []ggcheck.Check{
	ggcheck.ArchitectureDependencies,
	ggcheck.ModelSingularNaming,
	ggcheck.ModelFileNameHyphens,
	ggcheck.JSONTagNaming,
	ggcheck.ModelActionTypeNaming,
	ggcheck.ActionTypeForm,
	ggcheck.ModelFileBoundaries,
	ggcheck.ServiceFileBoundaries,
	ggcheck.ModelPackageNaming,
	ggcheck.DirectoryRestrictions,
	ggcheck.DSLDesignRules,
	ggcheck.DatabaseChainTermination,
	ggcheck.TransactionClosureContext,
	ggcheck.DetachedContext,
	ggcheck.ServiceErrorDiscipline,
	ggcheck.ServiceTestCoverage,
	ggcheck.ServiceTestOrganization,
	ggcheck.LogFieldBoundedness,
	ggcheck.ModelTableNameDeclaration,
	ggcheck.GormTagIndexBan,
	ggcheck.VersionFieldDeclaration,
	ggcheck.ModuleAssembly,
	ggcheck.ColumnReferenceMinting,
}

// projectCheckSkips closes the gg check help: what the checks leave out.
const projectCheckSkips = `Model and service subtrees owned by copyable framework modules are skipped by Service test coverage, Service test organization, Log field boundedness, Model table name declaration, Gorm tag index ban, Version field declaration and Column reference minting, and their service subtrees by Detached context: copied module code is tested inside the framework repository.
Paths ignored by the project's Git ignore rules are skipped by every check, so runtime artifacts such as log directories never fail checks.`

// projectCheckHelp renders the gg check help from projectChecks: one numbered
// line per check, in the order gg check runs them and under the name its
// result prints, then projectCheckSkips. The help starts
//
//	Check the project against the framework's conventions:
//	1. Architecture dependencies: service code must not call other service code, dao code must not call service, router, controller or middleware code, and model code must not call service or dao code
//	2. Model singular naming: model directories and files must be singular
//	3. Model file name hyphens: model file names must not contain hyphens (use underscores instead)
func projectCheckHelp() string {
	var b strings.Builder
	b.WriteString("Check the project against the framework's conventions:\n")
	for i, pc := range projectChecks {
		fmt.Fprintf(&b, "%d. %s: %s\n", i+1, pc.Name, pc.Rule)
	}
	b.WriteString("\n")
	b.WriteString(projectCheckSkips)
	return b.String()
}

func checkRun() {
	totalViolations := runProjectChecks(false, nil)

	clioutput.Section("Summary")
	if totalViolations > 0 {
		clioutput.Error("", "%d violations found", totalViolations)
		os.Exit(1)
	} else {
		clioutput.Success("", "All checks passed")
	}
}

// runProjectChecks runs every project check, shared by gg check and gg gen.
//
// quiet suppresses output when the project is clean; violations always
// print. Violations recorded in baseline are treated as pre-existing and
// are neither counted nor printed, so callers such as module copy fail only
// on violations introduced after the baseline snapshot. A nil baseline
// keeps the full check behavior.
func runProjectChecks(quiet bool, baseline map[string]struct{}) int {
	results := filterProjectCheckResults(ggcheck.Run(projectChecks), baseline)
	total := totalProjectCheckViolations(results)
	if !quiet || total > 0 {
		printProjectCheckResults(results)
	}
	return total
}

// collectProjectCheckBaseline snapshots the current project check violations.
// Module copy records this baseline before writing any file, so its embedded
// gg gen run fails only on violations introduced by the copied module instead
// of pre-existing project issues.
func collectProjectCheckBaseline() map[string]struct{} {
	baseline := make(map[string]struct{})
	for _, result := range ggcheck.Run(projectChecks) {
		for _, violation := range result.Violations {
			baseline[violation] = struct{}{}
		}
	}
	return baseline
}

// filterProjectCheckResults drops violations recorded in baseline, keeping
// only violations introduced after the baseline snapshot.
func filterProjectCheckResults(results []ggcheck.Result, baseline map[string]struct{}) []ggcheck.Result {
	if len(baseline) == 0 {
		return results
	}
	filtered := make([]ggcheck.Result, 0, len(results))
	for _, result := range results {
		violations := make([]string, 0, len(result.Violations))
		for _, violation := range result.Violations {
			if _, preexisting := baseline[violation]; preexisting {
				continue
			}
			violations = append(violations, violation)
		}
		filtered = append(filtered, ggcheck.Result{Name: result.Name, Violations: violations})
	}
	return filtered
}

func printProjectCheckResults(results []ggcheck.Result) {
	clioutput.Section("Project Checks")
	for _, result := range results {
		if len(result.Violations) == 0 {
			clioutput.Success("", "%s", result.Name)
			continue
		}

		clioutput.Error("", "%s (%d)", result.Name, len(result.Violations))
		for _, violation := range result.Violations {
			clioutput.Item("", "%s", violation)
		}
	}
}

func totalProjectCheckViolations(results []ggcheck.Result) int {
	var total int
	for _, result := range results {
		total += len(result.Violations)
	}
	return total
}
