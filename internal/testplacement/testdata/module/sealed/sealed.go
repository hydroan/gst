// Package sealed has an internal test that implements an interface only the
// package itself can: nothing in the test names an unexported declaration, so
// only type-checking the test as an external one shows it has to stay.
package sealed

// Doer has an unexported method, so only this package can implement it.
type Doer interface{ do() int }

// Run returns what d does.
func Run(d Doer) int { return d.do() }
