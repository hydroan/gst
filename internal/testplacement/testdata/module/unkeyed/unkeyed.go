// Package unkeyed has an internal test that writes an unkeyed literal of a
// struct with an unexported field.
package unkeyed

// Pair holds two numbers, one of them unexported.
type Pair struct {
	A int
	b int
}

// Sum adds the pair up.
func (p Pair) Sum() int { return p.A + p.b }
