// Package valuemethod has a function that only wraps one call of a method of
// a package-level value of another package's type, and has one use.
package valuemethod

import "regexp"

var param = regexp.MustCompile(`:([^/]+)`)

// Template writes the parameters of route in braces.
func Template(route string) string { return "/" + normalize(route) }

func normalize(route string) string { return param.ReplaceAllString(route, "{$1}") }
