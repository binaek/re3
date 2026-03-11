package re3

import (
	"fmt"
	"sort"
)

type node interface {
	Nullable() bool
	Derivative(char rune) node
	Equals(other node) bool
	Reverse() node
	String() string // Crucial for sorting and deduplicating states
}

// --- BASE NODES ---
type falseNode struct{}

func (n *falseNode) Nullable() bool            { return false }
func (n *falseNode) Derivative(char rune) node { return n }
func (n *falseNode) Equals(other node) bool    { _, ok := other.(*falseNode); return ok }
func (n *falseNode) Reverse() node             { return n }
func (n *falseNode) String() string            { return "False" }

type emptyNode struct{}

func (n *emptyNode) Nullable() bool            { return true }
func (n *emptyNode) Derivative(char rune) node { return &falseNode{} }
func (n *emptyNode) Equals(other node) bool    { _, ok := other.(*emptyNode); return ok }
func (n *emptyNode) Reverse() node             { return n }
func (n *emptyNode) String() string            { return "Empty" }

type literalNode struct{ Value rune }

func (n *literalNode) Nullable() bool { return false }
func (n *literalNode) Derivative(char rune) node {
	if char == n.Value {
		return &emptyNode{}
	}
	return &falseNode{}
}
func (n *literalNode) Equals(other node) bool {
	o, ok := other.(*literalNode)
	return ok && n.Value == o.Value
}
func (n *literalNode) Reverse() node  { return n }
func (n *literalNode) String() string { return fmt.Sprintf("Lit(%c)", n.Value) }

type anyNode struct{}

func (n *anyNode) Nullable() bool            { return false }
func (n *anyNode) Derivative(char rune) node {
	// Match any rune except newline, aligning with Go regexp (dot does not match \n by default).
	if char == '\n' {
		return &falseNode{}
	}
	return &emptyNode{}
}
func (n *anyNode) Equals(other node) bool    { _, ok := other.(*anyNode); return ok }
func (n *anyNode) Reverse() node             { return n }
func (n *anyNode) String() string            { return "Any" }

type charClassNode struct{ Class string }

func (n *charClassNode) Nullable() bool { return false }
func (n *charClassNode) Derivative(char rune) node {
	p := parseCharClass(n.Class)
	if char < 256 && p[char] {
		return &emptyNode{}
	}
	return &falseNode{}
}
func (n *charClassNode) Equals(other node) bool {
	o, ok := other.(*charClassNode)
	return ok && n.Class == o.Class
}
func (n *charClassNode) Reverse() node  { return n }
func (n *charClassNode) String() string { return fmt.Sprintf("Class(%s)", n.Class) }

// --- SET-FLATTENED BOOLEAN OPERATORS ---

type unionNode struct{ Left, Right node }

func flattenUnion(n node) []node {
	if u, ok := n.(*unionNode); ok {
		return append(flattenUnion(u.Left), flattenUnion(u.Right)...)
	}
	if _, ok := n.(*falseNode); ok {
		return nil // Drop False nodes instantly
	}
	return []node{n}
}

func newUnionNode(left, right node) node {
	// 1. Flatten the entire nested structure into a slice
	nodes := append(flattenUnion(left), flattenUnion(right)...)
	if len(nodes) == 0 {
		return &falseNode{}
	}

	// 2. Deduplicate mathematically identical states
	var unique []node
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
		res = &unionNode{Left: unique[i], Right: res}
	}
	return res
}

func (n *unionNode) Nullable() bool { return n.Left.Nullable() || n.Right.Nullable() }
func (n *unionNode) Derivative(char rune) node {
	return newUnionNode(n.Left.Derivative(char), n.Right.Derivative(char))
}
func (n *unionNode) Equals(other node) bool {
	o, ok := other.(*unionNode)
	return ok && ((n.Left.Equals(o.Left) && n.Right.Equals(o.Right)) || (n.Left.Equals(o.Right) && n.Right.Equals(o.Left)))
}
func (n *unionNode) Reverse() node { return newUnionNode(n.Left.Reverse(), n.Right.Reverse()) }
func (n *unionNode) String() string {
	return fmt.Sprintf("Union(%s,%s)", n.Left.String(), n.Right.String())
}

type intersectNode struct{ Left, Right node }

func flattenIntersect(n node) []node {
	if i, ok := n.(*intersectNode); ok {
		return append(flattenIntersect(i.Left), flattenIntersect(i.Right)...)
	}
	return []node{n}
}

