package avltree

type node[K comparable, V any] struct {
	Key      K
	Value    V
	Parent   *node[K, V]
	Children [2]*node[K, V]
	b        int
}
