// Package mislabeled has a test file named as an internal test that declares
// the external test package.
package mislabeled

// Double returns twice n.
func Double(n int) int { return 2 * n }
