// Package branch has a function that decides something before it forwards,
// which is work of its own rather than a shell.
package branch

// Sample is a named value.
type Sample struct{ name string }

// Rename renames s, unless name is empty.
func Rename(s *Sample, name string) {
	if name == "" {
		return
	}
	rename(s, name)
}

func rename(s *Sample, name string) { s.name = name }
