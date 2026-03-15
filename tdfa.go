package re3

import "context"

// tagOp represents a single tag operation: set capture group Id's start or end index.
type tagOp struct {
	Id      int
	IsStart bool
}

// tdfaConfig is one possible (next state, tag ops) after reading a character.
type tdfaConfig struct {
	NextNode node
	Tags     []tagOp
}

// stepTDFA computes the derivative and collects tags simultaneously.
// It does not call Node.Derivative() so smart constructors never collapse Union.
// Returns one config per surviving path (e.g. Union returns multiple configs).
func stepTDFA(ctx context.Context, n node, c byte, mctx matchContext) []tdfaConfig {
	switch nd := n.(type) {
	case *literalNode:
		if nd.Value == c {
			return []tdfaConfig{{NextNode: newEmptyNode(ctx), Tags: nil}}
		}
		return []tdfaConfig{{NextNode: newFalseNode(ctx), Tags: nil}}
	case *charClassNode:
		p := parseCharClass(ctx, nd.Class)
		if p[c] {
			return []tdfaConfig{{NextNode: newEmptyNode(ctx), Tags: nil}}
		}
		return []tdfaConfig{{NextNode: newFalseNode(ctx), Tags: nil}}
	case *anyNode:
		if c == '\n' {
			return []tdfaConfig{{NextNode: newFalseNode(ctx), Tags: nil}}
		}
		return []tdfaConfig{{NextNode: newEmptyNode(ctx), Tags: nil}}
	case *falseNode:
		return []tdfaConfig{{NextNode: newFalseNode(ctx), Tags: nil}}
	case *emptyNode:
		return []tdfaConfig{{NextNode: newFalseNode(ctx), Tags: nil}}
	case *tagNode:
		return []tdfaConfig{{NextNode: newEmptyNode(ctx), Tags: nil}}
	case *unionNode:
		left := stepTDFA(ctx, nd.Left, c, mctx)
		right := stepTDFA(ctx, nd.Right, c, mctx)
		return append(left, right...)
	case *concatNode:
		leftConfigs := stepTDFA(ctx, nd.Left, c, mctx)
		var result []tdfaConfig
		if nd.Left.Nullable(ctx, mctx) {
			rightConfigs := stepTDFA(ctx, nd.Right, c, mctx)
			for _, rc := range rightConfigs {
				tags := rc.Tags
				if t, ok := nd.Left.(*tagNode); ok {
					tags = make([]tagOp, 0, len(rc.Tags)+1)
					tags = append(tags, tagOp{Id: t.Id, IsStart: t.IsStart})
					tags = append(tags, rc.Tags...)
				}
				result = append(result, tdfaConfig{NextNode: rc.NextNode, Tags: tags})
			}
		}
		for _, lc := range leftConfigs {
			var next node
			tags := lc.Tags
			if _, isEmpty := lc.NextNode.(*emptyNode); isEmpty {
				next = nd.Right
				if t, ok := nd.Right.(*tagNode); ok {
					tags = make([]tagOp, len(lc.Tags)+1)
					copy(tags, lc.Tags)
					tags[len(lc.Tags)] = tagOp{Id: t.Id, IsStart: t.IsStart}
				}
			} else {
				next = newConcatNode(ctx, lc.NextNode, nd.Right)
			}
			result = append(result, tdfaConfig{NextNode: next, Tags: tags})
		}
		return result
	case *starNode:
		childConfigs := stepTDFA(ctx, nd.Child, c, mctx)
		var result []tdfaConfig
		for _, cc := range childConfigs {
			if _, isEmpty := cc.NextNode.(*emptyNode); isEmpty {
				result = append(result, tdfaConfig{NextNode: nd, Tags: cc.Tags})
			} else {
				result = append(result, tdfaConfig{
					NextNode: newConcatNode(ctx, cc.NextNode, nd),
					Tags:     cc.Tags,
				})
			}
		}
		return result
	case *groupNode:
		return stepTDFA(ctx, nd.Child, c, mctx)
	case *repeatNode:
		if nd.Max == 0 {
			return []tdfaConfig{{NextNode: newFalseNode(ctx), Tags: nil}}
		}
		nextMin := nd.Min - 1
		if nextMin < 0 {
			nextMin = 0
		}
		nextMax := nd.Max - 1
		nextRepeat := newRepeatNode(ctx, nd.Child, nextMin, nextMax)
		childConfigs := stepTDFA(ctx, nd.Child, c, mctx)
		var result []tdfaConfig
		for _, cc := range childConfigs {
			var next node
			if _, isEmpty := cc.NextNode.(*emptyNode); isEmpty {
				next = nextRepeat
			} else {
				next = newConcatNode(ctx, cc.NextNode, nextRepeat)
			}
			result = append(result, tdfaConfig{NextNode: next, Tags: cc.Tags})
		}
		if nd.Child.Nullable(ctx, mctx) && nd.Max > 0 {
			currentMin := nextMin
			currentMax := nextMax
			for currentMax > 0 {
				currentMin--
				if currentMin < 0 {
					currentMin = 0
				}
				currentMax--
				peeled := newRepeatNode(ctx, nd.Child, currentMin, currentMax)
				for _, cc := range childConfigs {
					var next node
					if _, isEmpty := cc.NextNode.(*emptyNode); isEmpty {
						next = peeled
					} else {
						next = newConcatNode(ctx, cc.NextNode, peeled)
					}
					result = append(result, tdfaConfig{NextNode: next, Tags: cc.Tags})
				}
			}
		}
		return result
	case *lookAheadNode:
		childConfigs := stepTDFA(ctx, nd.Child, c, mctx)
		var result []tdfaConfig
		for _, cc := range childConfigs {
			result = append(result, tdfaConfig{
				NextNode: newLookAheadNode(ctx, cc.NextNode),
				Tags:     cc.Tags,
			})
		}
		return result
	case *lookBehindNode:
		childConfigs := stepTDFA(ctx, nd.Child, c, mctx)
		var result []tdfaConfig
		for _, cc := range childConfigs {
			result = append(result, tdfaConfig{
				NextNode: newLookBehindNode(ctx, cc.NextNode),
				Tags:     cc.Tags,
			})
		}
		return result
	case *intersectNode:
		leftConfigs := stepTDFA(ctx, nd.Left, c, mctx)
		rightConfigs := stepTDFA(ctx, nd.Right, c, mctx)
		var result []tdfaConfig
		for _, lc := range leftConfigs {
			for _, rc := range rightConfigs {
				if lc.NextNode.Equals(rc.NextNode) {
					tags := append([]tagOp{}, lc.Tags...)
					tags = append(tags, rc.Tags...)
					result = append(result, tdfaConfig{NextNode: lc.NextNode, Tags: tags})
					break
				}
			}
		}
		return result
	case *complementNode:
		childConfigs := stepTDFA(ctx, nd.Child, c, mctx)
		var result []tdfaConfig
		for _, cc := range childConfigs {
			result = append(result, tdfaConfig{
				NextNode: newComplementNode(ctx, cc.NextNode),
				Tags:     cc.Tags,
			})
		}
		return result
	case *startNode, *endNode, *beginTextNode, *endTextNode, *endTextOptionalNewlineNode, *wordBoundaryNode, *notWordBoundaryNode:
		if nd.Nullable(ctx, mctx) {
			return []tdfaConfig{{NextNode: newEmptyNode(ctx), Tags: nil}}
		}
		return []tdfaConfig{{NextNode: newFalseNode(ctx), Tags: nil}}
	default:
		return nil
	}
}

