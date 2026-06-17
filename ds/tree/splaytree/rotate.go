package splaytree

/*
 * 右单旋（Zig 旋转）
 *
 *   Before Rotation:
 *        g
 *       / \
 *      p   C
 *     / \
 *    A   B
 *
 *   After Rotation:
 *        p
 *       / \
 *      A   g
 *         / \
 *        B   C
 */
func rotateZigRight[K comparable, V any](g *Node[K, V]) *Node[K, V] {
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

/*
 * 左单旋（Zig 旋转）
 *
 *   Before Rotation:
 *        g
 *       / \
 *      A   p
 *         / \
 *        B   C
 *
 *   After Rotation:
 *        p
 *       / \
 *      g   C
 *     / \
 *    A   B
 */
func rotateZigLeft[K comparable, V any](g *Node[K, V]) *Node[K, V] {
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

/*
 * 右双旋（Zig-Zig 旋转）
 *
 * Step 1: 先对 g 和 p 进行 **Zig 右旋**
 *         g
 *        / \
 *       p   D
 *      / \
 *     x   C
 *    / \
 *   A   B
 *
 *  Zig 右旋后：
 *       p
 *      / \
 *     x   g
 *    / \  / \
 *   A   B C  D
 *
 * Step 2: 再对 p 和 x 进行 **Zig 右旋**
 *       x
 *      / \
 *     A   p
 *        / \
 *       B   g
 *          / \
 *         C   D
 */
func rotateZigZigRight[K comparable, V any](g *Node[K, V]) *Node[K, V] {
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

/*
 * 左双旋（Zig-Zig 旋转）
 *
 * Step 1: 先对 g 和 p 进行 **Zig 左旋**
 *     g
 *    / \
 *   A   p
 *      / \
 *     B   x
 *        / \
 *       C   D
 *
 *  Zig 左旋后：
 *       p
 *      / \
 *     g   x
 *    / \  / \
 *   A   B C  D
 *
 * Step 2: 再对 p 和 x 进行 **Zig 左旋**
 *       x
 *      / \
 *     p   D
 *    / \
 *   g   C
 *  / \
 * A   B
 */
func rotateZigZigLeft[K comparable, V any](g *Node[K, V]) *Node[K, V] {
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

/*
 * 右-左双旋（Zig-Zag 旋转）
 *
 * Step 1: 先对 p 和 x 进行 **Zig 右旋**
 *     g
 *    / \
 *   D   p
 *      / \
 *     x   C
 *    / \
 *   A   B
 *
 *  右旋后：
 *       g
 *      / \
 *     D   x
 *        / \
 *       A   p
 *          / \
 *         B   C
 *
 * Step 2: 再对 g 和 x 进行 **Zig 左旋**
 *       x
 *      / \
 *     g   p
 *    /   / \
 *   D   B   C
 */
func rotateZigZagRightLeft[K comparable, V any](g *Node[K, V]) *Node[K, V] {
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

/*
 * 左-右双旋（Zig-Zag 旋转）
 *
 * Step 1: 先对 p 和 x 进行 **Zig 左旋**
 *       g
 *      / \
 *     p   C
 *      \
 *       x
 *      / \
 *     A   B
 *
 *  左旋后：
 *       g
 *      / \
 *     x   C
 *    / \
 *   p   B
 *  /
 * A
 *
 * Step 2: 再对 g 和 x 进行 **Zig 右旋**
 *       x
 *      / \
 *     p   g
 *    /   / \
 *   A   B   C
 */
func rotateZigZagLeftRight[K comparable, V any](g *Node[K, V]) *Node[K, V] {
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
