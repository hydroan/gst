// Package funclit has a function that wraps one call and has one use, but
// hands the call a function literal.
package funclit

// Lengths lists the length of every name, after the count of them.
func Lengths(names []string) []int { return append([]int{len(names)}, lengths(names)...) }

func lengths(names []string) []int {
	return mapped(names, func(name string) int { return len(name) })
}

func mapped(names []string, f func(string) int) []int {
	out := make([]int, 0, len(names))
	for _, name := range names {
		out = append(out, f(name))
	}
	return out
}
