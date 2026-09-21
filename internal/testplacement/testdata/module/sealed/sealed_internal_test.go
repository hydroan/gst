package sealed

import "testing"

type fake struct{}

func (fake) do() int { return 1 }

func TestRun(t *testing.T) {
	if Run(fake{}) != 1 {
		t.Fatal("unexpected result")
	}
}
