// Package exported has an exported function that only forwards and has one
// use in the package: it is API, whose uses outside the module no check can
// count.
package exported

// Double returns twice n.
func Double(n int) int { return twice(n) }

// Triple returns three times n.
func Triple(n int) int { return twice(n) + n }

// Quadruple returns four times n.
func Quadruple(n int) int { return 2 * Double(n) }

func twice(n int) int { return 2 * n }
