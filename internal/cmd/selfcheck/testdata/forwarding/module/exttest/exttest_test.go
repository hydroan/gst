package exttest_test

import (
	"testing"

	"example.com/module/exttest"
)

func TestNew(t *testing.T) {
	if newSample().Name != "one" {
		t.Fatal("unexpected name")
	}
}

func newSample() exttest.Sample { return exttest.New() }
