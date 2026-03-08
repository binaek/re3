package re3

import "unicode/utf8"

// tagOp represents a single tag operation: set capture group Id's start or end index.
type tagOp struct {
	Id      int
	IsStart bool
}

// tdfaConfig is one possible (next state, tag ops) after reading a character.
type tdfaConfig struct {
	NextNode Node
	Tags     []tagOp
}

// stepTDFA computes the derivative and collects tags simultaneously.
// It does not call Node.Derivative() so smart constructors never collapse Union.
// Returns one config per surviving path (e.g. Union returns multiple configs).
func stepTDFA(n Node, c rune) []tdfaConfig {
	switch node := n.(type) {
	case *LiteralNode:
		if node.Value == c {
			return []tdfaConfig{{NextNode: &EmptyNode{}, Tags: nil}}
		}
		return []tdfaConfig{{NextNode: &FalseNode{}, Tags: nil}}
	case *CharClassNode:
		p := parseCharClass(node.Class)
		if c < 256 && p[c] {
			return []tdfaConfig{{NextNode: &EmptyNode{}, Tags: nil}}
		}
		return []tdfaConfig{{NextNode: &FalseNode{}, Tags: nil}}
	case *AnyNode:
		if c == '\n' {
			return []tdfaConfig{{NextNode: &FalseNode{}, Tags: nil}}
		}
		return []tdfaConfig{{NextNode: &EmptyNode{}, Tags: nil}}
	case *FalseNode:
		return []tdfaConfig{{NextNode: &FalseNode{}, Tags: nil}}
	case *EmptyNode:
		return []tdfaConfig{{NextNode: &FalseNode{}, Tags: nil}}
	case *TagNode:
		// Zero-width: step to Empty. Do not emit here; tag is emitted when we enter (Concat with next=TagNode after Empty).
		return []tdfaConfig{{NextNode: &EmptyNode{}, Tags: nil}}
	case *UnionNode:
		left := stepTDFA(node.Left, c)
		right := stepTDFA(node.Right, c)
		return append(left, right...)
	case *ConcatNode:
		leftConfigs := stepTDFA(node.Left, c)
		var result []tdfaConfig
		if node.Left.Nullable() {
			rightConfigs := stepTDFA(node.Right, c)
			for _, rc := range rightConfigs {
				tags := rc.Tags
				if t, ok := node.Left.(*TagNode); ok {
					tags = make([]tagOp, 0, len(rc.Tags)+1)
					tags = append(tags, tagOp{Id: t.Id, IsStart: t.IsStart})
					tags = append(tags, rc.Tags...)
				}
				result = append(result, tdfaConfig{NextNode: rc.NextNode, Tags: tags})
			}
		}
		for _, lc := range leftConfigs {
			var next Node
			tags := lc.Tags
			if _, isEmpty := lc.NextNode.(*EmptyNode); isEmpty {
				next = node.Right
				// Emit tag only when entering Tag after consuming content (not when skipping False/etc).
				if t, ok := node.Right.(*TagNode); ok {
					tags = make([]tagOp, len(lc.Tags)+1)
					copy(tags, lc.Tags)
					tags[len(lc.Tags)] = tagOp{Id: t.Id, IsStart: t.IsStart}
				}
			} else {
				next = &ConcatNode{Left: lc.NextNode, Right: node.Right}
			}
			result = append(result, tdfaConfig{NextNode: next, Tags: tags})
		}
		return result
	case *StarNode:
		childConfigs := stepTDFA(node.Child, c)
		var result []tdfaConfig
		for _, cc := range childConfigs {
			if _, isEmpty := cc.NextNode.(*EmptyNode); isEmpty {
				result = append(result, tdfaConfig{NextNode: node, Tags: cc.Tags})
			} else {
				result = append(result, tdfaConfig{
					NextNode: &ConcatNode{Left: cc.NextNode, Right: node},
					Tags:     cc.Tags,
				})
			}
		}
		return result
	case *GroupNode:
		return stepTDFA(node.Child, c)
	case *LookAheadNode:
		childConfigs := stepTDFA(node.Child, c)
		var result []tdfaConfig
		for _, cc := range childConfigs {
			result = append(result, tdfaConfig{
				NextNode: &LookAheadNode{Child: cc.NextNode},
				Tags:     cc.Tags,
			})
		}
		return result
	case *LookBehindNode:
		childConfigs := stepTDFA(node.Child, c)
		var result []tdfaConfig
		for _, cc := range childConfigs {
			result = append(result, tdfaConfig{
				NextNode: &LookBehindNode{Child: cc.NextNode},
				Tags:     cc.Tags,
			})
		}
		return result
	case *IntersectNode:
		leftConfigs := stepTDFA(node.Left, c)
		rightConfigs := stepTDFA(node.Right, c)
		var result []tdfaConfig
		for _, lc := range leftConfigs {
			for _, rc := range rightConfigs {
				if lc.NextNode.Equals(rc.NextNode) {
					// Both branches agree on next node; merge tags (order: left then right)
					tags := append([]tagOp{}, lc.Tags...)
					tags = append(tags, rc.Tags...)
					result = append(result, tdfaConfig{NextNode: lc.NextNode, Tags: tags})
					break
				}
			}
		}
		return result
	case *ComplementNode:
		childConfigs := stepTDFA(node.Child, c)
		var result []tdfaConfig
		for _, cc := range childConfigs {
			result = append(result, tdfaConfig{
				NextNode: NewComplementNode(cc.NextNode),
				Tags:     cc.Tags,
			})
		}
		return result
	default:
		return nil
	}
}

