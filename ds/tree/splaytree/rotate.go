package splaytree

// rotateZigRight is the single right rotation (a zig).
//
//	Before Rotation:
//	     g
//	    / \
//	   p   C
//	  / \
//	 A   B
//
//	After Rotation:
//	     p
//	    / \
//	   A   g
//	      / \
//	     B   C
func rotateZigRight[K comparable, V any](g *node[K, V]) *node[K, V] {
	if g == nil || g.Children[0] == nil {
		return g
	}

	p := g.Children[0]

	g.Children[0] = p.Children[1]
	if g.Children[0] != nil {
		g.Children[0].Parent = g
	}
	p.Children[1] = g
	p.Parent = g.Parent
	if g.Parent != nil {
		if g.Parent.Children[0] == g {
			g.Parent.Children[0] = p
		} else {
			g.Parent.Children[1] = p
		}
	}
	g.Parent = p

	return p
}

// rotateZigLeft is the single left rotation (a zig).
//
//	Before Rotation:
//	     g
//	    / \
//	   A   p
//	      / \
//	     B   C
//
//	After Rotation:
//	     p
//	    / \
//	   g   C
//	  / \
//	 A   B
func rotateZigLeft[K comparable, V any](g *node[K, V]) *node[K, V] {
	if g == nil || g.Children[1] == nil {
		return g
	}

	p := g.Children[1]

	g.Children[1] = p.Children[0]
	if g.Children[1] != nil {
		g.Children[1].Parent = g
	}
	p.Children[0] = g
	p.Parent = g.Parent
	if g.Parent != nil {
		if g.Parent.Children[0] == g {
			g.Parent.Children[0] = p
		} else {
			g.Parent.Children[1] = p
		}
	}
	g.Parent = p

	return p
}

// rotateZigZigRight is the double right rotation (a zig-zig).
//
// Step 1: a zig right rotation of g and p.
//
//	       g
//	      / \
//	     p   D
//	    / \
//	   x   C
//	  / \
//	 A   B
//
//	After the zig right rotation:
//	     p
//	    / \
//	   x   g
//	  / \  / \
//	 A   B C  D
//
// Step 2: a zig right rotation of p and x.
//
//	  x
//	 / \
//	A   p
//	   / \
//	  B   g
//	     / \
//	    C   D
func rotateZigZigRight[K comparable, V any](g *node[K, V]) *node[K, V] {
	if g == nil || g.Children[0] == nil || g.Children[0].Children[0] == nil {
		return g
	}

	p := g.Children[0]
	x := p.Children[0]

	// Right Rotate "g"
	g.Children[0] = p.Children[1]
	if g.Children[0] != nil {
		g.Children[0].Parent = g
	}
	p.Children[1] = g
	p.Parent = g.Parent
	if g.Parent != nil {
		if g.Parent.Children[0] == g {
			g.Parent.Children[0] = p
		} else {
			g.Parent.Children[1] = p
		}
	}
	g.Parent = p

	// Right Rotate "p"
	p.Children[0] = x.Children[1]
	if p.Children[0] != nil {
		p.Children[0].Parent = p
	}
	x.Children[1] = p
	x.Parent = p.Parent
	if p.Parent != nil {
		if p.Parent.Children[0] == p {
			p.Parent.Children[0] = x
		} else {
			p.Parent.Children[1] = x
		}
	}
	p.Parent = x

	return x
}

