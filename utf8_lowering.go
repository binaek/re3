package re3

import (
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

var (
	unicodePropertyCacheMu sync.RWMutex
	unicodePropertyCache   = make(map[string]node)

	runeRangeCacheMu sync.RWMutex
	runeRangeCache   = make(map[uint64]node)
)

func newAnyNode() node {
	return newAnyRuneNode(true)
}

func newAnyRuneNode(excludeNewline bool) node {
	var ascii predicate
	for b := 0; b < 0x80; b++ {
		if excludeNewline && b == '\n' {
			continue
		}
		ascii[b] = true
	}
	var branches []node
	branches = append(branches, &charClassNode{Pred: ascii})

	// 2-byte runes: [C2-DF][80-BF]
	branches = append(branches, seqNode(
		byteRangeNode(0xC2, 0xDF),
		byteRangeNode(0x80, 0xBF),
	))

	// 3-byte runes.
	branches = append(branches, seqNode(
		byteRangeNode(0xE0, 0xE0),
		byteRangeNode(0xA0, 0xBF),
		byteRangeNode(0x80, 0xBF),
	))
	branches = append(branches, seqNode(
		byteRangeNode(0xE1, 0xEC),
		byteRangeNode(0x80, 0xBF),
		byteRangeNode(0x80, 0xBF),
	))
	branches = append(branches, seqNode(
		byteRangeNode(0xED, 0xED),
		byteRangeNode(0x80, 0x9F),
		byteRangeNode(0x80, 0xBF),
	))
	branches = append(branches, seqNode(
		byteRangeNode(0xEE, 0xEF),
		byteRangeNode(0x80, 0xBF),
		byteRangeNode(0x80, 0xBF),
	))

	// 4-byte runes.
	branches = append(branches, seqNode(
		byteRangeNode(0xF0, 0xF0),
		byteRangeNode(0x90, 0xBF),
		byteRangeNode(0x80, 0xBF),
		byteRangeNode(0x80, 0xBF),
	))
	branches = append(branches, seqNode(
		byteRangeNode(0xF1, 0xF3),
		byteRangeNode(0x80, 0xBF),
		byteRangeNode(0x80, 0xBF),
		byteRangeNode(0x80, 0xBF),
	))
	branches = append(branches, seqNode(
		byteRangeNode(0xF4, 0xF4),
		byteRangeNode(0x80, 0x8F),
		byteRangeNode(0x80, 0xBF),
		byteRangeNode(0x80, 0xBF),
	))
	return unionNodes(branches...)
}

func byteRangeNode(start, end byte) node {
	var p predicate
	for b := start; b <= end; b++ {
		p[b] = true
		if b == 0xFF {
			break
		}
	}
	return &charClassNode{Pred: p}
}

func seqNode(parts ...node) node {
	if len(parts) == 0 {
		return &emptyNode{}
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out = newConcatNode(out, parts[i])
	}
	return out
}

func unionNodes(parts ...node) node {
	if len(parts) == 0 {
		return &falseNode{}
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out = newUnionNode(out, parts[i])
	}
	return out
}

type utf8TrieNode struct {
	term     bool
	children map[byte]*utf8TrieNode
}

func (n *utf8TrieNode) insert(bs []byte) {
	cur := n
	for _, b := range bs {
		if cur.children == nil {
			cur.children = make(map[byte]*utf8TrieNode)
		}
		next := cur.children[b]
		if next == nil {
			next = &utf8TrieNode{}
			cur.children[b] = next
		}
		cur = next
	}
	cur.term = true
}

func (n *utf8TrieNode) toNode() node {
	var branches []node
	if n.term {
		branches = append(branches, &emptyNode{})
	}
	if len(n.children) > 0 {
		keys := make([]int, 0, len(n.children))
		for b := range n.children {
			keys = append(keys, int(b))
		}
		sort.Ints(keys)
		for _, k := range keys {
			b := byte(k)
			branches = append(branches, seqNode(&literalNode{Value: b}, n.children[b].toNode()))
		}
	}
	return unionNodes(branches...)
}

func compileRuneRangeToBytes(min, max rune) node {
	if min > max {
		return &falseNode{}
	}
	key := (uint64(min) << 21) | uint64(max)
	runeRangeCacheMu.RLock()
	if cached, ok := runeRangeCache[key]; ok {
		runeRangeCacheMu.RUnlock()
		return cached
	}
	runeRangeCacheMu.RUnlock()

	root := &utf8TrieNode{}
	var buf [utf8.UTFMax]byte
	for r := min; r <= max; r++ {
		if !utf8.ValidRune(r) {
			continue
		}
		n := utf8.EncodeRune(buf[:], r)
		root.insert(buf[:n])
		if r == utf8.MaxRune {
			break
		}
	}
	out := root.toNode()
	runeRangeCacheMu.Lock()
	runeRangeCache[key] = out
	runeRangeCacheMu.Unlock()
	return out
}

func compileRuneTableToBytes(tab *unicode.RangeTable) node {
	if tab == nil {
		return &falseNode{}
	}
	var nodes []node
	for _, r16 := range tab.R16 {
		lo := rune(r16.Lo)
		hi := rune(r16.Hi)
		stride := rune(r16.Stride)
		if stride == 1 {
			nodes = append(nodes, compileRuneRangeToBytes(lo, hi))
			continue
		}
		for r := lo; r <= hi; r += stride {
			nodes = append(nodes, compileRuneRangeToBytes(r, r))
		}
	}
	for _, r32 := range tab.R32 {
		lo := rune(r32.Lo)
		hi := rune(r32.Hi)
		stride := rune(r32.Stride)
		if stride == 1 {
			nodes = append(nodes, compileRuneRangeToBytes(lo, hi))
			continue
		}
		for r := lo; r <= hi; r += stride {
			nodes = append(nodes, compileRuneRangeToBytes(r, r))
		}
	}
	if len(nodes) == 0 {
		return &falseNode{}
	}
	return unionNodes(nodes...)
}

func compileRuneTablesToBytes(tabs ...*unicode.RangeTable) node {
	root := &utf8TrieNode{}
	var buf [utf8.UTFMax]byte
	for _, tab := range tabs {
		if tab == nil {
			continue
		}
		for _, r16 := range tab.R16 {
			lo := rune(r16.Lo)
			hi := rune(r16.Hi)
			stride := rune(r16.Stride)
			if stride == 0 {
				continue
			}
			for r := lo; r <= hi; r += stride {
				if !utf8.ValidRune(r) {
					continue
				}
				n := utf8.EncodeRune(buf[:], r)
				root.insert(buf[:n])
				if r == utf8.MaxRune {
					break
				}
			}
		}
		for _, r32 := range tab.R32 {
			lo := rune(r32.Lo)
			hi := rune(r32.Hi)
			stride := rune(r32.Stride)
			if stride == 0 {
				continue
			}
			for r := lo; r <= hi; r += stride {
				if !utf8.ValidRune(r) {
					continue
				}
				n := utf8.EncodeRune(buf[:], r)
				root.insert(buf[:n])
				if r == utf8.MaxRune {
					break
				}
			}
		}
	}
	return root.toNode()
}

func compileUnicodeProperty(name string) node {
	key := normalizeUnicodePropertyName(name)
	switch key {
	case "l", "letter":
		return compileUnicodePropertyCached("letter", func() node {
			return compileRuneTableToBytes(unicode.Letter)
		})
	case "lu", "uppercaseletter":
		return compileUnicodePropertyCached("lu", func() node {
			return compileRuneTableToBytes(unicode.Upper)
		})
	case "ll", "lowercaseletter":
		return compileUnicodePropertyCached("ll", func() node {
			return compileRuneTableToBytes(unicode.Lower)
		})
	case "llorlu", "lowerorupper", "lowercaseoruppercase":
		return compileUnicodePropertyCached("llorlu", func() node {
			lower := unicode.Lower
			if tab, ok := unicode.Properties["Lowercase"]; ok {
				lower = tab
			}
			upper := unicode.Upper
			if tab, ok := unicode.Properties["Uppercase"]; ok {
				upper = tab
			}
			title := unicode.Title
			if tab, ok := unicode.Properties["Titlecase"]; ok {
				title = tab
			}
			return compileRuneTablesToBytes(lower, upper, title)
		})
	case "lt", "titlecaseletter":
		return compileUnicodePropertyCached("lt", func() node {
			return compileRuneTableToBytes(unicode.Title)
		})
	case "uppercase":
		return compileUnicodePropertyCached("uppercase", func() node {
			if tab, ok := unicode.Properties["Uppercase"]; ok {
				return compileRuneTableToBytes(tab)
			}
			return compileRuneTableToBytes(unicode.Upper)
		})
	case "lowercase":
		return compileUnicodePropertyCached("lowercase", func() node {
			if tab, ok := unicode.Properties["Lowercase"]; ok {
				return compileRuneTableToBytes(tab)
			}
			return compileRuneTableToBytes(unicode.Lower)
		})
	case "titlecase":
		return compileUnicodePropertyCached("titlecase", func() node {
			return compileRuneTableToBytes(unicode.Title)
		})
	case "lm", "modifierletter":
		return compileUnicodePropertyCached("lm", func() node {
			return compileRuneTableToBytes(unicode.Lm)
		})
	case "lo", "otherletter":
		return compileUnicodePropertyCached("lo", func() node {
			return compileRuneTableToBytes(unicode.Lo)
		})
	case "m", "mark":
		return compileUnicodePropertyCached("mark", func() node {
			return compileRuneTableToBytes(unicode.Mark)
		})
	case "mn":
		return compileUnicodePropertyCached("mn", func() node {
			return compileRuneTableToBytes(unicode.Mn)
		})
	case "mc":
		return compileUnicodePropertyCached("mc", func() node {
			return compileRuneTableToBytes(unicode.Mc)
		})
	case "me":
		return compileUnicodePropertyCached("me", func() node {
			return compileRuneTableToBytes(unicode.Me)
		})
	case "nd", "digit", "number", "n":
		return compileUnicodePropertyCached("nd", func() node {
			return compileRuneTableToBytes(unicode.Digit)
		})
	case "nl":
		return compileUnicodePropertyCached("nl", func() node {
			return compileRuneTableToBytes(unicode.Nl)
		})
	case "no":
		return compileUnicodePropertyCached("no", func() node {
			return compileRuneTableToBytes(unicode.No)
		})
	case "z", "zs", "zl", "zp", "whitespace", "space":
		return compileUnicodePropertyCached("whitespace", func() node {
			return compileRuneTableToBytes(unicode.White_Space)
		})
	case "pc":
		return compileUnicodePropertyCached("pc", func() node {
			return compileRuneTableToBytes(unicode.Pc)
		})
	case "joincontrol":
		return compileUnicodePropertyCached("joincontrol", func() node {
			var rt unicode.RangeTable
			rt.R16 = []unicode.Range16{{Lo: 0x200C, Hi: 0x200D, Stride: 1}}
			return compileRuneTableToBytes(&rt)
		})
	default:
		if tab, ok := unicode.Categories[strings.ToUpper(name)]; ok {
			return compileUnicodePropertyCached("cat:"+strings.ToUpper(name), func() node {
				return compileRuneTableToBytes(tab)
			})
		}
		if tab, ok := unicode.Scripts[name]; ok {
			return compileUnicodePropertyCached("script:"+name, func() node {
				return compileRuneTableToBytes(tab)
			})
		}
		if tab, ok := unicode.Properties[name]; ok {
			return compileUnicodePropertyCached("prop:"+name, func() node {
				return compileRuneTableToBytes(tab)
			})
		}
		upperName := strings.ToUpper(name)
		if tab, ok := unicode.Scripts[upperName]; ok {
			return compileUnicodePropertyCached("script:"+upperName, func() node {
				return compileRuneTableToBytes(tab)
			})
		}
		if tab, ok := unicode.Properties[upperName]; ok {
			return compileUnicodePropertyCached("prop:"+upperName, func() node {
				return compileRuneTableToBytes(tab)
			})
		}
		return &falseNode{}
	}
}

func compileUnicodePropertyCached(key string, build func() node) node {
	unicodePropertyCacheMu.RLock()
	if cached, ok := unicodePropertyCache[key]; ok {
		unicodePropertyCacheMu.RUnlock()
		return cached
	}
	unicodePropertyCacheMu.RUnlock()

	compiled := build()

	unicodePropertyCacheMu.Lock()
	if cached, ok := unicodePropertyCache[key]; ok {
		unicodePropertyCacheMu.Unlock()
		return cached
	}
	unicodePropertyCache[key] = compiled
	unicodePropertyCacheMu.Unlock()
	return compiled
}

func normalizeUnicodePropertyName(name string) string {
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, " ", "")
	return strings.ToLower(name)
}

