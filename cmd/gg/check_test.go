package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

// TestProjectCheckHelpNumbersEveryCheckInRunOrder pins the help
// projectCheckHelp renders: it opens with the lines its comment shows, and
// numbers every check in the order gg check runs them, under the name its
// result prints.
func TestProjectCheckHelpNumbersEveryCheckInRunOrder(t *testing.T) {
	help := projectCheckHelp()

	opening := strings.Join([]string{
		"Check the project against the framework's conventions:",
		"1. Architecture dependencies: service code must not call other service code, dao code must not call service, router, controller or middleware code, and model code must not call service or dao code",
		"2. Model singular naming: model directories and files must be singular",
		"3. Model file name hyphens: model file names must not contain hyphens (use underscores instead)",
	}, "\n")
	if !strings.HasPrefix(help, opening) {
		t.Fatalf("help opening = %q, want %q", help[:min(len(help), len(opening))], opening)
	}
	for i, pc := range projectChecks {
		if line := fmt.Sprintf("\n%d. %s: %s\n", i+1, pc.Name, pc.Rule); !strings.Contains(help, line) {
			t.Fatalf("help lacks the line of check %d %q", i+1, pc.Name)
		}
	}
	if !strings.HasSuffix(help, "\n\n"+projectCheckSkips) {
		t.Fatal("help does not close with the paths the checks skip")
	}
}

// TestProjectChecksRunEveryCheckInOrder pins the checks gg check runs and
// the order it runs them in, by the names their results print: a check left
// out of the list is declared and never run.
func TestProjectChecksRunEveryCheckInOrder(t *testing.T) {
	want := []string{
		"Architecture dependencies",
		"Model singular naming",
		"Model file name hyphens",
		"JSON tag naming",
		"Action type naming",
		"Action type form",
		"Action type uniqueness",
		"Model file boundaries",
		"Service file boundaries",
		"Model package naming",
		"Directory restrictions",
		"DSL design rules",
		"Database chain termination",
		"Transaction closure context",
		"Detached context",
		"Error discipline",
		"Service test coverage",
		"Service test organization",
		"TestMain imports",
		"Log field boundedness",
		"Model table name declaration",
		"Gorm tag index ban",
		"Validate tag ban",
		"Index declarations",
		"Framework columns",
		"Tenant unique indexes",
		"Date columns",
		"Query names",
		"ID accessors",
		"Version field declaration",
		"Module assembly",
		"Column reference minting",
		"gRPC service context",
		"Protobuf definitions",
	}
	if got := checkNames(projectChecks); !slices.Equal(got, want) {
		t.Fatalf("projectChecks = %q, want %q", got, want)
	}
}

// TestModuleCopyChecksLeaveTheModuleAssemblyToTheProject pins the checks the
// gg gen run of gg module copy makes: the generation checks but Module
// assembly, which the copy itself cannot satisfy and prints for the project
// to write instead.
func TestModuleCopyChecksLeaveTheModuleAssemblyToTheProject(t *testing.T) {
	generation := checkNames(generationChecks())
	if !slices.Contains(generation, ggcheck.ModuleAssembly.Name) {
		t.Fatal("gg gen runs the Module assembly check")
	}
	want := slices.DeleteFunc(slices.Clone(generation), func(name string) bool { return name == ggcheck.ModuleAssembly.Name })
	if got := checkNames(moduleCopyChecks()); !slices.Equal(got, want) {
		t.Fatalf("moduleCopyChecks() = %v, want %v", got, want)
	}
}

// TestGenerationChecksLeaveOutWhatTheRunDoesItselfAndWhatAsksTheCompiledModels
// pins the checks gg gen leaves out of the project checks it runs before
// generating: Protobuf definitions, which the run derives itself, TestMain
// imports, which it syncs itself, and Index declarations, Framework columns,
// Tenant unique indexes, Date columns, Query names and ID accessors, which
// ask the compiled models of a project that need not compile yet; every other
// check runs, in the order of gg check.
func TestGenerationChecksLeaveOutWhatTheRunDoesItselfAndWhatAsksTheCompiledModels(t *testing.T) {
	left := []string{ggcheck.ProtobufDefinitions.Name, ggcheck.TestMainImports.Name, ggcheck.IndexDeclarations.Name, ggcheck.FrameworkColumns.Name, ggcheck.TenantUniqueIndexes.Name, ggcheck.DateColumns.Name, ggcheck.QueryNames.Name, ggcheck.IDAccessors.Name}
	want := slices.DeleteFunc(checkNames(projectChecks), func(name string) bool { return slices.Contains(left, name) })
	if got := checkNames(generationChecks()); !slices.Equal(got, want) {
		t.Fatalf("generationChecks() = %v, want %v", got, want)
	}
}

// checkNames lists the names of checks, in their order.
func checkNames(checks []ggcheck.Check) []string {
	names := make([]string, 0, len(checks))
	for _, check := range checks {
		names = append(names, check.Name)
	}
	return names
}
