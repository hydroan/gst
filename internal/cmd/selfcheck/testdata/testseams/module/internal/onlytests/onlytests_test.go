package onlytests

import "testing"

func TestFlag(t *testing.T) {
	if Flag != 1 {
		t.Fatal(Flag)
	}
}