func newIntersectNode(left, right node) node {
	if _, ok := left.(*falseNode); ok {
		return &falseNode{}
	}
	if _, ok := right.(*falseNode); ok {
		return &falseNode{}
	}

	nodes := append(flattenIntersect(left), flattenIntersect(right)...)

	var unique []node
	for _, n := range nodes {
		if _, ok := n.(*falseNode); ok {
			return &falseNode{}
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
		res = &intersectNode{Left: unique[i], Right: res}
	}
	return res
}

func (n *intersectNode) Nullable() bool { return n.Left.Nullable() && n.Right.Nullable() }
func (n *intersectNode) Derivative(char rune) node {
	return newIntersectNode(n.Left.Derivative(char), n.Right.Derivative(char))
}
func (n *intersectNode) Equals(other node) bool {
	o, ok := other.(*intersectNode)
	return ok && ((n.Left.Equals(o.Left) && n.Right.Equals(o.Right)) || (n.Left.Equals(o.Right) && n.Right.Equals(o.Left)))
}
func (n *intersectNode) Reverse() node { return newIntersectNode(n.Left.Reverse(), n.Right.Reverse()) }
func (n *intersectNode) String() string {
	return fmt.Sprintf("Int(%s,%s)", n.Left.String(), n.Right.String())
}

type complementNode struct{ Child node }

func newComplementNode(child node) node {
	if inner, ok := child.(*complementNode); ok {
		return inner.Child
	}
	return &complementNode{child}
}
func (n *complementNode) Nullable() bool { return !n.Child.Nullable() }
func (n *complementNode) Derivative(char rune) node {
	return newComplementNode(n.Child.Derivative(char))
}
func (n *complementNode) Equals(other node) bool {
	o, ok := other.(*complementNode)
	return ok && n.Child.Equals(o.Child)
}
func (n *complementNode) Reverse() node  { return newComplementNode(n.Child.Reverse()) }
func (n *complementNode) String() string { return fmt.Sprintf("Comp(%s)", n.Child.String()) }

// --- CONCAT, STAR & GROUPS ---
type concatNode struct{ Left, Right node }

func newConcatNode(left, right node) node {
	if _, ok := right.(*falseNode); ok {
		return &falseNode{}
	}
	if _, ok := left.(*falseNode); ok {
		return &falseNode{}
	}
	if _, ok := left.(*emptyNode); ok {
		return right
	}
	if _, ok := right.(*emptyNode); ok {
		return left
	}
	return &concatNode{left, right}
}
func (n *concatNode) Nullable() bool { return n.Left.Nullable() && n.Right.Nullable() }
func (n *concatNode) Derivative(char rune) node {
	leftDerivConcat := newConcatNode(n.Left.Derivative(char), n.Right)
	if n.Left.Nullable() {
		return newUnionNode(leftDerivConcat, n.Right.Derivative(char))
	}
	return leftDerivConcat
}
func (n *concatNode) Equals(other node) bool {
	o, ok := other.(*concatNode)
	return ok && n.Left.Equals(o.Left) && n.Right.Equals(o.Right)
}
func (n *concatNode) Reverse() node { return newConcatNode(n.Right.Reverse(), n.Left.Reverse()) }
func (n *concatNode) String() string {
	return fmt.Sprintf("Cat(%s,%s)", n.Left.String(), n.Right.String())
}

type starNode struct{ Child node }

func (n *starNode) Nullable() bool            { return true }
func (n *starNode) Derivative(char rune) node { return newConcatNode(n.Child.Derivative(char), n) }
func (n *starNode) Equals(other node) bool {
	o, ok := other.(*starNode)
	return ok && n.Child.Equals(o.Child)
}
func (n *starNode) Reverse() node  { return &starNode{Child: n.Child.Reverse()} }
func (n *starNode) String() string { return fmt.Sprintf("Star(%s)", n.Child.String()) }

// repeatNode is A{min,max}; bounds are inclusive, max >= 0. For A{n,} we use Concat(Repeat(A,n,n), Star(A)).
type repeatNode struct {
	Child node
	Min   int
	Max   int
}

func newRepeatNode(child node, min, max int) node {
	if max == 0 {
		return &emptyNode{}
	}
	if min == 1 && max == 1 {
		return child
	}
	if _, isFalse := child.(*falseNode); isFalse {
		if min == 0 {
			return &emptyNode{}
		}
		return &falseNode{}
	}
	if _, isEmpty := child.(*emptyNode); isEmpty {
		return &emptyNode{}
	}
	return &repeatNode{Child: child, Min: min, Max: max}
}

func (n *repeatNode) String() string {
	return fmt.Sprintf("Repeat(%s, %d, %d)", n.Child, n.Min, n.Max)
}

func (n *repeatNode) Nullable() bool {
	return n.Min == 0 || n.Child.Nullable()
}

func (n *repeatNode) Derivative(char rune) node {
	if n.Max == 0 {
		return &falseNode{}
	}
	nextMin := n.Min - 1
	if nextMin < 0 {
		nextMin = 0
	}
	nextMax := n.Max - 1
	nextRepeat := newRepeatNode(n.Child, nextMin, nextMax)
	derivChild := n.Child.Derivative(char)
	res := newConcatNode(derivChild, nextRepeat)
	if n.Child.Nullable() && n.Max > 0 {
		res = newUnionNode(res, nextRepeat.Derivative(char))
	}
	return res
}

func (n *repeatNode) Equals(other node) bool {
	o, ok := other.(*repeatNode)
	return ok && n.Min == o.Min && n.Max == o.Max && n.Child.Equals(o.Child)
}

func (n *repeatNode) Reverse() node {
	return newRepeatNode(n.Child.Reverse(), n.Min, n.Max)
}

type groupNode struct {
	GroupID int
	Child   node
}

func (n *groupNode) Nullable() bool            { return n.Child.Nullable() }
func (n *groupNode) Derivative(char rune) node { return n.Child.Derivative(char) }
func (n *groupNode) Equals(other node) bool {
	o, ok := other.(*groupNode)
	return ok && n.GroupID == o.GroupID && n.Child.Equals(o.Child)
}
func (n *groupNode) Reverse() node  { return &groupNode{GroupID: n.GroupID, Child: n.Child.Reverse()} }
func (n *groupNode) String() string { return fmt.Sprintf("Group%d(%s)", n.GroupID, n.Child.String()) }

// lookAheadNode is (?=R). Zero-width; does not consume input. Foundation for TDFA.
type lookAheadNode struct{ Child node }

func (n *lookAheadNode) Nullable() bool            { return n.Child.Nullable() }
func (n *lookAheadNode) Derivative(r rune) node   { return &lookAheadNode{Child: n.Child.Derivative(r)} }
func (n *lookAheadNode) Equals(other node) bool {
	o, ok := other.(*lookAheadNode)
	return ok && n.Child.Equals(o.Child)
}
func (n *lookAheadNode) Reverse() node  { return n }
func (n *lookAheadNode) String() string { return fmt.Sprintf("LookAhead(%s)", n.Child.String()) }

// lookBehindNode is (?<=R). Zero-width; foundation for TDFA.
type lookBehindNode struct{ Child node }

func (n *lookBehindNode) Nullable() bool            { return n.Child.Nullable() }
func (n *lookBehindNode) Derivative(r rune) node   { return &lookBehindNode{Child: n.Child.Derivative(r)} }
func (n *lookBehindNode) Equals(other node) bool {
	o, ok := other.(*lookBehindNode)
	return ok && n.Child.Equals(o.Child)
}
func (n *lookBehindNode) Reverse() node  { return n }
func (n *lookBehindNode) String() string { return fmt.Sprintf("LookBehind(%s)", n.Child.String()) }

// --- TDFA: capture boundaries (zero-width) ---

// tagNode marks a capture boundary for the TDFA. Zero-width; does not consume input.
// Id is the capture group number (1-based). IsStart true = open (set start index), false = close (set end index).
type tagNode struct {
	Id      int
	IsStart bool
}

func (n *tagNode) Nullable() bool            { return true }
func (n *tagNode) Derivative(rune) node      { return &emptyNode{} }
func (n *tagNode) Equals(other node) bool {
	o, ok := other.(*tagNode)
	return ok && n.Id == o.Id && n.IsStart == o.IsStart
}
func (n *tagNode) Reverse() node { return &tagNode{Id: n.Id, IsStart: !n.IsStart} }
func (n *tagNode) String() string {
	if n.IsStart {
		return fmt.Sprintf("Tag(%d,start)", n.Id)
	}
	return fmt.Sprintf("Tag(%d,end)", n.Id)
}
