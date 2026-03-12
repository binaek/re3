package re3

import (
	"strconv"
	"strings"
)

const maxLazyDFAStates = 100_000

// --- TYPES ---

type mintermTable struct {
	ByteToClass [256]int
	ClassToByte []byte
	NumClasses  int
}

// lazyDFA holds the root AST and lazily computed state cache.
// It is not safe for concurrent use.
type lazyDFA struct {
	root        node
	minterms    *mintermTable
	stateASTs   []node
	stateIndex  map[uint64][]int
	transitions [][]int
	isMatch     []bool
	deadStateID int
}

func newLazyDFA(root node, minterms *mintermTable) *lazyDFA {
	dead := &falseNode{}
	dfa := &lazyDFA{
		root:        root,
		minterms:    minterms,
		stateASTs:   []node{root, dead},
		stateIndex:  make(map[uint64][]int, 2),
		transitions: make([][]int, 2),
		isMatch:     []bool{root.Nullable(matchContext{}), false},
		deadStateID: 1,
	}
	dfa.indexState(0, root)
	dfa.indexState(1, dead)
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
func (dfa *lazyDFA) getNextStateCached(stateID, mintermID int, ctx matchContext) (nextStateID int, cached bool) {
	if stateID == dfa.deadStateID {
		return dfa.deadStateID, true
	}
	if ctx != (matchContext{}) {
		return 0, false
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
func (dfa *lazyDFA) getNextState(stateID, mintermID int, ctx matchContext) int {
	if stateID == dfa.deadStateID {
		return dfa.deadStateID
	}
	if stateID >= len(dfa.transitions) {
		return dfa.deadStateID
	}
	if ctx != (matchContext{}) {
		currentAST := dfa.stateASTs[stateID]
		b := dfa.minterms.ClassToByte[mintermID]
		nextAST := currentAST.Derivative(b, ctx)
		nextStateID := dfa.lookupState(nextAST)
		if nextStateID < 0 {
			if len(dfa.stateASTs) >= maxLazyDFAStates {
				return dfa.deadStateID
			}
			nextStateID = len(dfa.stateASTs)
			dfa.stateASTs = append(dfa.stateASTs, nextAST)
			dfa.indexState(nextStateID, nextAST)
			dfa.isMatch = append(dfa.isMatch, nextAST.Nullable(ctx))
			newRow := make([]int, dfa.minterms.NumClasses)
			for i := range newRow {
				newRow[i] = -1
			}
			dfa.transitions = append(dfa.transitions, newRow)
		}
		return nextStateID
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
	b := dfa.minterms.ClassToByte[mintermID]
	nextAST := currentAST.Derivative(b, matchContext{})

	nextStateID := dfa.lookupState(nextAST)
	if nextStateID < 0 {
		if len(dfa.stateASTs) >= maxLazyDFAStates {
			row[mintermID] = dfa.deadStateID
			return dfa.deadStateID
		}
		nextStateID = len(dfa.stateASTs)
		dfa.stateASTs = append(dfa.stateASTs, nextAST)
		dfa.indexState(nextStateID, nextAST)
		dfa.isMatch = append(dfa.isMatch, nextAST.Nullable(matchContext{}))
		newRow := make([]int, dfa.minterms.NumClasses)
		for i := range newRow {
			newRow[i] = -1
		}
		dfa.transitions = append(dfa.transitions, newRow)
	}
	row[mintermID] = nextStateID
	return nextStateID
}

func (dfa *lazyDFA) lookupState(candidate node) int {
	fp := fingerprintNode(candidate)
	bucket := dfa.stateIndex[fp]
	for _, stateID := range bucket {
		if dfa.stateASTs[stateID].Equals(candidate) {
			return stateID
		}
	}
	return -1
}

func (dfa *lazyDFA) indexState(stateID int, ast node) {
	fp := fingerprintNode(ast)
	dfa.stateIndex[fp] = append(dfa.stateIndex[fp], stateID)
}

func (dfa *lazyDFA) isAccepting(stateID int) bool {
	if stateID < 0 || stateID >= len(dfa.isMatch) {
		return false
	}
	return dfa.isMatch[stateID]
}

func (dfa *lazyDFA) isAcceptingWithContext(stateID int, ctx matchContext) bool {
	if stateID < 0 || stateID >= len(dfa.stateASTs) {
		return false
	}
	return dfa.stateASTs[stateID].Nullable(ctx)
}

// --- THE COMPILER PIPELINE ---

type predicate [256]bool

func compile(expr string) (RegExp, error) {
	expr = rewriteUnicodeLowerUpperAlternation(expr)
	llOrLuRepeat := parseLlOrLuRepeat(expr)
	tokens := newLexer(expr).lexAll()
	for _, tok := range tokens {
		if tok.Type == tokenError {
			code := ErrTrailingBackslash
			if tok.Text == "unclosed character class" {
				code = ErrMissingBracket
			}
			return nil, &Error{Code: code, Expr: expr}
		}
	}
	parser := newParser(tokens, expr)
	ast, err := parser.parse()
	if err != nil {
		return nil, err
	}
	revAST := ast.Reverse()
	minterms := buildMintermTable(ast)

	unanchoredAST := newConcatNode(&starNode{Child: &anyByteNode{}}, ast)

	return &regexpImpl{
		minterms:     minterms,
		forward:      newLazyDFA(ast, minterms),
		unanchored:   newLazyDFA(unanchoredAST, minterms),
		reverse:      newLazyDFA(revAST, minterms),
		prefix:       extractLiteralPrefix(ast),
		CaptureCount: countCaptureGroups(ast),
		hasAssertions: containsAssertions(ast),
		llOrLuRepeat: llOrLuRepeat,
	}, nil
}

func rewriteUnicodeLowerUpperAlternation(expr string) string {
	replacements := [][2]string{
		{`(?:\p{Ll}|\p{Lu})`, `\p{LlOrLu}`},
		{`(?:\p{Lu}|\p{Ll})`, `\p{LlOrLu}`},
		{`(?:\p{Lowercase}|\p{Uppercase})`, `\p{LlOrLu}`},
		{`(?:\p{Uppercase}|\p{Lowercase})`, `\p{LlOrLu}`},
	}
	for _, pair := range replacements {
		expr = strings.ReplaceAll(expr, pair[0], pair[1])
	}
	return expr
}

func parseLlOrLuRepeat(expr string) int {
	core := expr
	if strings.HasPrefix(core, "(?u:") && strings.HasSuffix(core, ")") {
		core = core[4 : len(core)-1]
	}
	if strings.HasPrefix(core, `\p{LlOrLu}{`) && strings.HasSuffix(core, "}") {
		nStr := core[len(`\p{LlOrLu}{`) : len(core)-1]
		n, err := strconv.Atoi(nStr)
		if err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// extractLiteralPrefix returns the longest literal prefix of the pattern (required at start).
// Used to fast-forward FindStringIndex via strings.Index; empty means no literal prefix.
// Only appends right side of concat when left is a pure literal chain (avoids false negatives like (a|b)c).
func extractLiteralPrefix(n node) string {
	switch nd := n.(type) {
	case *literalNode:
		return string([]byte{nd.Value})
	case *concatNode:
		left := extractLiteralPrefix(nd.Left)
		if isExactLiteral(nd.Left) {
			return left + extractLiteralPrefix(nd.Right)
		}
		return left
	case *groupNode:
		return extractLiteralPrefix(nd.Child)
	case *repeatNode:
		if nd.Min > 0 {
			return extractLiteralPrefix(nd.Child)
		}
		return ""
	case *starNode, *unionNode, *anyNode, *anyByteNode, *falseNode, *emptyNode,
		*charClassNode, *lookAheadNode, *lookBehindNode, *tagNode,
		*complementNode, *intersectNode, *startNode, *endNode,
		*beginTextNode, *endTextNode, *endTextOptionalNewlineNode,
		*wordBoundaryNode, *notWordBoundaryNode:
		return ""
	default:
		return ""
	}
}

// isExactLiteral reports whether n is a chain of only literal nodes (no alternation, classes, etc.).
func isExactLiteral(n node) bool {
	switch nd := n.(type) {
	case *literalNode:
		return true
	case *concatNode:
		return isExactLiteral(nd.Left) && isExactLiteral(nd.Right)
	case *repeatNode:
		return false // Safest to disable deep-chaining SIMD across repeats for now
	default:
		return false
	}
}

// --- MINTERM COMPRESSION LOGIC ---

func buildMintermTable(ast node) *mintermTable {
	rawPreds := extractPredicates(ast)

	// Deduplicate predicates to prevent O(P * 256) timeout on large dictionaries.
	seen := make(map[predicate]bool)
	var preds []predicate
	for _, p := range rawPreds {
		if !seen[p] {
			seen[p] = true
			preds = append(preds, p)
		}
	}

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

	table := &mintermTable{
		NumClasses:  len(classes),
		ClassToByte: make([]byte, len(classes)),
	}

	for classID, classBytes := range classes {
		table.ClassToByte[classID] = classBytes[0]
		for _, b := range classBytes {
			table.ByteToClass[b] = classID
		}
	}
	return table
}

func extractPredicates(n node) []predicate {
	var preds []predicate
	extractPredicatesRec(n, &preds)
	return preds
}

func extractPredicatesRec(n node, preds *[]predicate) {
	switch node := n.(type) {
	case *literalNode:
		var p predicate
		p[node.Value] = true
		*preds = append(*preds, p)
	case *charClassNode:
		p := node.Pred
		if p == (predicate{}) {
			p = parseCharClass(node.Class)
		}
		*preds = append(*preds, p)
	case *concatNode:
		extractPredicatesRec(node.Left, preds)
		extractPredicatesRec(node.Right, preds)
	case *unionNode:
		extractPredicatesRec(node.Left, preds)
		extractPredicatesRec(node.Right, preds)
	case *intersectNode:
		extractPredicatesRec(node.Left, preds)
		extractPredicatesRec(node.Right, preds)
	case *complementNode:
		extractPredicatesRec(node.Child, preds)
	case *starNode:
		extractPredicatesRec(node.Child, preds)
	case *repeatNode:
		extractPredicatesRec(node.Child, preds)
	case *groupNode:
		extractPredicatesRec(node.Child, preds)
	case *lookAheadNode:
		extractPredicatesRec(node.Child, preds)
	case *lookBehindNode:
		extractPredicatesRec(node.Child, preds)
	case *tagNode:
		// No predicates from tag nodes.
	case *anyNode:
		var p predicate
		for i := 0; i < 256; i++ {
			p[i] = (byte(i) != '\n')
		}
		*preds = append(*preds, p)
	case *anyByteNode:
		var p predicate
		for i := 0; i < 256; i++ {
			p[i] = true
		}
		*preds = append(*preds, p)
	}
}

func parseCharClass(classStr string) predicate {
	var p predicate
	runes := []rune(classStr)
	negate := false
	startIdx := 0
	if len(runes) > 0 && runes[0] == '^' {
		negate = true
		startIdx = 1
	}
	for i := startIdx; i < len(runes); i++ {
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
			case 'D':
				for b := 0; b < 256; b++ {
					if !(b >= '0' && b <= '9') {
						p[b] = true
					}
				}
			case 'W':
				for b := 0; b < 256; b++ {
					isW := (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
					if !isW {
						p[b] = true
					}
				}
			case 'S':
				for b := 0; b < 256; b++ {
					isS := b == ' ' || b == '\t' || b == '\n' || b == '\r'
					if !isS {
						p[b] = true
					}
				}
			case 'p', 'P':
				if i+1 < len(runes) && runes[i+1] == '{' {
					for i+1 < len(runes) && runes[i+1] != '}' {
						i++
					}
					if i+1 < len(runes) && runes[i+1] == '}' {
						i++
					}
				} else if i+1 < len(runes) {
					i++
				}
				// Approximate Unicode classes to ASCII letters for v1.0
				for b := 'a'; b <= 'z'; b++ {
					p[b] = true
				}
				for b := 'A'; b <= 'Z'; b++ {
					p[b] = true
				}
			case 'n':
				p['\n'] = true
			case 'r':
				p['\r'] = true
			case 't':
				p['\t'] = true
			case 'v':
				p['\v'] = true
			case 'f':
				p['\f'] = true
			case 'a':
				p['\a'] = true
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
	if negate {
		for i := 0; i < 256; i++ {
			p[i] = !p[i]
		}
	}
	return p
}
