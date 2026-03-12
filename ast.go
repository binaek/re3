package re3

import (
	"fmt"
	"reflect"
)

type node interface {
	Nullable(ctx matchContext) bool
	Derivative(b byte, ctx matchContext) node
	Equals(other node) bool
	Reverse() node
	String() string // Crucial for sorting and deduplicating states
}

// --- BASE NODES ---
type falseNode struct{}

func (n *falseNode) Nullable(_ matchContext) bool            { return false }
func (n *falseNode) Derivative(b byte, _ matchContext) node { return n }
func (n *falseNode) Equals(other node) bool    { _, ok := other.(*falseNode); return ok }
func (n *falseNode) Reverse() node             { return n }
func (n *falseNode) String() string            { return "False" }

type emptyNode struct{}

func (n *emptyNode) Nullable(_ matchContext) bool            { return true }
func (n *emptyNode) Derivative(b byte, _ matchContext) node { return &falseNode{} }
func (n *emptyNode) Equals(other node) bool    { _, ok := other.(*emptyNode); return ok }
func (n *emptyNode) Reverse() node             { return n }
func (n *emptyNode) String() string            { return "Empty" }

type literalNode struct{ Value byte }

func (n *literalNode) Nullable(_ matchContext) bool { return false }
func (n *literalNode) Derivative(b byte, _ matchContext) node {
	if b == n.Value {
		return &emptyNode{}
	}
	return &falseNode{}
}
func (n *literalNode) Equals(other node) bool {
	o, ok := other.(*literalNode)
	return ok && n.Value == o.Value
}
func (n *literalNode) Reverse() node  { return n }
func (n *literalNode) String() string { return fmt.Sprintf("Lit(0x%02x)", n.Value) }

type anyNode struct{}

func (n *anyNode) Nullable(_ matchContext) bool            { return false }
func (n *anyNode) Derivative(b byte, _ matchContext) node {
	// Match any rune except newline, aligning with Go regexp (dot does not match \n by default).
	if b == '\n' {
		return &falseNode{}
	}
	return &emptyNode{}
}
func (n *anyNode) Equals(other node) bool    { _, ok := other.(*anyNode); return ok }
func (n *anyNode) Reverse() node             { return n }
func (n *anyNode) String() string            { return "Any" }

// anyByteNode is an internal helper used for unanchored search pre-scan.
// It consumes exactly one byte (including '\n').
type anyByteNode struct{}

func (n *anyByteNode) Nullable(_ matchContext) bool            { return false }
func (n *anyByteNode) Derivative(b byte, _ matchContext) node  { return &emptyNode{} }
func (n *anyByteNode) Equals(other node) bool                  { _, ok := other.(*anyByteNode); return ok }
func (n *anyByteNode) Reverse() node                           { return n }
func (n *anyByteNode) String() string                          { return "AnyByte" }

type charClassNode struct {
	Class string
	Pred  predicate
}

func (n *charClassNode) Nullable(_ matchContext) bool { return false }
func (n *charClassNode) Derivative(b byte, _ matchContext) node {
	p := n.Pred
	if p == (predicate{}) {
		p = parseCharClass(n.Class)
	}
	if p[b] {
		return &emptyNode{}
	}
	return &falseNode{}
}
func (n *charClassNode) Equals(other node) bool {
	o, ok := other.(*charClassNode)
	if !ok {
		return false
	}
	if n.Class != "" || o.Class != "" {
		return n.Class == o.Class
	}
	return n.Pred == o.Pred
}
func (n *charClassNode) Reverse() node  { return n }
func (n *charClassNode) String() string {
	if n.Class != "" {
		return fmt.Sprintf("Class(%s)", n.Class)
	}
	return "Class(<bytes>)"
}

// --- SET-FLATTENED BOOLEAN OPERATORS ---

type unionNode struct{ Left, Right node }

func newUnionNode(left, right node) node {
	if _, ok := left.(*falseNode); ok {
		return right
	}
	if _, ok := right.(*falseNode); ok {
		return left
	}
	if left == right {
		return left
	}
	if shouldSwapCommutativeNodes(left, right) {
		left, right = right, left
	}
	return &unionNode{Left: left, Right: right}
}

