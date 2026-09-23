// Package fixture has a fixture file that uses nothing unexported itself but
// serves an internal test.
package fixture

// Sample is the value under test.
type Sample struct{ Name string }

func label(s Sample) string { return "sample " + s.Name }
