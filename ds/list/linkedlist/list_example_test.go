package linkedlist_test

import (
	"fmt"

	"github.com/hydroan/gst/ds/list/linkedlist"
	"github.com/hydroan/gst/ds/util"
)

func ExampleList_add() {
	// Create a new list
	l, _ := linkedlist.New[int]()

	// Add elements
	l.PushBack(1)
	l.PushBack(2)
	l.PushBack(3)

	n := l.Find(3, util.Equal)
	n = l.InsertAfter(n, 5)
	l.InsertBefore(n, 4)

	// Print all elements
	fmt.Println(l.Slice())
	// Output: [1 2 3 4 5]
}

func ExampleList_remove() {
	l, _ := linkedlist.NewFromSlice([]int{1, 2, 3, 4, 5})
	l.PopBack()
	l.PopFront()
	l.Remove(l.Find(3, util.Equal))
	fmt.Println(l.Slice())
	// Output: [2 4]
}

func ExampleList_reverse() {
	l, _ := linkedlist.NewFromSlice([]int{1, 2, 3, 4, 5})
	l.Reverse()
	fmt.Println(l.Slice())
	// Output: [5 4 3 2 1]
}

func ExampleList_merge() {
	l1, _ := linkedlist.NewFromSlice([]int{1, 3, 5})
	l2, _ := linkedlist.NewFromSlice([]int{2, 4, 6})

	l1.Merge(l2)

	fmt.Println(l1.Slice())
	// Output: [1 3 5 2 4 6]
}

func ExampleList_merge_sorted() {
	cmp := func(a, b int) int {
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	}

	l1, _ := linkedlist.NewFromSlice([]int{1, 3, 5})
	l2, _ := linkedlist.NewFromSlice([]int{2, 4, 6})

	l1.MergeSorted(l2, cmp)

	fmt.Println(l1.Slice())
	// Output: [1 2 3 4 5 6]
}

func ExampleList_find() {
	l, _ := linkedlist.NewFromSlice([]int{1, 2, 3, 4, 5})

	node := l.Find(3, util.Equal)

	if node != nil {
		fmt.Printf("Found value: %d\n", node.Value)
	}
	// Output: Found value: 3
}
