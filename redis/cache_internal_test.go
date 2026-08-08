package redis

import (
	"context"
	"testing"
)

// TestCacheWithoutClientFailsClosed asserts the uninitialized-client paths:
// the write and read operations report the missing client and Exists answers
// false, its only honest value without an error channel.
func TestCacheWithoutClientFailsClosed(t *testing.T) {
	old := cli
	cli = nil
	defer func() { cli = old }()

	ctx := context.Background()
	c := cache[string]{}
	if err := c.Set(ctx, "uninitialized", "value", 0); err == nil {
		t.Fatal("want error from Set without a client")
	}
	if _, err := c.Get(ctx, "uninitialized"); err == nil {
		t.Fatal("want error from Get without a client")
	}
	if err := c.Delete(ctx, "uninitialized"); err == nil {
		t.Fatal("want error from Delete without a client")
	}
	if c.Exists(ctx, "uninitialized") {
		t.Fatal("want false from Exists without a client")
	}
}
