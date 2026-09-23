package unkeyed

import "testing"

func TestSum(t *testing.T) {
	if (Pair{1, 2}).Sum() != 3 {
		t.Fatal("unexpected sum")
	}
}
