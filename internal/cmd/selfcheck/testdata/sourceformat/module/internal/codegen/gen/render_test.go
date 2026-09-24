package gen

import (
	"go/format"
	"testing"
)

func TestRender(t *testing.T) {
	if _, err := format.Source([]byte("package sample\n")); err != nil {
		t.Fatal(err)
	}
}
