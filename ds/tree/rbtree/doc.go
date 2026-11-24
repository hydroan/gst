// Package rbtree is a generic red-black tree: a self-balancing binary search
// tree holding keys in the order of a comparison function, each with its
// value. WithSafe makes a tree safe for concurrent use.
package rbtree

/*
Properties of a red-black tree
- A node is either red or black.
- The root is black.
- The children of a red node are black (no two red nodes in a row).
- Every path from the root to a leaf holds the same number of black nodes.

Insertion
- A node is inserted red, and, as in any binary search tree, as a leaf.
- When the parent of the inserted node is black nothing is left to do: the
  tree still is a red-black tree.
- A node inserted as the root is simply turned black.

- CASE 1: the focus node is a and its uncle d is red. In order:
  - turn a's parent b and its uncle d black;
  - turn a's grandparent c red;
  - the focus node becomes a's grandparent c;
  - go on to CASE 2 or CASE 3.

- CASE 2: the focus node is a, its uncle d is black, and a is the right child
  of its parent b. In order:
  - the focus node becomes a's parent b;
  - rotate left around the new focus node b;
  - go on to CASE 3.

- CASE 3: the focus node is a, its uncle d is black, and a is the left child
  of its parent b. In order:
  - rotate right around a's grandparent c;
  - swap the colors of a's parent b and its sibling c;
  - the adjustment is done.



Deletion
Rebalancing after a deletion takes two steps. The first adjusts around the
deleted node, and only makes sure the whole tree still meets the last
property once the node is gone: for every node, all the paths from it to the
leaves it reaches hold the same number of black nodes. The second adjusts
around the focus node so that the tree meets the third property again: no two
red nodes next to each other.

First adjustment, around the deleted node
- CASE 1: the node to delete is a and it has a single child, b. In order:
  - delete a and put b in its place, as in any binary search tree;
  - a can only be black and b can only be red, anything else would not be a
    red-black tree; turn b black;
  - the adjustment is done, and the second one is not needed.

- CASE 2: the node to delete, a, has two non-nil children and its successor
  is its right child c. In order:
  - being a's successor, the right child c has no left subtree; delete a and
    put c in its place, as in any binary search tree;
  - then give c the color a has;
  - if c is black, its right child d takes one more black, so as not to break
    the last property, and d is now "red-black" or "black-black";
  - the focus node is now d, and the second adjustment works on it.

Second adjustment, around the focus node
- CASE 2: the focus node is a, its sibling c is black, and c's children d
  and e are both black. In order:
  - turn a's sibling c red;
  - take one black off the focus node a, which is then plain red or black;
  - add one black to a's parent b, which is then "red-black" or
    "black-black";
  - the focus node moves from a to its parent b;
  - go on with whichever of the four cases applies.

- CASE 3: the focus node is a, its sibling c is black, c's left child d is
  red and c's right child e is black. In order:
  - rotate right around a's sibling c;
  - swap the colors of c and d;
  - the focus node stays;
  - go on to CASE 4.

- CASE 4: the sibling c of the focus node a is black and c's right child is
  red. In order:
  - rotate left around a's parent b;
  - give a's sibling c the color of a's parent b;
  - turn a's parent b black;
  - take one black off the focus node a, which is then plain red or black;
  - turn a's uncle e black;
  - the adjustment is done.


The properties of a red-black tree in full
  1. A node is either red or black.
  2. The root is black.
  3. Every leaf (a nil node) is black.
  4. A red node has no red child (no two red nodes in a row).
  5. Every path from a node to its leaves holds the same number of black
     nodes (black balance).

*/
