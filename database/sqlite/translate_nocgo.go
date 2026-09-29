//go:build !cgo

package sqlite

// translate translates no error, and so leaves every one to the driver's
// translation. Built without cgo, the driver is a stub whose every
// connection fails to open, the error saying the binary needs cgo, so no
// error of a statement ever reaches it; translate_cgo.go holds the
// translation of a binary built with cgo.
func translate(error) error {
	return nil
}
