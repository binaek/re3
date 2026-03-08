package re3

import (
	"fmt"
	"sort"
)

type Node interface {
	Nullable() bool
	Derivative(char rune) Node
	Equals(other Node) bool
	Reverse() Node
	String() string // Crucial for sorting and deduplicating states
}

// --- BASE NODES ---
type FalseNode struct{}

func (n *FalseNode) Nullable() bool            { return false }
func (n *FalseNode) Derivative(char rune) Node { return n }
func (n *FalseNode) Equals(other Node) bool    { _, ok := other.(*FalseNode); return ok }
func (n *FalseNode) Reverse() Node             { return n }
func (n *FalseNode) String() string            { return "False" }

type EmptyNode struct{}

func (n *EmptyNode) Nullable() bool            { return true }
func (n *EmptyNode) Derivative(char rune) Node { return &FalseNode{} }
func (n *EmptyNode) Equals(other Node) bool    { _, ok := other.(*EmptyNode); return ok }
func (n *EmptyNode) Reverse() Node             { return n }
func (n *EmptyNode) String() string            { return "Empty" }

type LiteralNode struct{ Value rune }

func (n *LiteralNode) Nullable() bool { return false }
func (n *LiteralNode) Derivative(char rune) Node {
	if char == n.Value {
		return &EmptyNode{}
	}
	return &FalseNode{}
}
func (n *LiteralNode) Equals(other Node) bool {
	o, ok := other.(*LiteralNode)
	return ok && n.Value == o.Value
}
func (n *LiteralNode) Reverse() Node  { return n }
func (n *LiteralNode) String() string { return fmt.Sprintf("Lit(%c)", n.Value) }

type AnyNode struct{}

func (n *AnyNode) Nullable() bool            { return false }
func (n *AnyNode) Derivative(char rune) Node { return &EmptyNode{} }
func (n *AnyNode) Equals(other Node) bool    { _, ok := other.(*AnyNode); return ok }
func (n *AnyNode) Reverse() Node             { return n }
func (n *AnyNode) String() string            { return "Any" }

type CharClassNode struct{ Class string }

func (n *CharClassNode) Nullable() bool { return false }
func (n *CharClassNode) Derivative(char rune) Node {
	p := parseCharClass(n.Class)
	if char < 256 && p[char] {
		return &EmptyNode{}
	}
	return &FalseNode{}
}
func (n *CharClassNode) Equals(other Node) bool {
	o, ok := other.(*CharClassNode)
	return ok && n.Class == o.Class
}
func (n *CharClassNode) Reverse() Node  { return n }
func (n *CharClassNode) String() string { return fmt.Sprintf("Class(%s)", n.Class) }

// --- SET-FLATTENED BOOLEAN OPERATORS ---

type UnionNode struct{ Left, Right Node }

func flattenUnion(n Node) []Node {
	if u, ok := n.(*UnionNode); ok {
		return append(flattenUnion(u.Left), flattenUnion(u.Right)...)
	}
	if _, ok := n.(*FalseNode); ok {
		return nil // Drop False nodes instantly
	}
	return []Node{n}
}

func NewUnionNode(left, right Node) Node {
	// 1. Flatten the entire nested structure into a slice
	nodes := append(flattenUnion(left), flattenUnion(right)...)
	if len(nodes) == 0 {
		return &FalseNode{}
	}

	// 2. Deduplicate mathematically identical states
	var unique []Node
	for _, n := range nodes {
		exists := false
		for _, u := range unique {
			if n.Equals(u) {
				exists = true
				break
			}
		}
		if !exists {
			unique = append(unique, n)
		}
	}

	if len(unique) == 1 {
		return unique[0]
	}

	// 3. Sort them to guarantee A|B is structurally identical to B|A
	sort.Slice(unique, func(i, j int) bool {
		return unique[i].String() < unique[j].String()
	})

	// 4. Rebuild as a strictly right-heavy tree
	res := unique[len(unique)-1]
	for i := len(unique) - 2; i >= 0; i-- {
		res = &UnionNode{Left: unique[i], Right: res}
	}
	return res
}

func (n *UnionNode) Nullable() bool { return n.Left.Nullable() || n.Right.Nullable() }
func (n *UnionNode) Derivative(char rune) Node {
	return NewUnionNode(n.Left.Derivative(char), n.Right.Derivative(char))
}
func (n *UnionNode) Equals(other Node) bool {
	o, ok := other.(*UnionNode)
	return ok && ((n.Left.Equals(o.Left) && n.Right.Equals(o.Right)) || (n.Left.Equals(o.Right) && n.Right.Equals(o.Left)))
}
func (n *UnionNode) Reverse() Node { return NewUnionNode(n.Left.Reverse(), n.Right.Reverse()) }
func (n *UnionNode) String() string {
	return fmt.Sprintf("Union(%s,%s)", n.Left.String(), n.Right.String())
}

type IntersectNode struct{ Left, Right Node }

func flattenIntersect(n Node) []Node {
	if i, ok := n.(*IntersectNode); ok {
		return append(flattenIntersect(i.Left), flattenIntersect(i.Right)...)
	}
	return []Node{n}
}

