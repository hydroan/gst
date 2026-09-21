// Command testplacementcheck reports the framework's test files that are
// named as internal tests without having to be (see package testplacement).
//
// Run it from the repository root through `make check`: golangci-lint's
// testpackage makes a test file that joins its package carry the
// _internal_test.go suffix, and this check makes the suffix true.
package main

import (
	"fmt"
	"os"

	"github.com/hydroan/gst/internal/testplacement"
)

func main() {
	violations, err := testplacement.Check(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "testplacementcheck:", err)
		os.Exit(1)
	}
	for _, v := range violations {
		fmt.Println(v.Message)
	}
	if len(violations) > 0 {
		os.Exit(1)
	}
}
