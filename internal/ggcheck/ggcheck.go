// Package ggcheck holds the rules gg check holds a business project to. Each
// Check finds the violations of one rule in the project in the working
// directory; the gg command running the checks decides which run, in what
// order, and how their results print.
package ggcheck

import (
	gitignore "github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// Check is one rule gg check holds a project to.
type Check struct {
	// Name is the name the check's result prints under.
	Name string
	// Rule states the rule, as the help of gg check lists it.
	Rule string

	run func(ignore gitignore.Matcher) []string
}

// Result is what one check found in the project.
type Result struct {
	// Name is the name of the check.
	Name string
	// Violations describes each violation found, one line each; it is empty
	// when the project follows the rule.
	Violations []string
}

// Run runs checks over the project in the working directory, in the order
// given, and returns their results in the same order. Paths the project's Git
// ignore rules ignore are left out of every check.
func Run(checks []Check) []Result {
	// One matcher serves every check: building it scans the whole worktree
	// for ignore files, which is too expensive to repeat per check.
	ignore := newProjectIgnoreMatcher()
	results := make([]Result, 0, len(checks))
	for _, check := range checks {
		results = append(results, Result{Name: check.Name, Violations: check.run(ignore)})
	}
	return results
}