// injectCaptureTags replaces each groupNode(id, child) with Concat(Tag(id,start), Concat(child, Tag(id,end))).
func injectCaptureTags(ctx context.Context, ast node) node {
	switch n := ast.(type) {
	case *groupNode:
		inner := injectCaptureTags(ctx, n.Child)
		return newConcatNode(ctx,
			newTagNode(ctx, n.GroupID, true),
			newConcatNode(ctx, inner, newTagNode(ctx, n.GroupID, false)),
		)
	case *concatNode:
		return newConcatNode(ctx, injectCaptureTags(ctx, n.Left), injectCaptureTags(ctx, n.Right))
	case *unionNode:
		return newUnionNode(ctx, injectCaptureTags(ctx, n.Left), injectCaptureTags(ctx, n.Right))
	case *starNode:
		return newStarNode(ctx, injectCaptureTags(ctx, n.Child))
	case *repeatNode:
		return newRepeatNode(ctx, injectCaptureTags(ctx, n.Child), n.Min, n.Max)
	case *intersectNode:
		return newIntersectNode(ctx, injectCaptureTags(ctx, n.Left), injectCaptureTags(ctx, n.Right))
	case *complementNode:
		return newComplementNode(ctx, injectCaptureTags(ctx, n.Child))
	case *lookAheadNode:
		return newLookAheadNode(ctx, injectCaptureTags(ctx, n.Child))
	case *lookBehindNode:
		return newLookBehindNode(ctx, injectCaptureTags(ctx, n.Child))
	default:
		return ast
	}
}