// rotateZigZigLeft is the double left rotation (a zig-zig).
//
// Step 1: a zig left rotation of g and p.
//
//	   g
//	  / \
//	 A   p
//	    / \
//	   B   x
//	      / \
//	     C   D
//
//	After the zig left rotation:
//	     p
//	    / \
//	   g   x
//	  / \  / \
//	 A   B C  D
//
// Step 2: a zig left rotation of p and x.
//
//	      x
//	     / \
//	    p   D
//	   / \
//	  g   C
//	 / \
//	A   B
func rotateZigZigLeft[K comparable, V any](g *node[K, V]) *node[K, V] {
	if g == nil || g.Children[1] == nil || g.Children[1].Children[1] == nil {
		return g
	}

	p := g.Children[1]
	x := p.Children[1]

	// Left Rotate "g"
	g.Children[1] = p.Children[0]
	if g.Children[1] != nil {
		g.Children[1].Parent = g
	}
	p.Children[0] = g
	p.Parent = g.Parent
	if g.Parent != nil {
		if g.Parent.Children[0] == g {
			g.Parent.Children[0] = p
		} else {
			g.Parent.Children[1] = p
		}
	}
	g.Parent = p

	// Left Rotate "p"
	p.Children[1] = x.Children[0]
	if p.Children[1] != nil {
		p.Children[1].Parent = p
	}
	x.Children[0] = p
	x.Parent = p.Parent
	if p.Parent != nil {
		if p.Parent.Children[0] == p {
			p.Parent.Children[0] = x
		} else {
			p.Parent.Children[1] = x
		}
	}
	p.Parent = x

	return x
}

// rotateZigZagRightLeft is the right-left double rotation (a zig-zag).
//
// Step 1: a zig right rotation of p and x.
//
//	   g
//	  / \
//	 D   p
//	    / \
//	   x   C
//	  / \
//	 A   B
//
//	After the right rotation:
//	     g
//	    / \
//	   D   x
//	      / \
//	     A   p
//	        / \
//	       B   C
//
// Step 2: a zig left rotation of g and x.
//
//	    x
//	   / \
//	  g   p
//	 /   / \
//	D   B   C
func rotateZigZagRightLeft[K comparable, V any](g *node[K, V]) *node[K, V] {
	if g == nil || g.Children[1] == nil || g.Children[1].Children[0] == nil {
		return g
	}
	p := g.Children[1]
	x := p.Children[0]

	// Right Rotate "p".
	p.Children[0] = x.Children[1]
	if p.Children[0] != nil {
		p.Children[0].Parent = p
	}
	x.Children[1] = p
	x.Parent = p.Parent
	if p.Parent != nil {
		if p.Parent.Children[0] == p {
			p.Parent.Children[0] = x
		} else {
			p.Parent.Children[1] = x
		}
	}
	p.Parent = x

	// Left Rotate "g".
	g.Children[1] = x.Children[0]
	if g.Children[1] != nil {
		g.Children[1].Parent = g
	}
	x.Children[0] = g
	x.Parent = g.Parent
	if g.Parent != nil {
		if g.Parent.Children[0] == g {
			g.Parent.Children[0] = x
		} else {
			g.Parent.Children[1] = x
		}
	}
	g.Parent = x

	return x
}

// rotateZigZagLeftRight is the left-right double rotation (a zig-zag).
//
// Step 1: a zig left rotation of p and x.
//
//	     g
//	    / \
//	   p   C
//	    \
//	     x
//	    / \
//	   A   B
//
//	After the left rotation:
//	      g
//	     / \
//	    x   C
//	   / \
//	  p   B
//	 /
//	A
//
// Step 2: a zig right rotation of g and x.
//
//	    x
//	   / \
//	  p   g
//	 /   / \
//	A   B   C
func rotateZigZagLeftRight[K comparable, V any](g *node[K, V]) *node[K, V] {
	if g == nil || g.Children[0] == nil || g.Children[0].Children[1] == nil {
		return g
	}

	p := g.Children[0]
	x := p.Children[1]

	// Left Rotate "p".
	p.Children[1] = x.Children[0]
	if p.Children[1] != nil {
		p.Children[1].Parent = p
	}
	x.Children[0] = p
	x.Parent = p.Parent
	if p.Parent != nil {
		if p.Parent.Children[0] == p {
			p.Parent.Children[0] = x
		} else {
			p.Parent.Children[1] = x
		}
	}
	p.Parent = x

	// Right Rotate "g"
	g.Children[0] = x.Children[1]
	if g.Children[0] != nil {
		g.Children[0].Parent = g
	}
	x.Children[1] = g
	x.Parent = g.Parent
	if g.Parent != nil {
		if g.Parent.Children[0] == g {
			g.Parent.Children[0] = x
		} else {
			g.Parent.Children[1] = x
		}
	}
	g.Parent = x

	return x
}
