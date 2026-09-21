// Package tied has a test that uses nothing unexported itself but relies on a
// helper that does.
package tied

// Double returns twice n.
func Double(n int) int { return 2 * n }

func unit() int { return 1 }
