// Command selfcheck holds the framework's own source to the rules
// golangci-lint cannot express, and prints each violation it finds.
//
// Run it from the repository root through `make check`. The rules bind the
// framework alone: make check runs them over the framework, and a project is
// held to none of them.
//
// # Test placement
//
// golangci-lint's testpackage makes a test file that joins the package it
// tests say so: its name ends in _internal_test.go. That settles the name but
// not the need, and this check makes the suffix true. A file can carry the
// suffix and still use nothing unexported of its package, or even declare the
// external test package; the check reports both, so an internal test is always
// one that could not be written from outside.
//
// # Thin forwarding
//
// A function whose only work is handing its own parameters, unchanged and in
// order, to another function is a layer that adds a name and nothing else.
// The check reports it where the layer can go: the function has a single use,
// which can call the other one itself, or the function it forwards to has no
// other use, so the two can be one. checkForwarding spells out what counts,
// and what the check leaves alone because it cannot count every use.
//
// # Source formatting
//
// The code generators build the Go code they generate as syntax trees and
// print them through one formatting path. A generator that formats source
// text instead assembled its output as a string, and one that imports
// text/template renders it from a template, so a call to a source text
// formatter is reported anywhere in the generators but that one path, and an
// import of text/template anywhere in them. checkSourceFormat names the
// formatters, the generators and the path.
//
// # Layout
//
// Each check lives in the file named after it, testplacement.go for the
// check "testplacement", holding its run function and the helpers only it
// uses; helper.go holds what the checks share, and main.go runs them.
// TestEveryCheckIsDeclaredInAFileNamedAfterIt holds the layout.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"golang.org/x/tools/go/packages"
)

// violation is a breach of one of the rules, found in one file.
type violation struct {
	// File is the path of the file, relative to the checked directory.
	File string
	// Message says what is wrong and how to fix it.
	Message string
}

// check is one of the rules selfcheck holds the framework to.
type check struct {
	// name prefixes the errors the check returns.
	name string
	// run reports the violations of the rule among pkgs, the packages under
	// root as load returns them.
	run func(root string, pkgs []*packages.Package) ([]violation, error)
}

// checks lists the rules in the order they run and report.
var checks = []check{
	{name: "testplacement", run: checkTestPlacement},
	{name: "forwarding", run: checkForwarding},
	{name: "sourceformat", run: checkSourceFormat},
}

func main() {
	violations, err := run(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "selfcheck:", err)
		os.Exit(1)
	}
	for _, v := range violations {
		fmt.Println(v.Message)
	}
	if len(violations) > 0 {
		os.Exit(1)
	}
}

// run loads the packages under dir once, tests included, and runs every check
// over them, returning what the checks find in the order they run. A package
// that does not type-check stops the run: no check can judge code the
// compiler rejects.
func run(dir string) ([]violation, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, errors.Wrap(err, "resolve the checked directory")
	}
	pkgs, err := load(root, nil, "./...")
	if err != nil {
		return nil, err
	}
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			return nil, errors.Newf("package %s does not type-check: %v", p.ID, p.Errors[0])
		}
	}

	var violations []violation
	for _, c := range checks {
		found, err := c.run(root, pkgs)
		if err != nil {
			return nil, errors.Wrap(err, c.name)
		}
		violations = append(violations, found...)
	}
	return violations, nil
}