// InjectCaptureTags replaces each GroupNode(id, child) with Concat(Tag(id,start), Concat(child, Tag(id,end))).
// Used to build the tagged AST for the TDFA; the plain DFA never sees TagNodes.
func InjectCaptureTags(ast Node) Node {
	switch n := ast.(type) {
	case *GroupNode:
		inner := InjectCaptureTags(n.Child)
		return &ConcatNode{
			Left: &TagNode{Id: n.GroupID, IsStart: true},
			Right: &ConcatNode{
				Left:  inner,
				Right: &TagNode{Id: n.GroupID, IsStart: false},
			},
		}
	case *ConcatNode:
		return &ConcatNode{
			Left:  InjectCaptureTags(n.Left),
			Right: InjectCaptureTags(n.Right),
		}
	case *UnionNode:
		return &UnionNode{
			Left:  InjectCaptureTags(n.Left),
			Right: InjectCaptureTags(n.Right),
		}
	case *StarNode:
		return &StarNode{Child: InjectCaptureTags(n.Child)}
	case *IntersectNode:
		return &IntersectNode{
			Left:  InjectCaptureTags(n.Left),
			Right: InjectCaptureTags(n.Right),
		}
	case *ComplementNode:
		return &ComplementNode{Child: InjectCaptureTags(n.Child)}
	case *LookAheadNode:
		return &LookAheadNode{Child: InjectCaptureTags(n.Child)}
	case *LookBehindNode:
		return &LookBehindNode{Child: InjectCaptureTags(n.Child)}
	default:
		return ast
	}
}

// countCaptureGroups returns the number of capturing groups in the AST (GroupNode count).
func countCaptureGroups(ast Node) int {
	var count int
	var walk func(Node)
	walk = func(n Node) {
		if g, ok := n.(*GroupNode); ok {
			count++
			walk(g.Child)
			return
		}
		switch n := n.(type) {
		case *ConcatNode:
			walk(n.Left)
			walk(n.Right)
		case *UnionNode:
			walk(n.Left)
			walk(n.Right)
		case *StarNode:
			walk(n.Child)
		case *IntersectNode:
			walk(n.Left)
			walk(n.Right)
		case *ComplementNode:
			walk(n.Child)
		case *LookAheadNode:
			walk(n.Child)
		case *LookBehindNode:
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
	root        Node
	minterms    *MintermTable
	stateASTs   []Node
	transitions [][]tdfaTransition
	isMatch     []bool
	deadStateID int
	numCaptures int
}

func newLazyTDFA(taggedRoot Node, minterms *MintermTable, numCaptures int) *lazyTDFA {
	dead := &FalseNode{}
	dfa := &lazyTDFA{
		root:        taggedRoot,
		minterms:    minterms,
		stateASTs:   []Node{taggedRoot, dead},
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
// capture[1..numCaptures] with [start, end] byte indices. capture[0] is the full match.
func (dfa *lazyTDFA) runTDFA(s string) [][]int {
	ncap := dfa.numCaptures + 1
	capture := make([][]int, ncap)
	for i := range capture {
		capture[i] = []int{-1, -1}
	}
	state := 0
	pos := 0
	for pos <= len(s) {
		if dfa.isAccepting(state) {
			capture[0][0] = 0
			capture[0][1] = pos
		}
		if pos >= len(s) {
			break
		}
		r, size := utf8.DecodeRuneInString(s[pos:])
		mintermID := dfa.minterms.RuneToClass(r)
		nextState, tags := dfa.getNextState(state, mintermID)
		for _, t := range tags {
			if t.Id >= ncap {
				continue
			}
			if t.IsStart {
				capture[t.Id][0] = pos
			}
		}
		state = nextState
		pos += size
		for _, t := range tags {
			if t.Id >= ncap {
				continue
			}
			if !t.IsStart {
				capture[t.Id][1] = pos
			}
		}
	}
	if state == dfa.deadStateID {
		return nil
	}
	if !dfa.isAccepting(state) {
		return nil
	}
	if capture[0][0] < 0 {
		capture[0][0] = 0
		capture[0][1] = pos
	}
	return capture
}
