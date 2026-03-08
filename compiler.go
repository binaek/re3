package re3

const maxLazyDFAStates = 100_000

// --- TYPES ---

type MintermTable struct {
	ByteToClass   [256]int
	ClassToByte   []byte
	ClassToRune   []rune // representative rune per class for derivative (rune-based)
	NumClasses    int
	highRuneClass int // class ID for runes >= 256
}

// RuneToClass returns the minterm class ID for rune r.
// For r < 256 uses the byte partition; for r >= 256 returns the single "high" class.
func (m *MintermTable) RuneToClass(r rune) int {
	if r < 256 {
		return m.ByteToClass[byte(r)]
	}
	return m.highRuneClass
}

// lazyDFA holds the root AST and lazily computed state cache.
// It is not safe for concurrent use.
type lazyDFA struct {
	root        Node
	minterms    *MintermTable
	stateASTs   []Node  // index = state ID; state 0 = root
	transitions [][]int // transitions[stateID][mintermID] = nextStateID; -1 = not computed
	isMatch     []bool  // isMatch[stateID]
	deadStateID int     // state that never accepts; used when state cap is exceeded
}

func newLazyDFA(root Node, minterms *MintermTable) *lazyDFA {
	dead := &FalseNode{}
	dfa := &lazyDFA{
		root:        root,
		minterms:    minterms,
		stateASTs:   []Node{root, dead},
		transitions: make([][]int, 2),
		isMatch:     []bool{root.Nullable(), false},
		deadStateID: 1,
	}
	dfa.transitions[0] = make([]int, minterms.NumClasses)
	for i := range dfa.transitions[0] {
		dfa.transitions[0][i] = -1
	}
	dfa.transitions[1] = make([]int, minterms.NumClasses)
	for i := range dfa.transitions[1] {
		dfa.transitions[1][i] = 1
	}
	return dfa
}

// getNextStateCached returns the next state ID if already cached; otherwise (0, false).
// Used by ConcurrentRegExp for a read-only fast path under RLock.
func (dfa *lazyDFA) getNextStateCached(stateID, mintermID int) (nextStateID int, cached bool) {
	if stateID == dfa.deadStateID {
		return dfa.deadStateID, true
	}
	if stateID < 0 || stateID >= len(dfa.transitions) {
		return 0, false
	}
	row := dfa.transitions[stateID]
	if row == nil {
		return 0, false
	}
	if row[mintermID] >= 0 {
		return row[mintermID], true
	}
	return 0, false
}

// getNextState returns the next state ID after reading mintermID from stateID.
// It computes and caches the derivative on first access.
func (dfa *lazyDFA) getNextState(stateID, mintermID int) int {
	if stateID == dfa.deadStateID {
		return dfa.deadStateID
	}
	if stateID >= len(dfa.transitions) {
		return dfa.deadStateID
	}
	row := dfa.transitions[stateID]
	if row == nil {
		row = make([]int, dfa.minterms.NumClasses)
		for i := range row {
			row[i] = -1
		}
		dfa.transitions[stateID] = row
	}
	if row[mintermID] >= 0 {
		return row[mintermID]
	}
	// Cache miss: compute derivative
	currentAST := dfa.stateASTs[stateID]
	r := rune(0)
	if mintermID < len(dfa.minterms.ClassToRune) {
		r = dfa.minterms.ClassToRune[mintermID]
	}
	nextAST := currentAST.Derivative(r)

	nextStateID := -1
	for id, seen := range dfa.stateASTs {
		if seen.Equals(nextAST) {
			nextStateID = id
			break
		}
	}
	if nextStateID < 0 {
		if len(dfa.stateASTs) >= maxLazyDFAStates {
			row[mintermID] = dfa.deadStateID
			return dfa.deadStateID
		}
		nextStateID = len(dfa.stateASTs)
		dfa.stateASTs = append(dfa.stateASTs, nextAST)
		dfa.isMatch = append(dfa.isMatch, nextAST.Nullable())
		newRow := make([]int, dfa.minterms.NumClasses)
		for i := range newRow {
			newRow[i] = -1
		}
		dfa.transitions = append(dfa.transitions, newRow)
	}
	row[mintermID] = nextStateID
	return nextStateID
}

func (dfa *lazyDFA) isAccepting(stateID int) bool {
	if stateID < 0 || stateID >= len(dfa.isMatch) {
		return false
	}
	return dfa.isMatch[stateID]
}

// --- THE COMPILER PIPELINE ---

type predicate [256]bool

func Compile(expr string) (RegExp, error) {
	tokens := NewLexer(expr).LexAll()
	for _, tok := range tokens {
		if tok.Type == TokenError {
			code := ErrTrailingBackslash
			if tok.Text == "unclosed character class" {
				code = ErrMissingBracket
			}
			return nil, &Error{Code: code, Expr: expr}
		}
	}
	ast, err := NewParser(tokens, expr).Parse()
	if err != nil {
		return nil, err
	}
	revAST := ast.Reverse()
	minterms := buildMintermTable(ast)

	unanchoredAST := NewConcatNode(&StarNode{Child: &AnyNode{}}, ast)

	return &regexpImpl{
		minterms:     minterms,
		forward:      newLazyDFA(ast, minterms),
		unanchored:   newLazyDFA(unanchoredAST, minterms),
		reverse:      newLazyDFA(revAST, minterms),
		prefix:       extractLiteralPrefix(ast),
		CaptureCount: countCaptureGroups(ast),
	}, nil
}

