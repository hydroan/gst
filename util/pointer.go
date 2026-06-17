package util

// ValueOf returns a pointer to the given value
//
//go:fix inline
func ValueOf[T any](v T) *T {
	return new(v)
}

// Deref returns the value pointed to by the pointer.
// If the pointer is nil, returns the zero value for the type.
func Deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// DerefOr returns the value pointed to by the pointer.
// If the pointer is nil, returns the default value.
func DerefOr[T any](p *T, defaultVal T) T {
	if p == nil {
		return defaultVal
	}
	return *p
}
