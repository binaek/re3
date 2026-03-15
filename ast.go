package re3

import (
	"context"
	"fmt"
)

type node interface {
	Nullable(ctx context.Context, mctx matchContext) bool
	Derivative(ctx context.Context, b byte, mctx matchContext) node
	Equals(other node) bool
	Reverse() node
	String() string // Crucial for sorting and deduplicating states
	FingerPrint(ctx context.Context) uint64
}

// --- BASE NODES ---
type falseNode struct {
	fp uint64
}

func newFalseNode(_ context.Context) *falseNode {
	return &falseNode{fp: mixFingerprint(fingerprintSeed, 1)}
}

func newEmptyNode(_ context.Context) *emptyNode {
	return &emptyNode{fp: mixFingerprint(fingerprintSeed, 2)}
}

func (n *falseNode) Nullable(_ context.Context, _ matchContext) bool           { return false }
func (n *falseNode) Derivative(_ context.Context, _ byte, _ matchContext) node { return n }
func (n *falseNode) Equals(other node) bool                                    { _, ok := other.(*falseNode); return ok }
func (n *falseNode) Reverse() node                                             { return n }
func (n *falseNode) String() string                                            { return "False" }
func (n *falseNode) FingerPrint(_ context.Context) uint64 {
	if n.fp != 0 {
		return n.fp
	}
	n.fp = mixFingerprint(fingerprintSeed, 1)
	return n.fp
}

type emptyNode struct {
	fp uint64
}

func (n *emptyNode) Nullable(_ context.Context, _ matchContext) bool { return true }
func (n *emptyNode) Derivative(ctx context.Context, _ byte, _ matchContext) node {
	return newFalseNode(ctx)
}
func (n *emptyNode) Equals(other node) bool { _, ok := other.(*emptyNode); return ok }
func (n *emptyNode) Reverse() node          { return n }
func (n *emptyNode) String() string         { return "Empty" }
func (n *emptyNode) FingerPrint(_ context.Context) uint64 {
	if n.fp != 0 {
		return n.fp
	}
	n.fp = mixFingerprint(fingerprintSeed, 2)
	return n.fp
}

type literalNode struct {
	Value byte
	fp    uint64
}

func newLiteralNode(_ context.Context, value byte) *literalNode {
	return &literalNode{Value: value}
}

func (n *literalNode) Nullable(_ context.Context, _ matchContext) bool { return false }
func (n *literalNode) Derivative(ctx context.Context, b byte, _ matchContext) node {
	if b == n.Value {
		return newEmptyNode(ctx)
	}
	return newFalseNode(ctx)
}
func (n *literalNode) Equals(other node) bool {
	o, ok := other.(*literalNode)
	return ok && n.Value == o.Value
}
func (n *literalNode) Reverse() node  { return n }
func (n *literalNode) String() string { return fmt.Sprintf("Lit(0x%02x)", n.Value) }
func (n *literalNode) FingerPrint(_ context.Context) uint64 {
	if n.fp != 0 {
		return n.fp
	}
	n.fp = mixFingerprint(mixFingerprint(fingerprintSeed, 3), uint64(n.Value))
	return n.fp
}

type anyNode struct {
	fp uint64
}

func newAnyNodeSimple(_ context.Context) *anyNode {
	return &anyNode{}
}

func (n *anyNode) Nullable(_ context.Context, _ matchContext) bool { return false }
func (n *anyNode) Derivative(ctx context.Context, b byte, _ matchContext) node {
	// Match any rune except newline, aligning with Go regexp (dot does not match \n by default).
	if b == '\n' {
		return newFalseNode(ctx)
	}
	return newEmptyNode(ctx)
}
func (n *anyNode) Equals(other node) bool { _, ok := other.(*anyNode); return ok }
func (n *anyNode) Reverse() node          { return n }
func (n *anyNode) String() string         { return "Any" }
func (n *anyNode) FingerPrint(_ context.Context) uint64 {
	if n.fp != 0 {
		return n.fp
	}
	n.fp = mixFingerprint(fingerprintSeed, 4)
	return n.fp
}