func countCaptureGroups(ast node) int {
	var count int
	var walk func(node)
	walk = func(n node) {
		if g, ok := n.(*groupNode); ok {
			count++
			walk(g.Child)
			return
		}
		switch n := n.(type) {
		case *concatNode:
			walk(n.Left)
			walk(n.Right)
		case *unionNode:
			walk(n.Left)
			walk(n.Right)
		case *starNode:
			walk(n.Child)
		case *repeatNode:
			walk(n.Child)
		case *intersectNode:
			walk(n.Left)
			walk(n.Right)
		case *complementNode:
			walk(n.Child)
		case *lookAheadNode:
			walk(n.Child)
		case *lookBehindNode:
			walk(n.Child)
		}
	}
	walk(ast)
	return count
}

const maxLazyTDFAStates = 100_000

// tdfaTransition holds the next state and tag ops for one (state, minterm) transition.
// Next == -1 means not yet computed.
type tdfaTransition struct {
	Next int
	Tags []tagOp
}

// lazyTDFA is like lazyDFA but transitions carry tag ops. Used only for submatch extraction.
type lazyTDFA struct {
	root        node
	minterms    *mintermTable
	stateASTs   []node
	stateIndex  map[uint64][]int
	transitions [][]tdfaTransition
	isMatch     []bool
	deadStateID int
	numCaptures int
}

func newLazyTDFA(ctx context.Context, taggedRoot node, minterms *mintermTable, numCaptures int) *lazyTDFA {
	dead := newFalseNode(ctx)
	dfa := &lazyTDFA{
		root:        taggedRoot,
		minterms:    minterms,
		stateASTs:   []node{taggedRoot, dead},
		stateIndex:  make(map[uint64][]int, 2),
		transitions: make([][]tdfaTransition, 2),
		isMatch:     []bool{taggedRoot.Nullable(ctx, matchContext{}), false},
		deadStateID: 1,
		numCaptures: numCaptures,
	}
	dfa.indexState(ctx, 0, taggedRoot)
	dfa.indexState(ctx, 1, dead)
	dfa.transitions[0] = make([]tdfaTransition, minterms.NumClasses)
	dfa.transitions[1] = make([]tdfaTransition, minterms.NumClasses)
	for i := 0; i < minterms.NumClasses; i++ {
		dfa.transitions[0][i].Next = -1
		dfa.transitions[1][i].Next = -1
	}
	for i := 0; i < minterms.NumClasses; i++ {
		dfa.transitions[1][i] = tdfaTransition{Next: 1, Tags: nil}
	}
	return dfa
}

