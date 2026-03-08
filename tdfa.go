package re3

import "unicode/utf8"

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
func stepTDFA(n node, c rune) []tdfaConfig {
	switch nd := n.(type) {
	case *literalNode:
		if nd.Value == c {
			return []tdfaConfig{{NextNode: &emptyNode{}, Tags: nil}}
		}
		return []tdfaConfig{{NextNode: &falseNode{}, Tags: nil}}
	case *charClassNode:
		p := parseCharClass(nd.Class)
		if c < 256 && p[c] {
			return []tdfaConfig{{NextNode: &emptyNode{}, Tags: nil}}
		}
		return []tdfaConfig{{NextNode: &falseNode{}, Tags: nil}}
	case *anyNode:
		if c == '\n' {
			return []tdfaConfig{{NextNode: &falseNode{}, Tags: nil}}
		}
		return []tdfaConfig{{NextNode: &emptyNode{}, Tags: nil}}
	case *falseNode:
		return []tdfaConfig{{NextNode: &falseNode{}, Tags: nil}}
	case *emptyNode:
		return []tdfaConfig{{NextNode: &falseNode{}, Tags: nil}}
	case *tagNode:
		return []tdfaConfig{{NextNode: &emptyNode{}, Tags: nil}}
	case *unionNode:
		left := stepTDFA(nd.Left, c)
		right := stepTDFA(nd.Right, c)
		return append(left, right...)
	case *concatNode:
		leftConfigs := stepTDFA(nd.Left, c)
		var result []tdfaConfig
		if nd.Left.Nullable() {
			rightConfigs := stepTDFA(nd.Right, c)
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
				next = &concatNode{Left: lc.NextNode, Right: nd.Right}
			}
			result = append(result, tdfaConfig{NextNode: next, Tags: tags})
		}
		return result
	case *starNode:
		childConfigs := stepTDFA(nd.Child, c)
		var result []tdfaConfig
		for _, cc := range childConfigs {
			if _, isEmpty := cc.NextNode.(*emptyNode); isEmpty {
				result = append(result, tdfaConfig{NextNode: nd, Tags: cc.Tags})
			} else {
				result = append(result, tdfaConfig{
					NextNode: &concatNode{Left: cc.NextNode, Right: nd},
					Tags:     cc.Tags,
				})
			}
		}
		return result
	case *groupNode:
		return stepTDFA(nd.Child, c)
	case *lookAheadNode:
		childConfigs := stepTDFA(nd.Child, c)
		var result []tdfaConfig
		for _, cc := range childConfigs {
			result = append(result, tdfaConfig{
				NextNode: &lookAheadNode{Child: cc.NextNode},
				Tags:     cc.Tags,
			})
		}
		return result
	case *lookBehindNode:
		childConfigs := stepTDFA(nd.Child, c)
		var result []tdfaConfig
		for _, cc := range childConfigs {
			result = append(result, tdfaConfig{
				NextNode: &lookBehindNode{Child: cc.NextNode},
				Tags:     cc.Tags,
			})
		}
		return result
	case *intersectNode:
		leftConfigs := stepTDFA(nd.Left, c)
		rightConfigs := stepTDFA(nd.Right, c)
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
		childConfigs := stepTDFA(nd.Child, c)
		var result []tdfaConfig
		for _, cc := range childConfigs {
			result = append(result, tdfaConfig{
				NextNode: newComplementNode(cc.NextNode),
				Tags:     cc.Tags,
			})
		}
		return result
	default:
		return nil
	}
}