func parseEscapedClassToken(rs []rune, i *int) (node, bool) {
	if *i >= len(rs) || rs[*i] != '\\' || *i+1 >= len(rs) {
		return nil, false
	}
	*i++
	ch := rs[*i]
	switch ch {
	case 'd', 'w', 's', 'D', 'W', 'S':
		return &charClassNode{Class: `\` + string(ch)}, true
	case 'n':
		return lowerRuneLiteral('\n'), true
	case 'r':
		return lowerRuneLiteral('\r'), true
	case 't':
		return lowerRuneLiteral('\t'), true
	case 'v':
		return lowerRuneLiteral('\v'), true
	case 'f':
		return lowerRuneLiteral('\f'), true
	case 'a':
		return lowerRuneLiteral('\a'), true
	case 'p', 'P':
		if *i+1 < len(rs) && rs[*i+1] == '{' {
			j := *i + 2
			for j < len(rs) && rs[j] != '}' {
				j++
			}
			if j < len(rs) {
				name := string(rs[*i+2 : j])
				*i = j
				base := compileUnicodeProperty(name)
				if ch == 'P' {
					return newIntersectNode(newAnyRuneNode(false), newComplementNode(base)), true
				}
				return base, true
			}
		} else if *i+1 < len(rs) {
			*i++
			base := compileUnicodeProperty(string(rs[*i]))
			if ch == 'P' {
				return newIntersectNode(newAnyRuneNode(false), newComplementNode(base)), true
			}
			return base, true
		}
	}
	return lowerRuneLiteral(ch), true
}

func parseClassAtom(rs []rune, i *int) node {
	if n, ok := parseEscapedClassToken(rs, i); ok {
		return n
	}
	return lowerRuneLiteral(rs[*i])
}

func parseClassAtomRune(rs []rune, i *int) (rune, bool) {
	if *i >= len(rs) {
		return 0, false
	}
	if rs[*i] == '\\' && *i+1 < len(rs) {
		*i++
		switch rs[*i] {
		case 'n':
			return '\n', true
		case 'r':
			return '\r', true
		case 't':
			return '\t', true
		case 'v':
			return '\v', true
		case 'f':
			return '\f', true
		case 'a':
			return '\a', true
		default:
			return rs[*i], true
		}
	}
	return rs[*i], true
}

func compileCharClassNode(classStr string, caseInsensitive bool, unicodeMode bool) node {
	// Property escapes tokenized directly as tokenCharClass (e.g. \p{L}).
	if strings.HasPrefix(classStr, `\p{`) || strings.HasPrefix(classStr, `\P{`) {
		rs := []rune(classStr)
		i := 0
		n, ok := parseEscapedClassToken(rs, &i)
		if ok {
			return n
		}
		return &charClassNode{Class: classStr}
	}

	rs := []rune(classStr)
	negate := false
	start := 0
	if len(rs) > 0 && rs[0] == '^' {
		negate = true
		start = 1
	}

	var bytePred predicate
	hasBytePred := false
	var nodes []node

	for i := start; i < len(rs); i++ {
		atomStart := i
		r1, ok1 := parseClassAtomRune(rs, &i)
		if !ok1 {
			break
		}
		if i+1 < len(rs)-0 && rs[i+1] == '-' && i+2 < len(rs) {
			i += 2
			r2, ok2 := parseClassAtomRune(rs, &i)
			if !ok2 {
				break
			}
			if r1 > r2 {
				r1, r2 = r2, r1
			}
			if caseInsensitive {
				for r := r1; r <= r2; r++ {
					appendRuneCaseFold(r, unicodeMode, &bytePred, &hasBytePred, &nodes)
					if r == utf8.MaxRune {
						break
					}
				}
				continue
			}
			if r1 <= 0x7F && r2 <= 0x7F {
				for b := byte(r1); b <= byte(r2); b++ {
					bytePred[b] = true
					if b == 0xFF {
						break
					}
				}
				hasBytePred = true
				continue
			}
			nodes = append(nodes, compileRuneRangeToBytes(r1, r2))
			continue
		}
		i = atomStart
		n := parseClassAtom(rs, &i)
		if lit, ok := n.(*literalNode); ok {
			if caseInsensitive {
				appendRuneCaseFold(rune(lit.Value), unicodeMode, &bytePred, &hasBytePred, &nodes)
			} else {
				bytePred[lit.Value] = true
				hasBytePred = true
			}
			continue
		}
		nodes = append(nodes, n)
	}

	if hasBytePred {
		nodes = append(nodes, &charClassNode{Pred: bytePred})
	}
	if len(nodes) == 0 {
		return &falseNode{}
	}
	classNode := unionNodes(nodes...)
	if negate {
		classNode = newIntersectNode(newAnyRuneNode(false), newComplementNode(classNode))
	}
	return classNode
}

func appendRuneCaseFold(r rune, unicodeMode bool, bytePred *predicate, hasBytePred *bool, nodes *[]node) {
	if !unicodeMode {
		if r >= 'a' && r <= 'z' {
			(*bytePred)[byte(r)] = true
			(*bytePred)[byte(r-'a'+'A')] = true
			*hasBytePred = true
			return
		}
		if r >= 'A' && r <= 'Z' {
			(*bytePred)[byte(r)] = true
			(*bytePred)[byte(r-'A'+'a')] = true
			*hasBytePred = true
			return
		}
		if r <= 0x7F {
			(*bytePred)[byte(r)] = true
			*hasBytePred = true
		}
		return
	}
	folds := map[rune]struct{}{r: {}}
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		folds[f] = struct{}{}
	}
	for fr := range folds {
		if fr <= 0x7F {
			(*bytePred)[byte(fr)] = true
			*hasBytePred = true
			continue
		}
		*nodes = append(*nodes, lowerRuneLiteral(fr))
	}
}
