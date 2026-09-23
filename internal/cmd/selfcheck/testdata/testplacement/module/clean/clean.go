// Package clean has an internal test that has to be one.
package clean

// Double returns twice n.
func Double(n int) int { return twice(n) }

func twice(n int) int { return 2 * n }
