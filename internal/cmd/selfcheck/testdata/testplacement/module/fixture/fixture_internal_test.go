package fixture

import "testing"

func TestLabel(t *testing.T) {
	if label(newSample()) != "sample one" {
		t.Fatal("unexpected label")
	}
}
