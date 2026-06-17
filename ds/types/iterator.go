package types

// Iterator is an interface of const iterator
type Iterator[V any] interface {
	IsValid() bool
	Next() Iterator[V]
	Value() V
	Clone() Iterator[V]
	Equal(other Iterator[V]) bool
}

// MutableIterator is an interface of mutable iterator
type MutableIterator[V any] interface {
	Iterator[V]
	SetValue(value V)
}

// KvIterator is an interface of const key-value type iterator
type KvIterator[K, V any] interface {
	Iterator[V]
	Key() K
}

// MutableKvIterator is an interface of mutable key-value type iterator
type MutableKvIterator[K, V any] interface {
	KvIterator[K, V]
	SetValue(value V)
}

// BidIterator is an interface of const bidirectional iterator
type BidIterator[V any] interface {
	Iterator[V]
	Prev() BidIterator[V]
}

// MutableBidIterator is an interface of mutable bidirectional iterator
type MutableBidIterator[V any] interface {
	BidIterator[V]
	SetValue(value V)
}

// KvBidIterator is an interface of const key-value type bidirectional iterator
type KvBidIterator[K, V any] interface {
	KvIterator[K, V]
	MutableBidIterator[V]
}

// MutableKvBidIterator is an interface of mutable key-value type bidirectional iterator
type MutableKvBidIterator[K, V any] interface {
	MutableKvIterator[K, V]
	MutableBidIterator[V]
}

// RandomAccessIterator is an interface of mutable random access iterator
type RandomAccessIterator[V any] interface {
	MutableBidIterator[V]
	// IteratorAt returns a new iterator at position
	IteratorAt(position int) RandomAccessIterator[V]
	Position() int
}
