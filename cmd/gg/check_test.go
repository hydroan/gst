package main

import (
	"fmt"
	"strings"
	"testing"
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
