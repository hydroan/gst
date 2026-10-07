package bigcache

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/types"
)

// TestGetDropsAnEntryItCannotDecode stores bytes the int codec cannot read
// straight into the backend, since the typed handle only writes what its codec
// reads back, and pins the read contract: Get answers ErrEntryNotFound and the
// entry is gone for the next read.
func TestGetDropsAnEntryItCannotDecode(t *testing.T) {
	ctx := context.Background()
	c, ok := Cache[int]().(*cache[int])
	if !ok {
		t.Fatalf("want the backend's own handle, got %T", Cache[int]())
	}
	if err := c.c.Set("unreadable-entry", []byte("not a number")); err != nil {
		t.Fatalf("store the raw bytes: %v", err)
	}
	if _, err := c.Get(ctx, "unreadable-entry"); !errors.Is(err, types.ErrEntryNotFound) {
		t.Fatalf("want ErrEntryNotFound for an entry that cannot be decoded, got %v", err)
	}
	if c.Exists(ctx, "unreadable-entry") {
		t.Fatal("want the unreadable entry dropped")
	}
}
