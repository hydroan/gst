package multimap_test

// func ExampleMultiMap_basic() {
// 	// Create a new MultiMap
// 	mm, _ := multimap.New[string, int](util.Equal[int])
//
// 	// Add values
// 	mm.Set("fruits", 1)
// 	mm.Set("fruits", 2)
// 	mm.Set("vegetables", 3)
//
// 	// Get all values for a key
// 	fruits, _ := mm.Get("fruits")
// 	fmt.Println("Fruits:", fruits)
//
// 	// Get single value
// 	vegetable, _ := mm.GetOne("vegetables")
// 	fmt.Println("First vegetable:", vegetable)
//
// 	// Check existence
// 	fmt.Println("Has fruits:", mm.Has("fruits"))
// 	fmt.Println("Has meat:", mm.Has("meat"))
//
// 	// Output:
// 	// Fruits: [1 2]
// 	// First vegetable: 3
// 	// Has fruits: true
// 	// Has meat: false
// }
//
// func ExampleMultiMap_iteration() {
// 	mm, _ := multimap.New[string, int](util.Equal[int])
// 	mm.Set("a", 1)
// 	mm.Set("a", 2)
// 	mm.Set("b", 3)
//
// 	// Using Range
// 	mm.Range(func(key string, values []int) bool {
// 		fmt.Printf("Key: %s, Values: %v\n", key, values)
// 		return true
// 	})
//
// 	// Using Keys and Values
// 	fmt.Println("Keys:", mm.Keys())
// 	fmt.Println("All values:", mm.Values())
//
// 	// Output:
// 	// Key: a, Values: [1 2]
// 	// Key: b, Values: [3]
// 	// Keys: [a b]
// 	// All values: [1 2 3]
// }
