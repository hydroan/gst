package ggmodule

import "go/format"

// Write formats a registration file as text.
func Write(src []byte) ([]byte, error) {
	return format.Source(src)
}
