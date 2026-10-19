package util

import "cmp"

type Comparator[T any] func(x, y T) int

// EqualFn is a function that returns whether 'a' and 'b' are equal.
type EqualFn[T any] func(a, b T) bool

// LessFn is a function that returns whether 'a' is less than 'b'.
type LessFn[T any] func(a, b T) bool

// HashFn is a function that returns the hash of 't'.
type HashFn[T any] func(t T) uint64

// Equal wraps the '==' operator for comparable types.
func Equal[T comparable](a, b T) bool {
	return a == b
}

// Less wraps the '<' operator for ordered types.
func Less[T cmp.Ordered](a, b T) bool {
	return a < b
}

// Compare uses a less function to determine the ordering of 'a' and 'b'.
// It returns:
// * -1 if a < b
// * 1 if a > b
// * 0 if a == b
func Compare[T any](a, b T, less LessFn[T]) int {
	if less(a, b) {
		return -1
	} else if less(b, a) {
		return 1
	}
	return 0
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
