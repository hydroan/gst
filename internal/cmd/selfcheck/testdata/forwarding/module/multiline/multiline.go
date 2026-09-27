// Package multiline has a function that only wraps one call and has one use,
// the call laid out over several lines: what its author set out to read on
// its own keeps its name, so nothing is reported.
package multiline

import "strings"

// Describe describes the parts, in brackets.
func Describe(parts []string) string { return "<" + join(parts) + ">" }

func join(parts []string) string {
	return strings.Join(
		parts,
		", ",
	)
}
