// Package single has a function that only forwards and has one use.
package single

import "strconv"

// Label names item n.
func Label(n int) string { return "item " + format(n) }

func format(n int) string { return strconv.Itoa(n) }
