package clean_test

import (
	"testing"

	"example.com/module/clean"
)

func TestDouble(t *testing.T) {
	if clean.Double(2) != 4 {
		t.Fatal("Double(2) != 4")
	}
}
