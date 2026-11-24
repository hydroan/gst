// Package avltree is a generic AVL tree: a self-balancing binary search tree
// holding keys in the order of a comparison function, each with its value.
// WithSafe makes a tree safe for concurrent use.
package avltree

/*

1. Right rotation (single rotation)
- Applies to: an LL (left-left) imbalance.
- Cause: the left subtree of a node is too high (balance factor BF > 1), and
  the new node went into the left side of that left subtree.
- Fix: rotate the unbalanced node right.
- Before and after the rotation: A < B < C.
Before
        C
       /
      B
     /
    A
After
        B
       / \
      A   C


2. Left rotation (single rotation)
- Applies to: an RR (right-right) imbalance.
- Cause: the right subtree of a node is too high (BF < -1), and the new node
  went into the right side of that right subtree.
- Fix: rotate the unbalanced node left.
- Before and after the rotation: A < B < C.
Before
    A
     \
      B
       \
        C
After
      B
     / \
    A   C


3. Left-right rotation (double rotation)
- Applies to: an LR (left-right) imbalance.
- Cause: the left subtree of a node is too high (BF > 1), but the new node
  went into the right side of that left subtree.
- Fix: rotate the left subtree left, then rotate the unbalanced node right.
- Before and after the rotation: A < B < C.
Before
        C
       /
      A
       \
        B
After the left rotation (of A)
        C
       /
      B
     /
    A
After the right rotation (of C)
        B
       / \
      A   C


4. Right-left rotation (double rotation)
- Applies to: an RL (right-left) imbalance.
- Cause: the right subtree of a node is too high (BF < -1), but the new node
  went into the left side of that right subtree.
- Fix: rotate the right subtree right, then rotate the unbalanced node left.
- Before and after the rotation: A < B < C.
Before
    A
     \
      C
     /
    B
After the right rotation (of C)
    A
     \
      B
       \
        C
After the left rotation (of A)
      B
     / \
    A   C

*/
