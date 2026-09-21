package util_test

import (
	"fmt"

	"github.com/hydroan/gst/util"
)

func ExampleRound() {
	fmt.Printf("%.3f\n", util.Round(3.14159, 3))
	fmt.Printf("%.2f\n", util.Round(2.71828, 2))
	fmt.Printf("%.1f\n", util.Round(-3.14159, 1))
	// Output:
	// 3.142
	// 2.72
	// -3.1
}
