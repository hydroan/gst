package util

import (
	"fmt"
	"math"
	"time"
)

func ExampleMax() {
	fmt.Println(Max(7, 3))
	fmt.Println(Max(2*time.Second, 3*time.Second).Milliseconds())

	// Output:
	// 7
	// 3000
}

func ExampleMin() {
	fmt.Println(Min(7, 3))
	fmt.Println(Min(2*time.Second, 3*time.Second).Milliseconds())

	// Output:
	// 3
	// 2000
}

func ExampleClamp() {
	fmt.Println(Clamp(500, 400, 600))
	fmt.Println(Clamp(200, 400, 600))
	fmt.Println(Clamp(800, 400, 600))

	fmt.Println(Clamp(5*time.Second, 4*time.Second, 6*time.Second).Milliseconds())
	fmt.Println(Clamp(2*time.Second, 4*time.Second, 6*time.Second).Milliseconds())
	fmt.Println(Clamp(8*time.Second, 4*time.Second, 6*time.Second).Milliseconds())

	fmt.Println(Clamp(1.5, 1.4, 1.8))
	fmt.Println(Clamp(1.5, 1.8, 1.8))
	fmt.Println(Clamp(1.5, 2.1, 1.9))

	// Output:
	// 500
	// 400
	// 600
	// 5000
	// 4000
	// 6000
	// 1.5
	// 1.8
	// 2.1
}

func lessMagnitude(a, b float64) bool {
	return math.Abs(a) < math.Abs(b)
}

func ExampleMaxFunc() {
	fmt.Println(MaxFunc(2.5, -3.1, lessMagnitude))
	// Output:
	// -3.1
}

func ExampleMinFunc() {
	fmt.Println(MinFunc(2.5, -3.1, lessMagnitude))
	// Output:
	// 2.5
}

func ExampleClampFunc() {
	fmt.Println(ClampFunc(1.5, 1.4, 1.8, lessMagnitude))
	fmt.Println(ClampFunc(1.5, 1.8, 1.8, lessMagnitude))
	fmt.Println(ClampFunc(1.5, 2.1, 1.9, lessMagnitude))
	fmt.Println(ClampFunc(-1.5, -1.4, -1.8, lessMagnitude))
	fmt.Println(ClampFunc(-1.5, -1.8, -1.8, lessMagnitude))
	fmt.Println(ClampFunc(-1.5, -2.1, -1.9, lessMagnitude))
	fmt.Println(ClampFunc(1.5, -1.5, -1.5, lessMagnitude))
	// Output:
	// 1.5
	// 1.8
	// 2.1
	// -1.5
	// -1.8
	// -2.1
	// 1.5
}
