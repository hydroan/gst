// Package closure has a function whose one statement before it forwards
// carries a function literal, which is work of its own rather than a shell.
package closure

import (
	"sort"
	"strings"
)

// Sorted sorts the names and joins them.
func Sorted(names []string) string {
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return joined(names)
}

func joined(names []string) string { return strings.Join(names, ", ") }
