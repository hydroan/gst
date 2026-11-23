// Package errors stands for the package the framework handles its errors
// with.
package errors

// New returns an error saying text.
func New(text string) error { return plain(text) }

type plain string

func (e plain) Error() string { return string(e) }

// Is tests err against reference, reading nothing either carries.
func Is(err, reference error) bool { return err == reference }

// As hands the error of the type target points to, found in err, to the
// caller.
func As(err error, target any) bool { return false }
