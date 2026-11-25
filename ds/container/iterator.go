package container

// Iterator walks the values of a structure forward and reads them.
type Iterator[V any] interface {
	IsValid() bool
	Next() Iterator[V]
	Value() V
	Clone() Iterator[V]
	Equal(other Iterator[V]) bool
}

// BidiIterator is an Iterator that also walks backward.
type BidiIterator[V any] interface {
	Iterator[V]
	Prev() BidiIterator[V]
}

// MutableBidiIterator is a BidiIterator that also replaces the value it
// stands on.
type MutableBidiIterator[V any] interface {
	BidiIterator[V]
	SetValue(value V)
}