func MustCompile(expr string) RegExp {
	re, err := Compile(expr)
	if err != nil {
		panic(err)
	}
	return re
}

// Concurrent returns a thread-safe RegExp implementation. If re is already
// a ConcurrentRegExp it is returned unchanged; otherwise re must be from
// Compile/MustCompile and is wrapped in a new ConcurrentRegExp.
func Concurrent(re RegExp) RegExp {
	if c, ok := re.(*concurrentRegExpImpl); ok {
		return c
	}
	if impl, ok := re.(*regexpImpl); ok {
		return &concurrentRegExpImpl{re: impl}
	}
	return re
}

// extractLiteralPrefix returns the longest literal prefix of the pattern (required at start).
// Used to fast-forward FindStringIndex via strings.Index; empty means no literal prefix.
func extractLiteralPrefix(n Node) string {
	switch node := n.(type) {
	case *LiteralNode:
		return string(node.Value)
	case *ConcatNode:
		return extractLiteralPrefix(node.Left) + extractLiteralPrefix(node.Right)
	case *GroupNode:
		return extractLiteralPrefix(node.Child)
	case *StarNode, *UnionNode, *AnyNode, *FalseNode, *EmptyNode,
		*CharClassNode, *LookAheadNode, *LookBehindNode, *TagNode,
		*ComplementNode, *IntersectNode:
		return ""
	default:
		return ""
	}
}

// --- MINTERM COMPRESSION LOGIC ---

func buildMintermTable(ast Node) *MintermTable {
	preds := extractPredicates(ast)

	var initialClass []byte
	for i := 0; i < 256; i++ {
		initialClass = append(initialClass, byte(i))
	}

	classes := [][]byte{initialClass}

	for _, p := range preds {
		var nextClasses [][]byte
		for _, class := range classes {
			var matched, unmatched []byte
			for _, b := range class {
				if p[b] {
					matched = append(matched, b)
				} else {
					unmatched = append(unmatched, b)
				}
			}
			if len(matched) > 0 {
				nextClasses = append(nextClasses, matched)
			}
			if len(unmatched) > 0 {
				nextClasses = append(nextClasses, unmatched)
			}
		}
		classes = nextClasses
	}

	table := &MintermTable{
		NumClasses:    len(classes) + 1,
		ClassToByte:   make([]byte, len(classes)+1),
		ClassToRune:   make([]rune, len(classes)+1),
		highRuneClass: len(classes),
	}

	for classID, classBytes := range classes {
		table.ClassToByte[classID] = classBytes[0]
		table.ClassToRune[classID] = rune(classBytes[0])
		for _, b := range classBytes {
			table.ByteToClass[b] = classID
		}
	}
	table.ClassToRune[table.highRuneClass] = 0x100
	return table
}

func extractPredicates(node Node) []predicate {
	var preds []predicate

	switch n := node.(type) {
	case *LiteralNode:
		var p predicate
		if n.Value < 256 {
			p[n.Value] = true
		}
		preds = append(preds, p)
	case *CharClassNode:
		preds = append(preds, parseCharClass(n.Class))
	case *ConcatNode:
		preds = append(preds, extractPredicates(n.Left)...)
		preds = append(preds, extractPredicates(n.Right)...)
	case *UnionNode:
		preds = append(preds, extractPredicates(n.Left)...)
		preds = append(preds, extractPredicates(n.Right)...)
	case *IntersectNode:
		preds = append(preds, extractPredicates(n.Left)...)
		preds = append(preds, extractPredicates(n.Right)...)
	case *ComplementNode:
		preds = append(preds, extractPredicates(n.Child)...)
	case *StarNode:
		preds = append(preds, extractPredicates(n.Child)...)
	case *GroupNode:
		preds = append(preds, extractPredicates(n.Child)...)
	case *LookAheadNode:
		preds = append(preds, extractPredicates(n.Child)...)
	case *LookBehindNode:
		preds = append(preds, extractPredicates(n.Child)...)
	case *TagNode:
		// No character predicates; tag is zero-width.
	case *AnyNode:
		// Dot does not match newline; ensure \n gets its own minterm class so Derivative('\n') is used.
		var p predicate
		for i := 0; i < 256; i++ {
			p[i] = (byte(i) != '\n')
		}
		preds = append(preds, p)
	}
	return preds
}

func parseCharClass(classStr string) predicate {
	var p predicate
	runes := []rune(classStr)
	for i := 0; i < len(runes); i++ {
		if i+2 < len(runes) && runes[i+1] == '-' {
			start, end := runes[i], runes[i+2]
			for b := start; b <= end; b++ {
				if b < 256 {
					p[b] = true
				}
			}
			i += 2
		} else if runes[i] == '\\' && i+1 < len(runes) {
			i++
			switch runes[i] {
			case 'd':
				for b := '0'; b <= '9'; b++ {
					p[b] = true
				}
			case 'w':
				for b := 'a'; b <= 'z'; b++ {
					p[b] = true
				}
				for b := 'A'; b <= 'Z'; b++ {
					p[b] = true
				}
				for b := '0'; b <= '9'; b++ {
					p[b] = true
				}
				p['_'] = true
			case 's':
				p[' '] = true
				p['\t'] = true
				p['\n'] = true
				p['\r'] = true
			default:
				if runes[i] < 256 {
					p[runes[i]] = true
				}
			}
		} else {
			if runes[i] < 256 {
				p[runes[i]] = true
			}
		}
	}
	return p
}
