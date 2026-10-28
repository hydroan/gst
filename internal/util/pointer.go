package util

// Deref returns the value pointed to by the pointer.
// If the pointer is nil, returns the zero value for the type.
func Deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
