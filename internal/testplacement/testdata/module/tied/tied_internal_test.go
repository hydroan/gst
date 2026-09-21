package tied

import "testing"

func TestDouble(t *testing.T) {
	if Double(1) != twoUnits() {
		t.Fatal("Double(1) != twoUnits()")
	}
}
