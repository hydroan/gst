package linkedlist

import "github.com/hydroan/gst/ds/types"

var _ types.MutableBidIterator[any] = (*iterator[any])(nil)

// iterator represents a list iterator.
// It immplements MutableBidIterator.
type iterator[V any] struct {
	node *Node[V]
}

// NewIterator returns an iterator for the list.
func (l *List[V]) NewIterator(n *Node[V]) types.MutableBidIterator[V] {
	return &iterator[V]{n}
}

func (i *iterator[V]) IsValid() bool {
	return i.node != nil
}

func (i *iterator[V]) Next() types.Iterator[V] {
	if i.node != nil {
		i.node = i.node.Next
	}
	return i
}

func (i *iterator[V]) Prev() types.BidIterator[V] {
	if i.node != nil {
		i.node = i.node.Prev
	}
	return i
}

func (i *iterator[V]) Value() V {
	var v V
	if i.node != nil {
		return i.node.Value
	}
	return v
}

func (i *iterator[V]) SetValue(v V) {
	if i.node != nil {
		i.node.Value = v
	}
}

func (i *iterator[V]) Clone() types.Iterator[V] {
	return &iterator[V]{i.node}
}

func (i *iterator[V]) Equal(other types.Iterator[V]) bool {
	ii, ok := other.(*iterator[V])
	if !ok {
		return false
	}
	if ii.node == i.node {
		return true
	}
	return false
}
