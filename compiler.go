package re3

// --- TYPES ---

type MintermTable struct {
	ByteToClass [256]int
	ClassToByte []byte
	NumClasses  int
}

// --- THE COMPILER PIPELINE ---
type RegExp struct {
	minterms              *MintermTable
	forwardTransitions    [][]int
	forwardIsMatch        []bool
	unanchoredTransitions [][]int // NEW: For the O(n) forward sweep
	unanchoredIsMatch     []bool  // NEW
	reverseTransitions    [][]int
	reverseIsMatch        []bool
}

type predicate [256]bool

func Compile(expr string) (*RegExp, error) {
	tokens := NewLexer(expr).LexAll()
	ast := NewParser(tokens).Parse()
	revAST := ast.Reverse()
	minterms := buildMintermTable(ast)

	// Build the unanchored AST: .* + your_regex
	unanchoredAST := NewConcatNode(&StarNode{Child: &AnyNode{}}, ast)

	// Compile the three DFAs
	ft, fm := compileDFA(ast, minterms)
	ut, um := compileDFA(unanchoredAST, minterms) // The O(n) forward hunter
	rt, rm := compileDFA(revAST, minterms)        // The O(n) backward hunter

	return &RegExp{
		minterms:              minterms,
		forwardTransitions:    ft,
		forwardIsMatch:        fm,
		unanchoredTransitions: ut,
		unanchoredIsMatch:     um,
		reverseTransitions:    rt,
		reverseIsMatch:        rm,
	}, nil
}

func MustCompile(expr string) *RegExp {
	re, err := Compile(expr)
	if err != nil {
		panic(err)
	}
	return re
}

// compileDFA generates the state machine using Brzozowski derivatives
func compileDFA(root Node, minterms *MintermTable) ([][]int, []bool) {
	var transitions [][]int
	var isMatch []bool
	stateASTs := []Node{root}
	queue := []int{0}

	for len(queue) > 0 {
		currentStateID := queue[0]
		queue = queue[1:]
		currentAST := stateASTs[currentStateID]

		isMatch = append(isMatch, currentAST.Nullable())
		stateTransitions := make([]int, minterms.NumClasses)

		for mintermID := 0; mintermID < minterms.NumClasses; mintermID++ {
			char := rune(minterms.ClassToByte[mintermID])
			nextAST := currentAST.Derivative(char)

			nextStateID := -1
			for id, seenAST := range stateASTs {
				if seenAST.Equals(nextAST) {
					nextStateID = id
					break
				}
			}

			if nextStateID == -1 {
				nextStateID = len(stateASTs)
				stateASTs = append(stateASTs, nextAST)
				queue = append(queue, nextStateID)
			}
			stateTransitions[mintermID] = nextStateID
		}
		transitions = append(transitions, stateTransitions)
	}
	return transitions, isMatch
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
