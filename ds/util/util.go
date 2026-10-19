package util

import "cmp"

type Comparator[T any] func(x, y T) int

// EqualFn is a function that returns whether 'a' and 'b' are equal.
type EqualFn[T any] func(a, b T) bool

// LessFn is a function that returns whether 'a' is less than 'b'.
type LessFn[T any] func(a, b T) bool

// Equal wraps the '==' operator for comparable types.
func Equal[T comparable](a, b T) bool {
	return a == b
}

// Clamp returns x constrained within [lo:hi] range.
// If x compares less than lo, returns lo;
// otherwise if hi compares less than x, returns hi;
// otherwise returns x.
func Clamp[T cmp.Ordered](x, lo, hi T) T {
	return max(lo, min(hi, x))
}

// MaxFunc returns the max of a and b using the less func.
func MaxFunc[T any](a, b T, less LessFn[T]) T {
	if less(b, a) {
		return a
	}
	return b
}

// MinFunc returns the min of a and b using the less func.
func MinFunc[T any](a, b T, less LessFn[T]) T {
	if less(a, b) {
		return a
	}
	return b
}

// ClampFunc returns x constrained within [lo:hi] range using the less func.
// If x compares less than lo, returns lo;
// otherwise if hi compares less than x, returns hi;
// otherwise returns x.
func ClampFunc[T any](x, lo, hi T, less LessFn[T]) T {
	return MaxFunc(lo, MinFunc(hi, x, less), less)
}
