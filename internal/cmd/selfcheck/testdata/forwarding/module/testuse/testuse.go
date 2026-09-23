// Package testuse has a function that only forwards, used once by the package
// and once more by its internal test.
package testuse

import "strconv"

// Label names item n.
func Label(n int) string { return "item " + format(n) }

func format(n int) string { return strconv.Itoa(n) }