// injectCaptureTags replaces each groupNode(id, child) with Concat(Tag(id,start), Concat(child, Tag(id,end))).
func injectCaptureTags(ast node) node {
	switch n := ast.(type) {
	case *groupNode:
		inner := injectCaptureTags(n.Child)
		return &concatNode{
			Left: &tagNode{Id: n.GroupID, IsStart: true},
			Right: &concatNode{
				Left:  inner,
				Right: &tagNode{Id: n.GroupID, IsStart: false},
			},
		}
	case *concatNode:
		return &concatNode{
			Left:  injectCaptureTags(n.Left),
			Right: injectCaptureTags(n.Right),
		}
	case *unionNode:
		return &unionNode{
			Left:  injectCaptureTags(n.Left),
			Right: injectCaptureTags(n.Right),
		}
	case *starNode:
		return &starNode{Child: injectCaptureTags(n.Child)}
	case *intersectNode:
		return &intersectNode{
			Left:  injectCaptureTags(n.Left),
			Right: injectCaptureTags(n.Right),
		}
	case *complementNode:
		return &complementNode{Child: injectCaptureTags(n.Child)}
	case *lookAheadNode:
		return &lookAheadNode{Child: injectCaptureTags(n.Child)}
	case *lookBehindNode:
		return &lookBehindNode{Child: injectCaptureTags(n.Child)}
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
	transitions [][]tdfaTransition
	isMatch     []bool
	deadStateID int
	numCaptures int
}

func newLazyTDFA(taggedRoot node, minterms *mintermTable, numCaptures int) *lazyTDFA {
	dead := &falseNode{}
	dfa := &lazyTDFA{
		root:        taggedRoot,
		minterms:    minterms,
		stateASTs:   []node{taggedRoot, dead},
		transitions: make([][]tdfaTransition, 2),
		isMatch:     []bool{taggedRoot.Nullable(), false},
		deadStateID: 1,
		numCaptures: numCaptures,
	}
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
func (dfa *lazyTDFA) getNextState(stateID, mintermID int) (nextStateID int, tagOps []tagOp) {
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
	r := rune(0)
	if mintermID < len(dfa.minterms.ClassToRune) {
		r = dfa.minterms.ClassToRune[mintermID]
	}
	configs := stepTDFA(dfa.stateASTs[stateID], r)
	if len(configs) == 0 {
		row[mintermID] = tdfaTransition{Next: dfa.deadStateID, Tags: nil}
		return dfa.deadStateID, nil
	}
	chosen := configs[0]
	nextStateID = -1
	for id, seen := range dfa.stateASTs {
		if seen.Equals(chosen.NextNode) {
			nextStateID = id
			break
		}
	}
	if nextStateID < 0 {
		if len(dfa.stateASTs) >= maxLazyTDFAStates {
			row[mintermID] = tdfaTransition{Next: dfa.deadStateID, Tags: nil}
			return dfa.deadStateID, nil
		}
		nextStateID = len(dfa.stateASTs)
		dfa.stateASTs = append(dfa.stateASTs, chosen.NextNode)
		dfa.isMatch = append(dfa.isMatch, chosen.NextNode.Nullable())
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

func (dfa *lazyTDFA) isAccepting(stateID int) bool {
	if stateID < 0 || stateID >= len(dfa.isMatch) {
		return false
	}
	return dfa.isMatch[stateID]
}

// runTDFA runs the TDFA on s (typically the match span from two-pass) and fills
// a flat []int of length (numCaptures+1)*2: for slot i, start=capture[2*i], end=capture[2*i+1].
// Slot 0 is the full match; slots 1..numCaptures are capture groups. -1 means unmatched.
func (dfa *lazyTDFA) runTDFA(s string) []int {
	ncap := dfa.numCaptures + 1
	capture := make([]int, ncap*2)
	for i := 0; i < ncap*2; i++ {
		capture[i] = -1
	}
	state := 0
	pos := 0
	for pos <= len(s) {
		if dfa.isAccepting(state) {
			capture[0] = 0
			capture[1] = pos
		}
		if pos >= len(s) {
			break
		}
		r, size := utf8.DecodeRuneInString(s[pos:])
		mintermID := dfa.minterms.runeToClass(r)
		nextState, tags := dfa.getNextState(state, mintermID)
		for _, t := range tags {
			if t.Id >= ncap {
				continue
			}
			if t.IsStart {
				capture[t.Id*2] = pos
			}
		}
		state = nextState
		pos += size
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
	if !dfa.isAccepting(state) {
		return nil
	}
	if capture[0] < 0 {
		capture[0] = 0
		capture[1] = pos
	}
	return capture
}