func NewIntersectNode(left, right Node) Node {
	if _, ok := left.(*FalseNode); ok {
		return &FalseNode{}
	}
	if _, ok := right.(*FalseNode); ok {
		return &FalseNode{}
	}

	nodes := append(flattenIntersect(left), flattenIntersect(right)...)

	var unique []Node
	for _, n := range nodes {
		if _, ok := n.(*FalseNode); ok {
			return &FalseNode{}
		} // A & False = False
		exists := false
		for _, u := range unique {
			if n.Equals(u) {
				exists = true
				break
			}
		}
		if !exists {
			unique = append(unique, n)
		}
	}

	if len(unique) == 1 {
		return unique[0]
	}

	sort.Slice(unique, func(i, j int) bool {
		return unique[i].String() < unique[j].String()
	})

	res := unique[len(unique)-1]
	for i := len(unique) - 2; i >= 0; i-- {
		res = &IntersectNode{Left: unique[i], Right: res}
	}
	return res
}

func (n *IntersectNode) Nullable() bool { return n.Left.Nullable() && n.Right.Nullable() }
func (n *IntersectNode) Derivative(char rune) Node {
	return NewIntersectNode(n.Left.Derivative(char), n.Right.Derivative(char))
}
func (n *IntersectNode) Equals(other Node) bool {
	o, ok := other.(*IntersectNode)
	return ok && ((n.Left.Equals(o.Left) && n.Right.Equals(o.Right)) || (n.Left.Equals(o.Right) && n.Right.Equals(o.Left)))
}
func (n *IntersectNode) Reverse() Node { return NewIntersectNode(n.Left.Reverse(), n.Right.Reverse()) }
func (n *IntersectNode) String() string {
	return fmt.Sprintf("Int(%s,%s)", n.Left.String(), n.Right.String())
}

type ComplementNode struct{ Child Node }

func NewComplementNode(child Node) Node {
	if inner, ok := child.(*ComplementNode); ok {
		return inner.Child
	}
	return &ComplementNode{child}
}
func (n *ComplementNode) Nullable() bool { return !n.Child.Nullable() }
func (n *ComplementNode) Derivative(char rune) Node {
	return NewComplementNode(n.Child.Derivative(char))
}
func (n *ComplementNode) Equals(other Node) bool {
	o, ok := other.(*ComplementNode)
	return ok && n.Child.Equals(o.Child)
}
func (n *ComplementNode) Reverse() Node  { return NewComplementNode(n.Child.Reverse()) }
func (n *ComplementNode) String() string { return fmt.Sprintf("Comp(%s)", n.Child.String()) }

// --- CONCAT, STAR & GROUPS ---
type ConcatNode struct{ Left, Right Node }

func NewConcatNode(left, right Node) Node {
	if _, ok := right.(*FalseNode); ok {
		return &FalseNode{}
	}
	if _, ok := left.(*FalseNode); ok {
		return &FalseNode{}
	}
	if _, ok := left.(*EmptyNode); ok {
		return right
	}
	if _, ok := right.(*EmptyNode); ok {
		return left
	}
	return &ConcatNode{left, right}
}
func (n *ConcatNode) Nullable() bool { return n.Left.Nullable() && n.Right.Nullable() }
func (n *ConcatNode) Derivative(char rune) Node {
	leftDerivConcat := NewConcatNode(n.Left.Derivative(char), n.Right)
	if n.Left.Nullable() {
		return NewUnionNode(leftDerivConcat, n.Right.Derivative(char))
	}
	return leftDerivConcat
}
func (n *ConcatNode) Equals(other Node) bool {
	o, ok := other.(*ConcatNode)
	return ok && n.Left.Equals(o.Left) && n.Right.Equals(o.Right)
}
func (n *ConcatNode) Reverse() Node { return NewConcatNode(n.Right.Reverse(), n.Left.Reverse()) }
func (n *ConcatNode) String() string {
	return fmt.Sprintf("Cat(%s,%s)", n.Left.String(), n.Right.String())
}

type StarNode struct{ Child Node }

func (n *StarNode) Nullable() bool            { return true }
func (n *StarNode) Derivative(char rune) Node { return NewConcatNode(n.Child.Derivative(char), n) }
func (n *StarNode) Equals(other Node) bool {
	o, ok := other.(*StarNode)
	return ok && n.Child.Equals(o.Child)
}
func (n *StarNode) Reverse() Node  { return &StarNode{Child: n.Child.Reverse()} }
func (n *StarNode) String() string { return fmt.Sprintf("Star(%s)", n.Child.String()) }

type GroupNode struct {
	GroupID int
	Child   Node
}

func (n *GroupNode) Nullable() bool            { return n.Child.Nullable() }
func (n *GroupNode) Derivative(char rune) Node { return n.Child.Derivative(char) }
func (n *GroupNode) Equals(other Node) bool {
	o, ok := other.(*GroupNode)
	return ok && n.GroupID == o.GroupID && n.Child.Equals(o.Child)
}
func (n *GroupNode) Reverse() Node  { return &GroupNode{GroupID: n.GroupID, Child: n.Child.Reverse()} }
func (n *GroupNode) String() string { return fmt.Sprintf("Group%d(%s)", n.GroupID, n.Child.String()) }
