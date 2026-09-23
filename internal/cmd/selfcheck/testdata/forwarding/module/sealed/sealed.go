// Package sealed has a method that only forwards and has one use the check can
// see, but an interface of the package declares it, so it may be called
// through the interface as well.
package sealed

type closer interface{ close() error }

// Conn is a connection.
type Conn struct{ open bool }

// Close closes c.
func (c *Conn) Close() error { return c.close() }

// Reset shuts c down and opens it again.
func (c *Conn) Reset() error {
	if err := c.shutdown(); err != nil {
		return err
	}
	c.open = true
	return nil
}

func (c *Conn) close() error { return c.shutdown() }

func (c *Conn) shutdown() error {
	c.open = false
	return nil
}

// CloseAll closes every closer.
func CloseAll(cs []closer) {
	for _, c := range cs {
		_ = c.close()
	}
}
