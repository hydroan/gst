package mislabeled_test

import (
	"testing"

	"example.com/module/mislabeled"
)

func TestDouble(t *testing.T) {
	if mislabeled.Double(2) != 4 {
		t.Fatal("Double(2) != 4")
	}
}
