package arraylist_test

import (
	"fmt"

	"github.com/hydroan/gst/ds/list/arraylist"
)

func ExampleList() {
	list, _ := arraylist.New(cmp)

	list.Append(1, 2, 3)
	list.Remove(2)
	list.Sort()

	values := list.Values()
	for _, v := range values {
		fmt.Println(v)
	}
	// Output:
	// 1
	// 3
}
