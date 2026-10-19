package types

// Iterator is an interface of const iterator
type Iterator[V any] interface {
	IsValid() bool
	Next() Iterator[V]
	Value() V
	Clone() Iterator[V]
	Equal(other Iterator[V]) bool
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
