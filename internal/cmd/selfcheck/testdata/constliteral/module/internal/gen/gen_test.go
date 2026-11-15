package gen

import "testing"

// TestLocal spells a constant in a test file, which the check leaves alone.
func TestLocal(t *testing.T) {
	if local("gst") != "gst" {
		t.Fatal("local returns its argument")
	}
}
