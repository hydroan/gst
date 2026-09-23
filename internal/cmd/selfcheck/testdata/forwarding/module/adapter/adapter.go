// Package adapter has functions that call another with something other than
// their own parameters, unchanged and in order.
package adapter

import "strings"

// Joined joins the names with commas, in brackets.
func Joined(names []string) string { return "[" + join(names) + "]" }

func join(names []string) string { return strings.Join(names, ", ") }

// Pair pairs a with b.
func Pair(a, b string) string { return "<" + pair(a, b) + ">" }

func pair(a, b string) string { return concat(b, a) }

func concat(a, b string) string { return a + b }
