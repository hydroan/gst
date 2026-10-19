package linkedlist

import "github.com/hydroan/gst/ds/types"

var _ types.MutableBidIterator[any] = (*iterator[any])(nil)

// iterator represents a list iterator.
// It implements MutableBidIterator.
type iterator[V any] struct {
	node *Node[V]
}

// NewIterator returns an iterator positioned at the node n. A node not on
// this list gives an iterator that is not valid.
func (l *List[V]) NewIterator(n *Node[V]) types.MutableBidIterator[V] {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if n != nil && n.list != l {
		n = nil
	}
	return &iterator[V]{n}
}

func (i *iterator[V]) IsValid() bool {
	return i.node != nil
}

func (i *iterator[V]) Next() types.Iterator[V] {
	if i.node != nil {
		i.node = i.node.next
	}
	return i
}

func (i *iterator[V]) Prev() types.BidIterator[V] {
	if i.node != nil {
		i.node = i.node.prev
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
