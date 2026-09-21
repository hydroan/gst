package method

import "testing"

func (c *Counter) reset() { c.N = 0 }

func TestReset(t *testing.T) {
	c := &Counter{N: 3}
	c.reset()
	if c.N != 0 {
		t.Fatal("reset left", c.N)
	}
}
