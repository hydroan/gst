package util

import (
	"cmp"

	"github.com/segmentio/fasthash/fnv1a"
)

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

// Max returns the max of a and b.
func Max[T cmp.Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}

// Min returns the min of a and b.
func Min[T cmp.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

// Clamp returns x constrained within [lo:hi] range.
// If x compares less than lo, returns lo;
// otherwise if hi compares less that x, returns hi;
// otherwise returns v.
func Clamp[T cmp.Ordered](x, lo, hi T) T {
	return Max(lo, Min(hi, x))
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
// otherwise if hi compares less that x, returns hi;
// otherwise returns v.
func ClampFunc[T any](x, lo, hi T, less LessFn[T]) T {
	return MaxFunc(lo, MinFunc(hi, x, less), less)
}

func HashUint64(u uint64) uint64 { return hash(u) }
func HashUint32(u uint32) uint64 { return hash(uint64(u)) }
func HashUint16(u uint16) uint64 { return hash(uint64(u)) }
func HashUint8(u uint8) uint64   { return hash(uint64(u)) }
func HashInt64(i int64) uint64   { return hash(uint64(i)) } //nolint:gosec
func HashInt32(i int32) uint64   { return hash(uint64(i)) } //nolint:gosec
func HashInt16(i int16) uint64   { return hash(uint64(i)) } //nolint:gosec
func HashInt8(i int8) uint64     { return hash(uint64(i)) } //nolint:gosec
func HashInt(i int) uint64       { return hash(uint64(i)) } //nolint:gosec
func HashUint(i uint) uint64     { return hash(uint64(i)) }
func HashString(s string) uint64 { return fnv1a.HashString64(s) }
func HashBytes(b []byte) uint64  { return fnv1a.HashBytes64(b) }

func hash(u uint64) uint64 {
	u ^= u >> 33
	u *= 0xff51afd7ed558ccd
	u ^= u >> 33
	u *= 0xc4ceb9fe1a85ec53
	u ^= u >> 33
	return u
}
