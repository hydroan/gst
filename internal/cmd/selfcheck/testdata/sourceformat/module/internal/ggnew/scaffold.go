package ggnew

import "go/format"

// Scaffold formats a scaffold file as text.
func Scaffold(src []byte) ([]byte, error) {
	return format.Source(src)
}
