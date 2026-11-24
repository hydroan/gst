// Package splaytree is a generic splay tree: a binary search tree holding
// keys in the order of a comparison function, each with its value, in which a
// lookup moves the node it ended on to the root, so keys looked up often stay
// near it. WithSafe makes a tree safe for concurrent use.
package splaytree

/*
====================
1. Zig rotation (single rotation)
- Applies when: the node being accessed, X, is a direct child of the root
  (its left or its right child).
- Operation: a single rotation, which makes X the root.

Left zig rotation
- X becomes the new root.
- S becomes the right child of X.
- A is the left subtree of X.
Before
    S
   /
  X
 /
A
After
  X
 / \
A   S

Right zig rotation
- X becomes the new root.
- S becomes the left child of X.
- A is the right subtree of X.
Before
  S
   \
    X
     \
      A
After
    X
   / \
  S   A



==============================
2. Zig-zig rotation (double rotation, same direction)
- Applies when:
  - X is the left child of S and S is the left child of G, the grandparent
    (X and S lie in the same direction);
  - or X is the right child of S and S is the right child of G.

Left zig-zig rotation
- Rotate right at G, which raises S.
- Rotate right at S, which raises X.
- X becomes the new root.
- S becomes the right subtree of X.
- G becomes the right subtree of S.
Before
      G
     /
    S
   /
  X
 /
A
After
      X
     / \
    A   S
         \
          G

Right zig-zig rotation
- Rotate left at G, which raises S.
- Rotate left at S, which raises X.
- X becomes the new root.
- S becomes the left subtree of X.
- G becomes the left subtree of S.
Before
  G
   \
    S
     \
      X
       \
        A
After
      X
     / \
    S   A
   /
  G



==============================
3. Zig-zag rotation (double rotation, opposite directions)
Applies when:
- X is the left child of S but S is the right child of G (X and S lie in
  opposite directions);
- or X is the right child of S but S is the left child of G.

Left-right zig-zag rotation
- Rotate right at S, which raises X.
- Rotate left at G, which raises X.
- X becomes the new root.
- G becomes the left subtree of X.
- S becomes the right subtree of X.
Before
      G
       \
        S
       /
      X
     /
    A
After the right rotation at S: X rises, and S becomes the right child of X.
      G
       \
        X
       / \
      A   S
After the left rotation at G: X rises, and G becomes the left child of X.
      X
     / \
    G   S
     \
      A

Right-left zig-zag rotation
- Rotate left at S, which raises X.
- Rotate right at G, which raises X.
- X becomes the new root.
- S becomes the left subtree of X.
- G becomes the right subtree of X.
Before
      G
     /
    S
     \
      X
       \
        A
After the left rotation at S: X rises, and S becomes the left child of X.
      G
     /
    X
   / \
  S   A
After the right rotation at G: X rises, and G becomes the right child of X.
      X
     / \
    S   G
       /
      A


Summary
Rotation                    When                                   Rotations
Zig (single)                the node accessed is a direct child    1
                            of the root
Zig-zig (double, same       the node, its parent and its           2
direction)                  grandparent lie in the same direction
Zig-zag (double, opposite   the node and its parent lie in         2
directions)                 opposite directions
These rotations are what makes a splay tree adjust itself: a node accessed
recently is reached more easily, which improves the locality of lookups and
with it their speed.

*/
