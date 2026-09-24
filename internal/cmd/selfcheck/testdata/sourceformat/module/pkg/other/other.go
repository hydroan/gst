package other

import "go/format"

// Tidy formats source text outside the generators.
func Tidy(src []byte) ([]byte, error) {
	return format.Source(src)
}
