// Package tagged has a function that only forwards, used once in the default
// build and once more in a file the build leaves out.
package tagged

import "strconv"

// Label names item n.
func Label(n int) string { return "item " + format(n) }

func format(n int) string { return strconv.Itoa(n) }
