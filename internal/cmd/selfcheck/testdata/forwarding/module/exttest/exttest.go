// Package exttest has an external test with a helper that only forwards.
package exttest

// Sample is a value under test.
type Sample struct{ Name string }

// New returns a fresh sample.
func New() Sample { return Sample{Name: "one"} }
