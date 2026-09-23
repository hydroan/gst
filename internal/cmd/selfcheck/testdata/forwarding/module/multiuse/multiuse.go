// Package multiuse has a function that only forwards but has two uses.
package multiuse

import "strconv"

// Label names item n.
func Label(n int) string { return "item " + format(n) }

// Title names section n.
func Title(n int) string { return "section " + format(n) }

func format(n int) string { return strconv.Itoa(n) }
