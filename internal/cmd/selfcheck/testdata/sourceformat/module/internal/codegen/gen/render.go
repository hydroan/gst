package gen

import "go/format"

// render builds its output as text and formats it.
func render() ([]byte, error) {
	return format.Source([]byte("package sample\n"))
}
