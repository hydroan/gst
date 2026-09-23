package testuse

import "testing"

func TestFormat(t *testing.T) {
	if format(1) != "1" {
		t.Fatal("unexpected format")
	}
}
