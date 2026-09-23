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
package main

import (
	"fmt"
	"os"
)

func main() {
	violations, err := checkTestPlacement(".")
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
