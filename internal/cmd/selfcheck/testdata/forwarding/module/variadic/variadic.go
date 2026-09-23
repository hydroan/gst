// Package variadic has a variadic function that only forwards and has one
// use.
package variadic

import "fmt"

// Describe describes a sample by name and size.
func Describe(name string, size int) string { return describef("%s (%d)", name, size) }

func describef(format string, args ...any) string { return fmt.Sprintf(format, args...) }
