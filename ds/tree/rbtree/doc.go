// Package rbtree is a generic red-black tree: a self-balancing binary search
// tree holding keys in the order of a comparison function, each with its
// value. WithSafe makes a tree safe for concurrent use.
package rbtree

/*
A red-black tree keeps five properties:
  1. A node is either red or black.
  2. The root is black.
  3. Every leaf (a nil node) is black.
  4. A red node has no red child.
  5. Every path from a node to the leaves below it holds the same number of
     black nodes.

The notes below follow the code, one entry per function of rbtree.go. In
them n is the node the function is called on, P its parent, G its
grandparent, U its uncle (the sibling of P) and S its sibling; a missing
node counts as black, which is what colorOf answers.

Insertion (Put)

An empty tree takes the new node as its black root. Otherwise the key is
looked up from the root: a node holding it gets the new value and nothing
else changes, and a key not found is added as a red leaf, on which
insertCase1 runs. A red node keeps property 5, so what is left to repair is
property 4, a red node under a red parent.

- insertCase1: n is the root. Paint it black; done. Otherwise go on to
  insertCase2.
- insertCase2: P is black. Nothing is broken; done. Otherwise go on to
  insertCase3.
- insertCase3: P and U are both red. Paint P and U black and G red, then
  start over from G at insertCase1, since G may now have a red parent or be
  the root. Otherwise, U being black, go on to insertCase4.
- insertCase4: n is an inner grandchild, the right child of a left child or
  the left child of a right child. Rotate at P, left in the first form and
  right in the second, which puts the two red nodes on the outside, and
  continue with the former parent as n. Either way go on to insertCase5.
- insertCase5: n is an outer grandchild. Paint P black and G red and rotate
  at G, right when n and P are left children and left when they are right
  children: P takes the place of G, black, over two red children; done.

Deletion (Delete)

A node with two children is not unlinked itself: it takes the key and the
value of its predecessor, the largest node of its left subtree, and the
predecessor, which has no right child, is unlinked in its place. The node to
unlink therefore has one child at most, which takes its position.

Unlinking a red node breaks nothing. Unlinking a black one leaves the paths
through its position one black node short, so before it goes it takes the
color of the child that replaces it, black when it has none, and stands in
for that child while deleteCase1 repairs the tree around it. A child that
ends up as the root is painted black.

- deleteCase1: n is the root. Every path lost the same node; done. Otherwise
  go on to deleteCase2.
- deleteCase2: S is red. Paint P red and S black and rotate at P toward n,
  left when n is a left child and right otherwise, which gives n a black
  sibling. Either way go on to deleteCase3.
- deleteCase3: P, S and both children of S are black. Paint S red, which
  leaves the whole subtree of P one black node short, and start over from P
  at deleteCase1. Otherwise go on to deleteCase4.
- deleteCase4: P is red while S and both its children are black. Paint S red
  and P black: the side of n gets its black node back and the side of S
  keeps its count; done. Otherwise go on to deleteCase5.
- deleteCase5: S is black, its child nearer to n red and its farther child
  black. Paint S red and the nearer child black and rotate at S away from n,
  right when n is a left child and left otherwise, which gives n a black
  sibling with a red farther child. Either way go on to deleteCase6.
- deleteCase6: S is black with a red farther child. S takes the color of P,
  P and the farther child turn black, and the tree rotates at P toward n,
  left when n is a left child and right otherwise; done.

*/
