package gen

import "go/format"

// Format is the one formatting path of the generators.
func Format(src []byte) ([]byte, error) {
	return format.Source(src)
}