// getNextState returns (nextStateID, tagOps). tagOps are applied when taking the transition.
func (dfa *lazyTDFA) getNextState(ctx context.Context, stateID, mintermID int, mctx matchContext) (nextStateID int, tagOps []tagOp) {
	if stateID == dfa.deadStateID {
		return dfa.deadStateID, nil
	}
	if stateID >= len(dfa.transitions) {
		return dfa.deadStateID, nil
	}
	row := dfa.transitions[stateID]
	if row == nil {
		row = make([]tdfaTransition, dfa.minterms.NumClasses)
		for i := range row {
			row[i].Next = -1
		}
		dfa.transitions[stateID] = row
	}
	t := &row[mintermID]
	if t.Next >= 0 {
		return t.Next, t.Tags
	}
	b := dfa.minterms.ClassToByte[mintermID]
	configs := stepTDFA(ctx, dfa.stateASTs[stateID], b, mctx)
	if len(configs) == 0 {
		row[mintermID] = tdfaTransition{Next: dfa.deadStateID, Tags: nil}
		return dfa.deadStateID, nil
	}
	chosen := configs[0]
	nextStateID = dfa.lookupState(ctx, chosen.NextNode)
	if nextStateID < 0 {
		if len(dfa.stateASTs) >= maxLazyTDFAStates {
			row[mintermID] = tdfaTransition{Next: dfa.deadStateID, Tags: nil}
			return dfa.deadStateID, nil
		}
		nextStateID = len(dfa.stateASTs)
		dfa.stateASTs = append(dfa.stateASTs, chosen.NextNode)
		dfa.indexState(ctx, nextStateID, chosen.NextNode)
		dfa.isMatch = append(dfa.isMatch, chosen.NextNode.Nullable(ctx, mctx))
		newRow := make([]tdfaTransition, dfa.minterms.NumClasses)
		for i := range newRow {
			newRow[i].Next = -1
		}
		dfa.transitions = append(dfa.transitions, newRow)
	}
	var tags []tagOp
	if len(chosen.Tags) > 0 {
		tags = make([]tagOp, len(chosen.Tags))
		copy(tags, chosen.Tags)
	}
	row[mintermID] = tdfaTransition{Next: nextStateID, Tags: tags}
	return nextStateID, tags
}

func (dfa *lazyTDFA) lookupState(ctx context.Context, candidate node) int {
	fp := candidate.FingerPrint(ctx)
	bucket := dfa.stateIndex[fp]
	for _, stateID := range bucket {
		if dfa.stateASTs[stateID].Equals(candidate) {
			return stateID
		}
	}
	return -1
}

func (dfa *lazyTDFA) indexState(ctx context.Context, stateID int, ast node) {
	fp := ast.FingerPrint(ctx)
	dfa.stateIndex[fp] = append(dfa.stateIndex[fp], stateID)
}

func (dfa *lazyTDFA) isAccepting(stateID int) bool {
	if stateID < 0 || stateID >= len(dfa.isMatch) {
		return false
	}
	return dfa.isMatch[stateID]
}

func (dfa *lazyTDFA) isAcceptingWithContext(ctx context.Context, stateID int, mctx matchContext) bool {
	if stateID < 0 || stateID >= len(dfa.stateASTs) {
		return false
	}
	return dfa.stateASTs[stateID].Nullable(ctx, mctx)
}

// runTDFA runs the TDFA on s (typically the match span from two-pass) and fills
// a flat []int of length (numCaptures+1)*2: for slot i, start=capture[2*i], end=capture[2*i+1].
// Slot 0 is the full match; slots 1..numCaptures are capture groups. -1 means unmatched.
func (dfa *lazyTDFA) runTDFA(ctx context.Context, s string) []int {
	ncap := dfa.numCaptures + 1
	capture := make([]int, ncap*2)
	for i := 0; i < ncap*2; i++ {
		capture[i] = -1
	}
	state := 0
	pos := 0
	for pos <= len(s) {
		ctxAtPos := makeMatchContextString(s, pos)
		if dfa.isAcceptingWithContext(ctx, state, ctxAtPos) {
			capture[0] = 0
			capture[1] = pos
		}
		if pos >= len(s) {
			break
		}
		mintermID := dfa.minterms.ByteToClass[s[pos]]
		nextState, tags := dfa.getNextState(ctx, state, mintermID, ctxAtPos)
		for _, t := range tags {
			if t.Id >= ncap {
				continue
			}
			if t.IsStart {
				capture[t.Id*2] = pos
			}
		}
		state = nextState
		pos++
		for _, t := range tags {
			if t.Id >= ncap {
				continue
			}
			if !t.IsStart {
				capture[t.Id*2+1] = pos
			}
		}
	}
	if state == dfa.deadStateID {
		return nil
	}
	if !dfa.isAcceptingWithContext(ctx, state, makeMatchContextString(s, pos)) {
		return nil
	}
	if capture[0] < 0 {
		capture[0] = 0
		capture[1] = pos
	}
	return capture
}