// anyByteNode is an internal helper used for unanchored search pre-scan.
// It consumes exactly one byte (including '\n').
type anyByteNode struct {
	fp uint64
}

func newAnyByteNode(_ context.Context) *anyByteNode {
	return &anyByteNode{fp: mixFingerprint(fingerprintSeed, 23)}
}

func (n *anyByteNode) Nullable(_ context.Context, _ matchContext) bool { return false }
func (n *anyByteNode) Derivative(ctx context.Context, _ byte, _ matchContext) node {
	return newEmptyNode(ctx)
}
func (n *anyByteNode) Equals(other node) bool { _, ok := other.(*anyByteNode); return ok }
func (n *anyByteNode) Reverse() node          { return n }
func (n *anyByteNode) String() string         { return "AnyByte" }
func (n *anyByteNode) FingerPrint(_ context.Context) uint64 {
	if n.fp != 0 {
		return n.fp
	}
	n.fp = mixFingerprint(fingerprintSeed, 23)
	return n.fp
}

type charClassNode struct {
	Class string
	Pred  predicate
	fp    uint64
}

func newCharClassNode(_ context.Context, class string, pred predicate) *charClassNode {
	return &charClassNode{Class: class, Pred: pred}
}

func (n *charClassNode) Nullable(_ context.Context, _ matchContext) bool { return false }
func (n *charClassNode) Derivative(ctx context.Context, b byte, _ matchContext) node {
	p := n.Pred
	if p == (predicate{}) {
		p = parseCharClass(ctx, n.Class)
	}
	if p[b] {
		return newEmptyNode(ctx)
	}
	return newFalseNode(ctx)
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
func (n *charClassNode) Reverse() node { return n }
func (n *charClassNode) String() string {
	if n.Class != "" {
		return fmt.Sprintf("Class(%s)", n.Class)
	}
	return "Class(<bytes>)"
}
func (n *charClassNode) FingerPrint(_ context.Context) uint64 {
	if n.fp != 0 {
		return n.fp
	}
	if n.Class != "" {
		return mixFingerprint(mixFingerprint(fingerprintSeed, 5), hashString64(n.Class))
	}
	return mixFingerprint(mixFingerprint(fingerprintSeed, 5), hashPredicate64(n.Pred))
}

type complementNode struct {
	Child node
	fp    uint64
}

func newComplementNode(ctx context.Context, child node) node {
	if inner, ok := child.(*complementNode); ok {
		return inner.Child
	}
	return &complementNode{
		Child: child,
		fp:    mixFingerprint(mixFingerprint(fingerprintSeed, 8), child.FingerPrint(ctx)),
	}
}
func (n *complementNode) Nullable(ctx context.Context, mctx matchContext) bool {
	return !n.Child.Nullable(ctx, mctx)
}
func (n *complementNode) Derivative(ctx context.Context, b byte, mctx matchContext) node {
	return newComplementNode(ctx, n.Child.Derivative(ctx, b, mctx))
}
func (n *complementNode) Equals(other node) bool {
	o, ok := other.(*complementNode)
	return ok && n.Child.Equals(o.Child)
}
func (n *complementNode) Reverse() node {
	return newComplementNode(context.Background(), n.Child.Reverse())
}
func (n *complementNode) String() string { return fmt.Sprintf("Comp(%s)", n.Child.String()) }
func (n *complementNode) FingerPrint(ctx context.Context) uint64 {
	return mixFingerprint(mixFingerprint(fingerprintSeed, 8), n.Child.FingerPrint(ctx))
}

type starNode struct {
	Child node
	fp    uint64
}

func newStarNode(_ context.Context, child node) *starNode {
	return &starNode{Child: child}
}

func (n *starNode) Nullable(_ context.Context, _ matchContext) bool { return true }
func (n *starNode) Derivative(ctx context.Context, b byte, mctx matchContext) node {
	return newConcatNode(ctx, n.Child.Derivative(ctx, b, mctx), n)
}
func (n *starNode) Equals(other node) bool {
	o, ok := other.(*starNode)
	return ok && n.Child.Equals(o.Child)
}
func (n *starNode) Reverse() node  { return &starNode{Child: n.Child.Reverse()} }
func (n *starNode) String() string { return fmt.Sprintf("Star(%s)", n.Child.String()) }
func (n *starNode) FingerPrint(ctx context.Context) uint64 {
	if n.fp != 0 {
		return n.fp
	}
	n.fp = mixFingerprint(mixFingerprint(fingerprintSeed, 10), n.Child.FingerPrint(ctx))
	return n.fp
}

type groupNode struct {
	GroupID int
	Child   node
	fp      uint64
}

func newGroupNode(_ context.Context, groupID int, child node) *groupNode {
	return &groupNode{GroupID: groupID, Child: child}
}

func (n *groupNode) Nullable(ctx context.Context, mctx matchContext) bool {
	return n.Child.Nullable(ctx, mctx)
}
func (n *groupNode) Derivative(ctx context.Context, b byte, mctx matchContext) node {
	return n.Child.Derivative(ctx, b, mctx)
}
func (n *groupNode) Equals(other node) bool {
	o, ok := other.(*groupNode)
	return ok && n.GroupID == o.GroupID && n.Child.Equals(o.Child)
}
func (n *groupNode) Reverse() node  { return &groupNode{GroupID: n.GroupID, Child: n.Child.Reverse()} }
func (n *groupNode) String() string { return fmt.Sprintf("Group%d(%s)", n.GroupID, n.Child.String()) }
func (n *groupNode) FingerPrint(ctx context.Context) uint64 {
	if n.fp != 0 {
		return n.fp
	}
	n.fp = mixFingerprint(mixFingerprint(mixFingerprint(fingerprintSeed, 12), uint64(n.GroupID)), n.Child.FingerPrint(ctx))
	return n.fp
}

// lookAheadNode is (?=R). Zero-width; does not consume input. Foundation for TDFA.
type lookAheadNode struct {
	Child node
	fp    uint64
}

func newLookAheadNode(_ context.Context, child node) *lookAheadNode {
	return &lookAheadNode{Child: child}
}

func (n *lookAheadNode) Nullable(ctx context.Context, mctx matchContext) bool {
	return n.Child.Nullable(ctx, mctx)
}
func (n *lookAheadNode) Derivative(ctx context.Context, b byte, mctx matchContext) node {
	return newLookAheadNode(ctx, n.Child.Derivative(ctx, b, mctx))
}
func (n *lookAheadNode) Equals(other node) bool {
	o, ok := other.(*lookAheadNode)
	return ok && n.Child.Equals(o.Child)
}
func (n *lookAheadNode) Reverse() node  { return n }
func (n *lookAheadNode) String() string { return fmt.Sprintf("LookAhead(%s)", n.Child.String()) }
func (n *lookAheadNode) FingerPrint(ctx context.Context) uint64 {
	if n.fp != 0 {
		return n.fp
	}
	n.fp = mixFingerprint(mixFingerprint(fingerprintSeed, 13), n.Child.FingerPrint(ctx))
	return n.fp
}

// lookBehindNode is (?<=R). Zero-width; foundation for TDFA.
type lookBehindNode struct {
	Child node
	fp    uint64
}

func newLookBehindNode(_ context.Context, child node) *lookBehindNode {
	return &lookBehindNode{Child: child}
}

func (n *lookBehindNode) Nullable(ctx context.Context, mctx matchContext) bool {
	return n.Child.Nullable(ctx, mctx)
}
func (n *lookBehindNode) Derivative(ctx context.Context, b byte, mctx matchContext) node {
	return newLookBehindNode(ctx, n.Child.Derivative(ctx, b, mctx))
}
func (n *lookBehindNode) Equals(other node) bool {
	o, ok := other.(*lookBehindNode)
	return ok && n.Child.Equals(o.Child)
}
func (n *lookBehindNode) Reverse() node  { return n }
func (n *lookBehindNode) String() string { return fmt.Sprintf("LookBehind(%s)", n.Child.String()) }
func (n *lookBehindNode) FingerPrint(ctx context.Context) uint64 {
	if n.fp != 0 {
		return n.fp
	}
	n.fp = mixFingerprint(mixFingerprint(fingerprintSeed, 14), n.Child.FingerPrint(ctx))
	return n.fp
}

// --- TDFA: capture boundaries (zero-width) ---

// tagNode marks a capture boundary for the TDFA. Zero-width; does not consume input.
// Id is the capture group number (1-based). IsStart true = open (set start index), false = close (set end index).
type tagNode struct {
	Id      int
	IsStart bool
	fp      uint64
}

func newTagNode(_ context.Context, id int, isStart bool) *tagNode {
	return &tagNode{Id: id, IsStart: isStart}
}

func (n *tagNode) Nullable(_ context.Context, _ matchContext) bool { return true }
func (n *tagNode) Derivative(ctx context.Context, _ byte, _ matchContext) node {
	return newEmptyNode(ctx)
}
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
func (n *tagNode) FingerPrint(_ context.Context) uint64 {
	if n.fp != 0 {
		return n.fp
	}
	if n.IsStart {
		n.fp = mixFingerprint(mixFingerprint(mixFingerprint(fingerprintSeed, 15), uint64(n.Id)), 1)
	} else {
		n.fp = mixFingerprint(mixFingerprint(mixFingerprint(fingerprintSeed, 15), uint64(n.Id)), 0)
	}
	return n.fp
}

type startNode struct {
	fp uint64
}

func newStartNode(_ context.Context) *startNode {
	return &startNode{}
}

func (n *startNode) Nullable(ctx context.Context, mctx matchContext) bool {
	return mctx.AtStart || mctx.PrevIsNewline
}
func (n *startNode) Derivative(ctx context.Context, _ byte, _ matchContext) node {
	return newFalseNode(ctx)
}
func (n *startNode) Equals(other node) bool { _, ok := other.(*startNode); return ok }
func (n *startNode) Reverse() node          { return &endNode{} }
func (n *startNode) String() string         { return "Start" }
func (n *startNode) FingerPrint(_ context.Context) uint64 {
	if n.fp != 0 {
		return n.fp
	}
	n.fp = mixFingerprint(fingerprintSeed, 16)
	return n.fp
}

type endNode struct {
	fp uint64
}

func newEndNode(_ context.Context) *endNode {
	return &endNode{}
}

func (n *endNode) Nullable(ctx context.Context, mctx matchContext) bool {
	return mctx.AtEnd || mctx.NextIsNewline
}
func (n *endNode) Derivative(ctx context.Context, _ byte, _ matchContext) node {
	return newFalseNode(ctx)
}
func (n *endNode) Equals(other node) bool { _, ok := other.(*endNode); return ok }
func (n *endNode) Reverse() node          { return &startNode{} }
func (n *endNode) String() string         { return "End" }
func (n *endNode) FingerPrint(_ context.Context) uint64 {
	if n.fp != 0 {
		return n.fp
	}
	n.fp = mixFingerprint(fingerprintSeed, 17)
	return n.fp
}

type beginTextNode struct {
	fp uint64
}

func newBeginTextNode(_ context.Context) *beginTextNode {
	return &beginTextNode{}
}

func (n *beginTextNode) Nullable(ctx context.Context, mctx matchContext) bool {
	return mctx.AtStart
}
func (n *beginTextNode) Derivative(ctx context.Context, _ byte, _ matchContext) node {
	return newFalseNode(ctx)
}
func (n *beginTextNode) Equals(other node) bool { _, ok := other.(*beginTextNode); return ok }
func (n *beginTextNode) Reverse() node          { return &endTextNode{} }
func (n *beginTextNode) String() string         { return "BeginText" }
func (n *beginTextNode) FingerPrint(_ context.Context) uint64 {
	return mixFingerprint(fingerprintSeed, 18)
}

type endTextNode struct {
	fp uint64
}

func newEndTextNode(_ context.Context) *endTextNode {
	return &endTextNode{}
}

func (n *endTextNode) Nullable(ctx context.Context, mctx matchContext) bool {
	return mctx.AtEnd
}
func (n *endTextNode) Derivative(ctx context.Context, _ byte, _ matchContext) node {
	return newFalseNode(ctx)
}
func (n *endTextNode) Equals(other node) bool { _, ok := other.(*endTextNode); return ok }
func (n *endTextNode) Reverse() node          { return &beginTextNode{} }
func (n *endTextNode) String() string         { return "EndText" }
func (n *endTextNode) FingerPrint(_ context.Context) uint64 {
	return mixFingerprint(fingerprintSeed, 19)
}

type endTextOptionalNewlineNode struct {
	fp uint64
}

func newEndTextOptionalNewlineNode(_ context.Context) *endTextOptionalNewlineNode {
	return &endTextOptionalNewlineNode{}
}

func (n *endTextOptionalNewlineNode) Nullable(ctx context.Context, mctx matchContext) bool {
	return mctx.AtEndAfterOptionalNewline
}
func (n *endTextOptionalNewlineNode) Derivative(ctx context.Context, _ byte, _ matchContext) node {
	return newFalseNode(ctx)
}
func (n *endTextOptionalNewlineNode) Equals(other node) bool {
	_, ok := other.(*endTextOptionalNewlineNode)
	return ok
}
func (n *endTextOptionalNewlineNode) Reverse() node  { return &beginTextNode{} }
func (n *endTextOptionalNewlineNode) String() string { return "EndTextOptionalNewline" }
func (n *endTextOptionalNewlineNode) FingerPrint(_ context.Context) uint64 {
	return mixFingerprint(fingerprintSeed, 20)
}

type wordBoundaryNode struct {
	Unicode bool
	fp      uint64
}

func newWordBoundaryNode(_ context.Context, unicode bool) *wordBoundaryNode {
	return &wordBoundaryNode{Unicode: unicode}
}

func (n *wordBoundaryNode) Nullable(ctx context.Context, mctx matchContext) bool {
	if n.Unicode {
		return mctx.PrevIsWord != mctx.NextIsWord
	}
	return mctx.PrevIsASCIIWord != mctx.NextIsASCIIWord
}
func (n *wordBoundaryNode) Derivative(ctx context.Context, _ byte, _ matchContext) node {
	return newFalseNode(ctx)
}
func (n *wordBoundaryNode) Equals(other node) bool {
	o, ok := other.(*wordBoundaryNode)
	return ok && n.Unicode == o.Unicode
}
func (n *wordBoundaryNode) Reverse() node { return n }
func (n *wordBoundaryNode) String() string {
	if n.Unicode {
		return "WordBoundaryU"
	}
	return "WordBoundary"
}
func (n *wordBoundaryNode) FingerPrint(_ context.Context) uint64 {
	if n.Unicode {
		return mixFingerprint(mixFingerprint(fingerprintSeed, 21), 1)
	}
	return mixFingerprint(mixFingerprint(fingerprintSeed, 21), 0)
}

type notWordBoundaryNode struct {
	Unicode bool
	fp      uint64
}

func newNotWordBoundaryNode(_ context.Context, unicode bool) *notWordBoundaryNode {
	return &notWordBoundaryNode{Unicode: unicode}
}

func (n *notWordBoundaryNode) Nullable(ctx context.Context, mctx matchContext) bool {
	if n.Unicode {
		return mctx.PrevIsWord == mctx.NextIsWord
	}
	return mctx.PrevIsASCIIWord == mctx.NextIsASCIIWord
}
func (n *notWordBoundaryNode) Derivative(ctx context.Context, _ byte, _ matchContext) node {
	return newFalseNode(ctx)
}
func (n *notWordBoundaryNode) Equals(other node) bool {
	o, ok := other.(*notWordBoundaryNode)
	return ok && n.Unicode == o.Unicode
}
func (n *notWordBoundaryNode) Reverse() node { return n }
func (n *notWordBoundaryNode) String() string {
	if n.Unicode {
		return "NotWordBoundaryU"
	}
	return "NotWordBoundary"
}
func (n *notWordBoundaryNode) FingerPrint(_ context.Context) uint64 {
	if n.Unicode {
		return mixFingerprint(mixFingerprint(fingerprintSeed, 22), 1)
	}
	return mixFingerprint(mixFingerprint(fingerprintSeed, 22), 0)
}

// --- UNION ---
type unionNode struct {
	Left, Right node
	fp          uint64
}

// newUnionNodeOrdered creates a union with left before right, without reordering by fingerprint.
// Used by tests when order matters (e.g. POSIX leftmost semantics).
func newUnionNodeOrdered(ctx context.Context, left, right node) node {
	fp := mixFingerprint(mixFingerprint(mixFingerprint(fingerprintSeed, 6), left.FingerPrint(ctx)), right.FingerPrint(ctx))
	return &unionNode{Left: left, Right: right, fp: fp}
}

func newUnionNode(ctx context.Context, left, right node) node {
	if _, isFalse := left.(*falseNode); isFalse {
		return right
	}
	if _, isFalse := right.(*falseNode); isFalse {
		return left
	}
	if left.Equals(right) {
		return left
	}

	// Unpack left-heavy unions to maintain a strict right-heavy sorted list
	if uLeft, ok := left.(*unionNode); ok {
		return newUnionNode(ctx, uLeft.Left, newUnionNode(ctx, uLeft.Right, right))
	}

	lfp := left.FingerPrint(ctx)

	// Insert into the right-heavy list deterministically
	if uRight, isUnion := right.(*unionNode); isUnion {
		rfp := uRight.Left.FingerPrint(ctx)
		if lfp == rfp && left.Equals(uRight.Left) {
			return right
		} // Deduplicate
		if lfp > rfp {
			return newUnionNode(ctx, uRight.Left, newUnionNode(ctx, left, uRight.Right))
		}
		fp := mixFingerprint(mixFingerprint(mixFingerprint(fingerprintSeed, 6), lfp), right.FingerPrint(ctx))
		return &unionNode{Left: left, Right: right, fp: fp}
	}

	rfp := right.FingerPrint(ctx)
	if lfp > rfp {
		left, right = right, left
	}

	fp := mixFingerprint(mixFingerprint(mixFingerprint(fingerprintSeed, 6), left.FingerPrint(ctx)), right.FingerPrint(ctx))
	return &unionNode{Left: left, Right: right, fp: fp}
}

func (n *unionNode) FingerPrint(ctx context.Context) uint64 {
	if n.fp != 0 {
		return n.fp
	}
	n.fp = mixFingerprint(mixFingerprint(mixFingerprint(fingerprintSeed, 6), n.Left.FingerPrint(ctx)), n.Right.FingerPrint(ctx))
	return n.fp
}

func (n *unionNode) Equals(other node) bool {
	o, ok := other.(*unionNode)
	if !ok || n.FingerPrint(context.Background()) != o.FingerPrint(context.Background()) {
		return false
	}
	return n.Left.Equals(o.Left) && n.Right.Equals(o.Right)
}
func (n *unionNode) Nullable(ctx context.Context, mctx matchContext) bool {
	return n.Left.Nullable(ctx, mctx) || n.Right.Nullable(ctx, mctx)
}
func (n *unionNode) Derivative(ctx context.Context, b byte, mctx matchContext) node {
	return newUnionNode(ctx, n.Left.Derivative(ctx, b, mctx), n.Right.Derivative(ctx, b, mctx))
}
func (n *unionNode) Reverse() node {
	return newUnionNode(context.Background(), n.Left.Reverse(), n.Right.Reverse())
}
func (n *unionNode) String() string {
	return fmt.Sprintf("Union(%s,%s)", n.Left.String(), n.Right.String())
}

// --- INTERSECT ---
type intersectNode struct {
	Left, Right node
	fp          uint64
}

func newIntersectNode(ctx context.Context, left, right node) node {
	if _, isFalse := left.(*falseNode); isFalse {
		return newFalseNode(ctx)
	}
	if _, isFalse := right.(*falseNode); isFalse {
		return newFalseNode(ctx)
	}
	if left.Equals(right) {
		return left
	}

	// Unpack left-heavy intersections to maintain a strict right-heavy sorted list
	if iLeft, ok := left.(*intersectNode); ok {
		return newIntersectNode(ctx, iLeft.Left, newIntersectNode(ctx, iLeft.Right, right))
	}

	lfp := left.FingerPrint(ctx)

	// Insert into the right-heavy list deterministically
	if iRight, isIntersect := right.(*intersectNode); isIntersect {
		rfp := iRight.Left.FingerPrint(ctx)
		if lfp == rfp && left.Equals(iRight.Left) {
			return right
		} // Deduplicate
		if lfp > rfp {
			return newIntersectNode(ctx, iRight.Left, newIntersectNode(ctx, left, iRight.Right))
		}
		fp := mixFingerprint(mixFingerprint(mixFingerprint(fingerprintSeed, 7), lfp), right.FingerPrint(ctx))
		return &intersectNode{Left: left, Right: right, fp: fp}
	}

	rfp := right.FingerPrint(ctx)
	if lfp > rfp {
		left, right = right, left
	}

	fp := mixFingerprint(mixFingerprint(mixFingerprint(fingerprintSeed, 7), left.FingerPrint(ctx)), right.FingerPrint(ctx))
	return &intersectNode{Left: left, Right: right, fp: fp}
}

func (n *intersectNode) FingerPrint(ctx context.Context) uint64 {
	if n.fp != 0 {
		return n.fp
	}
	n.fp = mixFingerprint(mixFingerprint(mixFingerprint(fingerprintSeed, 7), n.Left.FingerPrint(ctx)), n.Right.FingerPrint(ctx))
	return n.fp
}

func (n *intersectNode) Equals(other node) bool {
	o, ok := other.(*intersectNode)
	if !ok || n.FingerPrint(context.Background()) != o.FingerPrint(context.Background()) {
		return false
	}
	return n.Left.Equals(o.Left) && n.Right.Equals(o.Right)
}
func (n *intersectNode) Nullable(ctx context.Context, mctx matchContext) bool {
	return n.Left.Nullable(ctx, mctx) && n.Right.Nullable(ctx, mctx)
}
func (n *intersectNode) Derivative(ctx context.Context, b byte, mctx matchContext) node {
	return newIntersectNode(ctx, n.Left.Derivative(ctx, b, mctx), n.Right.Derivative(ctx, b, mctx))
}
func (n *intersectNode) Reverse() node {
	return newIntersectNode(context.Background(), n.Left.Reverse(), n.Right.Reverse())
}
func (n *intersectNode) String() string {
	return fmt.Sprintf("Int(%s,%s)", n.Left.String(), n.Right.String())
}

// --- CONCAT ---
type concatNode struct {
	Left, Right node
	fp          uint64
}

func newConcatNode(ctx context.Context, left, right node) node {
	if _, ok := right.(*falseNode); ok {
		return newFalseNode(ctx)
	}
	if _, ok := left.(*falseNode); ok {
		return newFalseNode(ctx)
	}
	if _, ok := left.(*emptyNode); ok {
		return right
	}
	if _, ok := right.(*emptyNode); ok {
		return left
	}

	// Unpack left-heavy concats to guarantee a canonical right-heavy structure
	if cLeft, ok := left.(*concatNode); ok {
		return newConcatNode(ctx, cLeft.Left, newConcatNode(ctx, cLeft.Right, right))
	}

	fp := mixFingerprint(mixFingerprint(mixFingerprint(fingerprintSeed, 9), left.FingerPrint(ctx)), right.FingerPrint(ctx))
	return &concatNode{Left: left, Right: right, fp: fp}
}

func (n *concatNode) FingerPrint(ctx context.Context) uint64 {
	if n.fp != 0 {
		return n.fp
	}
	n.fp = mixFingerprint(mixFingerprint(mixFingerprint(fingerprintSeed, 9), n.Left.FingerPrint(ctx)), n.Right.FingerPrint(ctx))
	return n.fp
}

func (n *concatNode) Equals(other node) bool {
	o, ok := other.(*concatNode)
	if !ok || n.FingerPrint(context.Background()) != o.FingerPrint(context.Background()) {
		return false
	}
	return n.Left.Equals(o.Left) && n.Right.Equals(o.Right)
}
func (n *concatNode) Nullable(ctx context.Context, mctx matchContext) bool {
	return n.Left.Nullable(ctx, mctx) && n.Right.Nullable(ctx, mctx)
}
func (n *concatNode) Derivative(ctx context.Context, b byte, mctx matchContext) node {
	leftDerivConcat := newConcatNode(ctx, n.Left.Derivative(ctx, b, mctx), n.Right)
	if n.Left.Nullable(ctx, mctx) {
		return newUnionNode(ctx, leftDerivConcat, n.Right.Derivative(ctx, b, mctx))
	}
	return leftDerivConcat
}
func (n *concatNode) Reverse() node {
	return newConcatNode(context.Background(), n.Right.Reverse(), n.Left.Reverse())
}
func (n *concatNode) String() string {
	return fmt.Sprintf("Cat(%s,%s)", n.Left.String(), n.Right.String())
}

// --- REPEAT ---
type repeatNode struct {
	Child node
	Min   int
	Max   int
	fp    uint64
}

func newRepeatNode(ctx context.Context, child node, min, max int) node {
	if max == 0 {
		return newEmptyNode(ctx)
	}
	if min == 1 && max == 1 {
		return child
	}
	if _, isFalse := child.(*falseNode); isFalse {
		if min == 0 {
			return newEmptyNode(ctx)
		}
		return newFalseNode(ctx)
	}
	if _, isEmpty := child.(*emptyNode); isEmpty {
		return newEmptyNode(ctx)
	}

	h := mixFingerprint(fingerprintSeed, 11)
	h = mixFingerprint(h, child.FingerPrint(ctx))
	h = mixFingerprint(h, uint64(min+1))
	fp := mixFingerprint(h, uint64(max+1))

	return &repeatNode{Child: child, Min: min, Max: max, fp: fp}
}

func (n *repeatNode) FingerPrint(ctx context.Context) uint64 {
	if n.fp != 0 {
		return n.fp
	}
	h := mixFingerprint(fingerprintSeed, 11)
	h = mixFingerprint(h, n.Child.FingerPrint(ctx))
	h = mixFingerprint(h, uint64(n.Min+1))
	n.fp = mixFingerprint(h, uint64(n.Max+1))
	return n.fp
}

func (n *repeatNode) Equals(other node) bool {
	o, ok := other.(*repeatNode)
	if !ok || n.FingerPrint(context.Background()) != o.FingerPrint(context.Background()) {
		return false
	}
	return n.Min == o.Min && n.Max == o.Max && n.Child.Equals(o.Child)
}
func (n *repeatNode) Nullable(ctx context.Context, mctx matchContext) bool {
	return n.Min == 0 || n.Child.Nullable(ctx, mctx)
}
func (n *repeatNode) Derivative(ctx context.Context, b byte, mctx matchContext) node {
	if n.Max == 0 {
		return newFalseNode(ctx)
	}

	nextMin := n.Min - 1
	if nextMin < 0 {
		nextMin = 0
	}
	nextMax := n.Max - 1

	nextRepeat := newRepeatNode(ctx, n.Child, nextMin, nextMax)
	derivChild := n.Child.Derivative(ctx, b, mctx)

	if !n.Child.Nullable(ctx, mctx) {
		return newConcatNode(ctx, derivChild, nextRepeat)
	}

	unionTree := make([]node, 0, nextMax+1)
	unionTree = append(unionTree, newConcatNode(ctx, derivChild, nextRepeat))

	currentMin := nextMin
	currentMax := nextMax
	for currentMax > 0 {
		currentMin--
		if currentMin < 0 {
			currentMin = 0
		}
		currentMax--
		unionTree = append(unionTree, newConcatNode(ctx, derivChild, newRepeatNode(ctx, n.Child, currentMin, currentMax)))
	}

	if len(unionTree) == 1 {
		return unionTree[0]
	}
	res := unionTree[len(unionTree)-1]
	for i := len(unionTree) - 2; i >= 0; i-- {
		res = newUnionNode(ctx, unionTree[i], res)
	}
	return res
}
func (n *repeatNode) Reverse() node {
	return newRepeatNode(context.Background(), n.Child.Reverse(), n.Min, n.Max)
}
func (n *repeatNode) String() string { return fmt.Sprintf("Repeat(%s, %d, %d)", n.Child, n.Min, n.Max) }

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

const fingerprintSeed uint64 = 1469598103934665603

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