func (n *unionNode) Nullable(ctx matchContext) bool { return n.Left.Nullable(ctx) || n.Right.Nullable(ctx) }
func (n *unionNode) Derivative(b byte, ctx matchContext) node {
	return newUnionNode(n.Left.Derivative(b, ctx), n.Right.Derivative(b, ctx))
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

func newIntersectNode(left, right node) node {
	if _, ok := left.(*falseNode); ok {
		return &falseNode{}
	}
	if _, ok := right.(*falseNode); ok {
		return &falseNode{}
	}
	if left == right {
		return left
	}
	if shouldSwapCommutativeNodes(left, right) {
		left, right = right, left
	}
	return &intersectNode{Left: left, Right: right}
}

func shouldSwapCommutativeNodes(left, right node) bool {
	lp := nodePointerID(left)
	rp := nodePointerID(right)
	if lp != rp {
		return lp > rp
	}
	lf := fingerprintNode(left)
	rf := fingerprintNode(right)
	return lf > rf
}

func nodePointerID(n node) uintptr {
	return reflect.ValueOf(n).Pointer()
}

func (n *intersectNode) Nullable(ctx matchContext) bool { return n.Left.Nullable(ctx) && n.Right.Nullable(ctx) }
func (n *intersectNode) Derivative(b byte, ctx matchContext) node {
	return newIntersectNode(n.Left.Derivative(b, ctx), n.Right.Derivative(b, ctx))
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
func (n *complementNode) Nullable(ctx matchContext) bool { return !n.Child.Nullable(ctx) }
func (n *complementNode) Derivative(b byte, ctx matchContext) node {
	return newComplementNode(n.Child.Derivative(b, ctx))
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
func (n *concatNode) Nullable(ctx matchContext) bool { return n.Left.Nullable(ctx) && n.Right.Nullable(ctx) }
func (n *concatNode) Derivative(b byte, ctx matchContext) node {
	leftDerivConcat := newConcatNode(n.Left.Derivative(b, ctx), n.Right)
	if n.Left.Nullable(ctx) {
		return newUnionNode(leftDerivConcat, n.Right.Derivative(b, ctx))
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

func (n *starNode) Nullable(_ matchContext) bool            { return true }
func (n *starNode) Derivative(b byte, ctx matchContext) node { return newConcatNode(n.Child.Derivative(b, ctx), n) }
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

func (n *repeatNode) Nullable(ctx matchContext) bool {
	return n.Min == 0 || n.Child.Nullable(ctx)
}

func (n *repeatNode) Derivative(b byte, ctx matchContext) node {
	if n.Max == 0 {
		return &falseNode{}
	}

	nextMin := n.Min - 1
	if nextMin < 0 {
		nextMin = 0
	}
	nextMax := n.Max - 1

	nextRepeat := newRepeatNode(n.Child, nextMin, nextMax)
	derivChild := n.Child.Derivative(b, ctx)

	if !n.Child.Nullable(ctx) {
		return newConcatNode(derivChild, nextRepeat)
	}

	// Iteratively unroll nullable repeats to avoid recursive derivation explosion.
	unionTree := make([]node, 0, nextMax+1)
	unionTree = append(unionTree, newConcatNode(derivChild, nextRepeat))

	currentMin := nextMin
	currentMax := nextMax
	for currentMax > 0 {
		currentMin--
		if currentMin < 0 {
			currentMin = 0
		}
		currentMax--
		unionTree = append(unionTree, newConcatNode(derivChild, newRepeatNode(n.Child, currentMin, currentMax)))
	}

	if len(unionTree) == 1 {
		return unionTree[0]
	}
	res := unionTree[len(unionTree)-1]
	for i := len(unionTree) - 2; i >= 0; i-- {
		res = newUnionNode(unionTree[i], res)
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

func (n *groupNode) Nullable(ctx matchContext) bool            { return n.Child.Nullable(ctx) }
func (n *groupNode) Derivative(b byte, ctx matchContext) node { return n.Child.Derivative(b, ctx) }
func (n *groupNode) Equals(other node) bool {
	o, ok := other.(*groupNode)
	return ok && n.GroupID == o.GroupID && n.Child.Equals(o.Child)
}
func (n *groupNode) Reverse() node  { return &groupNode{GroupID: n.GroupID, Child: n.Child.Reverse()} }
func (n *groupNode) String() string { return fmt.Sprintf("Group%d(%s)", n.GroupID, n.Child.String()) }

// lookAheadNode is (?=R). Zero-width; does not consume input. Foundation for TDFA.
type lookAheadNode struct{ Child node }

func (n *lookAheadNode) Nullable(ctx matchContext) bool            { return n.Child.Nullable(ctx) }
func (n *lookAheadNode) Derivative(b byte, ctx matchContext) node   { return &lookAheadNode{Child: n.Child.Derivative(b, ctx)} }
func (n *lookAheadNode) Equals(other node) bool {
	o, ok := other.(*lookAheadNode)
	return ok && n.Child.Equals(o.Child)
}
func (n *lookAheadNode) Reverse() node  { return n }
func (n *lookAheadNode) String() string { return fmt.Sprintf("LookAhead(%s)", n.Child.String()) }

// lookBehindNode is (?<=R). Zero-width; foundation for TDFA.
type lookBehindNode struct{ Child node }

func (n *lookBehindNode) Nullable(ctx matchContext) bool            { return n.Child.Nullable(ctx) }
func (n *lookBehindNode) Derivative(b byte, ctx matchContext) node   { return &lookBehindNode{Child: n.Child.Derivative(b, ctx)} }
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

func (n *tagNode) Nullable(_ matchContext) bool            { return true }
func (n *tagNode) Derivative(byte, matchContext) node      { return &emptyNode{} }
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

type startNode struct{}

func (n *startNode) Nullable(ctx matchContext) bool            { return ctx.AtStart || ctx.PrevIsNewline }
func (n *startNode) Derivative(byte, matchContext) node        { return &falseNode{} }
func (n *startNode) Equals(other node) bool                    { _, ok := other.(*startNode); return ok }
func (n *startNode) Reverse() node                             { return &endNode{} }
func (n *startNode) String() string                            { return "Start" }

type endNode struct{}

func (n *endNode) Nullable(ctx matchContext) bool              { return ctx.AtEnd || ctx.NextIsNewline }
func (n *endNode) Derivative(byte, matchContext) node          { return &falseNode{} }
func (n *endNode) Equals(other node) bool                      { _, ok := other.(*endNode); return ok }
func (n *endNode) Reverse() node                               { return &startNode{} }
func (n *endNode) String() string                              { return "End" }

type beginTextNode struct{}

func (n *beginTextNode) Nullable(ctx matchContext) bool        { return ctx.AtStart }
func (n *beginTextNode) Derivative(byte, matchContext) node    { return &falseNode{} }
func (n *beginTextNode) Equals(other node) bool                { _, ok := other.(*beginTextNode); return ok }
func (n *beginTextNode) Reverse() node                         { return &endTextNode{} }
func (n *beginTextNode) String() string                        { return "BeginText" }

type endTextNode struct{}

func (n *endTextNode) Nullable(ctx matchContext) bool          { return ctx.AtEnd }
func (n *endTextNode) Derivative(byte, matchContext) node      { return &falseNode{} }
func (n *endTextNode) Equals(other node) bool                  { _, ok := other.(*endTextNode); return ok }
func (n *endTextNode) Reverse() node                           { return &beginTextNode{} }
func (n *endTextNode) String() string                          { return "EndText" }

type endTextOptionalNewlineNode struct{}

func (n *endTextOptionalNewlineNode) Nullable(ctx matchContext) bool {
	return ctx.AtEndAfterOptionalNewline
}
func (n *endTextOptionalNewlineNode) Derivative(byte, matchContext) node { return &falseNode{} }
func (n *endTextOptionalNewlineNode) Equals(other node) bool {
	_, ok := other.(*endTextOptionalNewlineNode)
	return ok
}
func (n *endTextOptionalNewlineNode) Reverse() node  { return &beginTextNode{} }
func (n *endTextOptionalNewlineNode) String() string { return "EndTextOptionalNewline" }

type wordBoundaryNode struct{}

func (n *wordBoundaryNode) Nullable(ctx matchContext) bool {
	return ctx.PrevIsWord != ctx.NextIsWord
}
func (n *wordBoundaryNode) Derivative(byte, matchContext) node { return &falseNode{} }
func (n *wordBoundaryNode) Equals(other node) bool {
	_, ok := other.(*wordBoundaryNode)
	return ok
}
func (n *wordBoundaryNode) Reverse() node  { return n }
func (n *wordBoundaryNode) String() string { return "WordBoundary" }

type notWordBoundaryNode struct{}

func (n *notWordBoundaryNode) Nullable(ctx matchContext) bool {
	return ctx.PrevIsWord == ctx.NextIsWord
}
func (n *notWordBoundaryNode) Derivative(byte, matchContext) node { return &falseNode{} }
func (n *notWordBoundaryNode) Equals(other node) bool {
	_, ok := other.(*notWordBoundaryNode)
	return ok
}
func (n *notWordBoundaryNode) Reverse() node  { return n }
func (n *notWordBoundaryNode) String() string { return "NotWordBoundary" }

func containsAssertions(n node) bool {
	switch nd := n.(type) {
	case *startNode, *endNode, *beginTextNode, *endTextNode, *endTextOptionalNewlineNode, *wordBoundaryNode, *notWordBoundaryNode:
		return true
	case *concatNode:
		return containsAssertions(nd.Left) || containsAssertions(nd.Right)
	case *unionNode:
		return containsAssertions(nd.Left) || containsAssertions(nd.Right)
	case *intersectNode:
		return containsAssertions(nd.Left) || containsAssertions(nd.Right)
	case *complementNode:
		return containsAssertions(nd.Child)
	case *starNode:
		return containsAssertions(nd.Child)
	case *repeatNode:
		return containsAssertions(nd.Child)
	case *groupNode:
		return containsAssertions(nd.Child)
	case *lookAheadNode:
		return containsAssertions(nd.Child)
	case *lookBehindNode:
		return containsAssertions(nd.Child)
	default:
		return false
	}
}

func fingerprintNode(n node) uint64 {
	const seed uint64 = 1469598103934665603
	h := seed
	switch nd := n.(type) {
	case *falseNode:
		return mixFingerprint(h, 1)
	case *emptyNode:
		return mixFingerprint(h, 2)
	case *literalNode:
		return mixFingerprint(mixFingerprint(h, 3), uint64(nd.Value))
	case *anyNode:
		return mixFingerprint(h, 4)
	case *charClassNode:
		if nd.Class != "" {
			return mixFingerprint(mixFingerprint(h, 5), hashString64(nd.Class))
		}
		return mixFingerprint(mixFingerprint(h, 5), hashPredicate64(nd.Pred))
	case *anyByteNode:
		return mixFingerprint(h, 23)
	case *unionNode:
		return mixFingerprint(mixFingerprint(mixFingerprint(h, 6), fingerprintNode(nd.Left)), fingerprintNode(nd.Right))
	case *intersectNode:
		return mixFingerprint(mixFingerprint(mixFingerprint(h, 7), fingerprintNode(nd.Left)), fingerprintNode(nd.Right))
	case *complementNode:
		return mixFingerprint(mixFingerprint(h, 8), fingerprintNode(nd.Child))
	case *concatNode:
		return mixFingerprint(mixFingerprint(mixFingerprint(h, 9), fingerprintNode(nd.Left)), fingerprintNode(nd.Right))
	case *starNode:
		return mixFingerprint(mixFingerprint(h, 10), fingerprintNode(nd.Child))
	case *repeatNode:
		h = mixFingerprint(h, 11)
		h = mixFingerprint(h, fingerprintNode(nd.Child))
		h = mixFingerprint(h, uint64(nd.Min+1))
		return mixFingerprint(h, uint64(nd.Max+1))
	case *groupNode:
		return mixFingerprint(mixFingerprint(mixFingerprint(h, 12), uint64(nd.GroupID+1)), fingerprintNode(nd.Child))
	case *lookAheadNode:
		return mixFingerprint(mixFingerprint(h, 13), fingerprintNode(nd.Child))
	case *lookBehindNode:
		return mixFingerprint(mixFingerprint(h, 14), fingerprintNode(nd.Child))
	case *tagNode:
		h = mixFingerprint(mixFingerprint(h, 15), uint64(nd.Id+1))
		if nd.IsStart {
			return mixFingerprint(h, 1)
		}
		return mixFingerprint(h, 0)
	case *startNode:
		return mixFingerprint(h, 16)
	case *endNode:
		return mixFingerprint(h, 17)
	case *beginTextNode:
		return mixFingerprint(h, 18)
	case *endTextNode:
		return mixFingerprint(h, 19)
	case *endTextOptionalNewlineNode:
		return mixFingerprint(h, 20)
	case *wordBoundaryNode:
		return mixFingerprint(h, 21)
	case *notWordBoundaryNode:
		return mixFingerprint(h, 22)
	default:
		return mixFingerprint(h, 255)
	}
}

func mixFingerprint(h, v uint64) uint64 {
	h ^= v + 0x9e3779b97f4a7c15 + (h << 6) + (h >> 2)
	return h
}

func hashString64(s string) uint64 {
	h := uint64(1469598103934665603)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

func hashPredicate64(p predicate) uint64 {
	h := uint64(1469598103934665603)
	for i := 0; i < len(p); i++ {
		if p[i] {
			h ^= uint64(i + 1)
			h *= 1099511628211
		}
	}
	return h
}
