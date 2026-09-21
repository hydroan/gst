package clean

import "testing"

func TestTwice(t *testing.T) {
	if twice(2) != 4 {
		t.Fatal("twice(2) != 4")
	}
}
