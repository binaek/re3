package re3

import (
	"context"
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

func newAnyNode(ctx context.Context) node {
	return newAnyRuneNode(ctx, true)
}

func newAnyRuneNode(ctx context.Context, excludeNewline bool) node {
	var ascii predicate
	for b := 0; b < 0x80; b++ {
		if excludeNewline && b == '\n' {
			continue
		}
		ascii[b] = true
	}
	var branches []node
	branches = append(branches, newCharClassNode(ctx, "", ascii))

	// 2-byte runes: [C2-DF][80-BF]
	branches = append(branches, seqNode(ctx,
		byteRangeNode(ctx, 0xC2, 0xDF),
		byteRangeNode(ctx, 0x80, 0xBF),
	))

	// 3-byte runes.
	branches = append(branches, seqNode(ctx,
		byteRangeNode(ctx, 0xE0, 0xE0),
		byteRangeNode(ctx, 0xA0, 0xBF),
		byteRangeNode(ctx, 0x80, 0xBF),
	))
	branches = append(branches, seqNode(ctx,
		byteRangeNode(ctx, 0xE1, 0xEC),
		byteRangeNode(ctx, 0x80, 0xBF),
		byteRangeNode(ctx, 0x80, 0xBF),
	))
	branches = append(branches, seqNode(ctx,
		byteRangeNode(ctx, 0xED, 0xED),
		byteRangeNode(ctx, 0x80, 0x9F),
		byteRangeNode(ctx, 0x80, 0xBF),
	))
	branches = append(branches, seqNode(ctx,
		byteRangeNode(ctx, 0xEE, 0xEF),
		byteRangeNode(ctx, 0x80, 0xBF),
		byteRangeNode(ctx, 0x80, 0xBF),
	))

	// 4-byte runes.
	branches = append(branches, seqNode(ctx,
		byteRangeNode(ctx, 0xF0, 0xF0),
		byteRangeNode(ctx, 0x90, 0xBF),
		byteRangeNode(ctx, 0x80, 0xBF),
		byteRangeNode(ctx, 0x80, 0xBF),
	))
	branches = append(branches, seqNode(ctx,
		byteRangeNode(ctx, 0xF1, 0xF3),
		byteRangeNode(ctx, 0x80, 0xBF),
		byteRangeNode(ctx, 0x80, 0xBF),
		byteRangeNode(ctx, 0x80, 0xBF),
	))
	branches = append(branches, seqNode(ctx,
		byteRangeNode(ctx, 0xF4, 0xF4),
		byteRangeNode(ctx, 0x80, 0x8F),
		byteRangeNode(ctx, 0x80, 0xBF),
		byteRangeNode(ctx, 0x80, 0xBF),
	))
	return unionNodes(ctx, branches...)
}

func byteRangeNode(ctx context.Context, start, end byte) node {
	var p predicate
	for b := start; b <= end; b++ {
		p[b] = true
		if b == 0xFF {
			break
		}
	}
	return newCharClassNode(ctx, "", p)
}

func seqNode(ctx context.Context, parts ...node) node {
	if len(parts) == 0 {
		return newEmptyNode(ctx)
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out = newConcatNode(ctx, out, parts[i])
	}
	return out
}

func unionNodes(ctx context.Context, parts ...node) node {
	if len(parts) == 0 {
		return newFalseNode(ctx)
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out = newUnionNode(ctx, out, parts[i])
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

func (n *utf8TrieNode) toNode(ctx context.Context) node {
	var branches []node
	if n.term {
		branches = append(branches, newEmptyNode(ctx))
	}
	if len(n.children) > 0 {
		keys := make([]int, 0, len(n.children))
		for b := range n.children {
			keys = append(keys, int(b))
		}
		sort.Ints(keys)
		for _, k := range keys {
			b := byte(k)
			branches = append(branches, seqNode(ctx, newLiteralNode(ctx, b), n.children[b].toNode(ctx)))
		}
	}
	return unionNodes(ctx, branches...)
}

func compileRuneRangeToBytes(ctx context.Context, min, max rune) node {
	if min > max {
		return newFalseNode(ctx)
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
	out := root.toNode(ctx)
	runeRangeCacheMu.Lock()
	runeRangeCache[key] = out
	runeRangeCacheMu.Unlock()
	return out
}

func compileRuneTableToBytes(ctx context.Context, tab *unicode.RangeTable) node {
	if tab == nil {
		return newFalseNode(ctx)
	}
	var nodes []node
	for _, r16 := range tab.R16 {
		lo := rune(r16.Lo)
		hi := rune(r16.Hi)
		stride := rune(r16.Stride)
		if stride == 1 {
			nodes = append(nodes, compileRuneRangeToBytes(ctx, lo, hi))
			continue
		}
		for r := lo; r <= hi; r += stride {
			nodes = append(nodes, compileRuneRangeToBytes(ctx, r, r))
		}
	}
	for _, r32 := range tab.R32 {
		lo := rune(r32.Lo)
		hi := rune(r32.Hi)
		stride := rune(r32.Stride)
		if stride == 1 {
			nodes = append(nodes, compileRuneRangeToBytes(ctx, lo, hi))
			continue
		}
		for r := lo; r <= hi; r += stride {
			nodes = append(nodes, compileRuneRangeToBytes(ctx, r, r))
		}
	}
	if len(nodes) == 0 {
		return newFalseNode(ctx)
	}
	return unionNodes(ctx, nodes...)
}

func compileRuneTablesToBytes(ctx context.Context, tabs ...*unicode.RangeTable) node {
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
	return root.toNode(ctx)
}

func compileUnicodeProperty(ctx context.Context, name string) node {
	key := normalizeUnicodePropertyName(name)
	switch key {
	case "l", "letter":
		return compileUnicodePropertyCached(ctx, "letter", func() node {
			return compileRuneTableToBytes(ctx, unicode.Letter)
		})
	case "lu", "uppercaseletter":
		return compileUnicodePropertyCached(ctx, "lu", func() node {
			return compileRuneTableToBytes(ctx, unicode.Upper)
		})
	case "ll", "lowercaseletter":
		return compileUnicodePropertyCached(ctx, "ll", func() node {
			return compileRuneTableToBytes(ctx, unicode.Lower)
		})
	case "llorlu", "lowerorupper", "lowercaseoruppercase":
		return compileUnicodePropertyCached(ctx, "llorlu", func() node {
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
			return compileRuneTablesToBytes(ctx, lower, upper, title)
		})
	case "lt", "titlecaseletter":
		return compileUnicodePropertyCached(ctx, "lt", func() node {
			return compileRuneTableToBytes(ctx, unicode.Title)
		})
	case "uppercase":
		return compileUnicodePropertyCached(ctx, "uppercase", func() node {
			if tab, ok := unicode.Properties["Uppercase"]; ok {
				return compileRuneTableToBytes(ctx, tab)
			}
			return compileRuneTableToBytes(ctx, unicode.Upper)
		})
	case "lowercase":
		return compileUnicodePropertyCached(ctx, "lowercase", func() node {
			if tab, ok := unicode.Properties["Lowercase"]; ok {
				return compileRuneTableToBytes(ctx, tab)
			}
			return compileRuneTableToBytes(ctx, unicode.Lower)
		})
	case "titlecase":
		return compileUnicodePropertyCached(ctx, "titlecase", func() node {
			return compileRuneTableToBytes(ctx, unicode.Title)
		})
	case "lm", "modifierletter":
		return compileUnicodePropertyCached(ctx, "lm", func() node {
			return compileRuneTableToBytes(ctx, unicode.Lm)
		})
	case "lo", "otherletter":
		return compileUnicodePropertyCached(ctx, "lo", func() node {
			return compileRuneTableToBytes(ctx, unicode.Lo)
		})
	case "m", "mark":
		return compileUnicodePropertyCached(ctx, "mark", func() node {
			return compileRuneTableToBytes(ctx, unicode.Mark)
		})
	case "mn":
		return compileUnicodePropertyCached(ctx, "mn", func() node {
			return compileRuneTableToBytes(ctx, unicode.Mn)
		})
	case "mc":
		return compileUnicodePropertyCached(ctx, "mc", func() node {
			return compileRuneTableToBytes(ctx, unicode.Mc)
		})
	case "me":
		return compileUnicodePropertyCached(ctx, "me", func() node {
			return compileRuneTableToBytes(ctx, unicode.Me)
		})
	case "nd", "digit", "number", "n":
		return compileUnicodePropertyCached(ctx, "nd", func() node {
			return compileRuneTableToBytes(ctx, unicode.Digit)
		})
	case "nl":
		return compileUnicodePropertyCached(ctx, "nl", func() node {
			return compileRuneTableToBytes(ctx, unicode.Nl)
		})
	case "no":
		return compileUnicodePropertyCached(ctx, "no", func() node {
			return compileRuneTableToBytes(ctx, unicode.No)
		})
	case "z", "zs", "zl", "zp", "whitespace", "space":
		return compileUnicodePropertyCached(ctx, "whitespace", func() node {
			return compileRuneTableToBytes(ctx, unicode.White_Space)
		})
	case "pc":
		return compileUnicodePropertyCached(ctx, "pc", func() node {
			return compileRuneTableToBytes(ctx, unicode.Pc)
		})
	case "joincontrol":
		return compileUnicodePropertyCached(ctx, "joincontrol", func() node {
			var rt unicode.RangeTable
			rt.R16 = []unicode.Range16{{Lo: 0x200C, Hi: 0x200D, Stride: 1}}
			return compileRuneTableToBytes(ctx, &rt)
		})
	default:
		if tab, ok := unicode.Categories[strings.ToUpper(name)]; ok {
			return compileUnicodePropertyCached(ctx, "cat:"+strings.ToUpper(name), func() node {
				return compileRuneTableToBytes(ctx, tab)
			})
		}
		if tab, ok := unicode.Scripts[name]; ok {
			return compileUnicodePropertyCached(ctx, "script:"+name, func() node {
				return compileRuneTableToBytes(ctx, tab)
			})
		}
		if tab, ok := unicode.Properties[name]; ok {
			return compileUnicodePropertyCached(ctx, "prop:"+name, func() node {
				return compileRuneTableToBytes(ctx, tab)
			})
		}
		upperName := strings.ToUpper(name)
		if tab, ok := unicode.Scripts[upperName]; ok {
			return compileUnicodePropertyCached(ctx, "script:"+upperName, func() node {
				return compileRuneTableToBytes(ctx, tab)
			})
		}
		if tab, ok := unicode.Properties[upperName]; ok {
			return compileUnicodePropertyCached(ctx, "prop:"+upperName, func() node {
				return compileRuneTableToBytes(ctx, tab)
			})
		}
		return newFalseNode(ctx)
	}
}

func compileUnicodePropertyCached(ctx context.Context, key string, build func() node) node {
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

func parseEscapedClassToken(ctx context.Context, rs []rune, i *int) (node, bool) {
	if *i >= len(rs) || rs[*i] != '\\' || *i+1 >= len(rs) {
		return nil, false
	}
	*i++
	ch := rs[*i]
	switch ch {
	case 'd', 'w', 's', 'D', 'W', 'S':
		return &charClassNode{Class: `\` + string(ch)}, true
	case 'n':
		return lowerRuneLiteral(ctx, '\n'), true
	case 'r':
		return lowerRuneLiteral(ctx, '\r'), true
	case 't':
		return lowerRuneLiteral(ctx, '\t'), true
	case 'v':
		return lowerRuneLiteral(ctx, '\v'), true
	case 'f':
		return lowerRuneLiteral(ctx, '\f'), true
	case 'a':
		return lowerRuneLiteral(ctx, '\a'), true
	case 'p', 'P':
		if *i+1 < len(rs) && rs[*i+1] == '{' {
			j := *i + 2
			for j < len(rs) && rs[j] != '}' {
				j++
			}
			if j < len(rs) {
				name := string(rs[*i+2 : j])
				*i = j
				base := compileUnicodeProperty(ctx, name)
				if ch == 'P' {
					return newIntersectNode(ctx, newAnyRuneNode(ctx, false), newComplementNode(ctx, base)), true
				}
				return base, true
			}
		} else if *i+1 < len(rs) {
			*i++
			base := compileUnicodeProperty(ctx, string(rs[*i]))
			if ch == 'P' {
				return newIntersectNode(ctx, newAnyRuneNode(ctx, false), newComplementNode(ctx, base)), true
			}
			return base, true
		}
	}
	return lowerRuneLiteral(ctx, ch), true
}

func parseClassAtom(ctx context.Context, rs []rune, i *int) node {
	if n, ok := parseEscapedClassToken(ctx, rs, i); ok {
		return n
	}
	return lowerRuneLiteral(ctx, rs[*i])
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

func compileCharClassNode(ctx context.Context, classStr string, caseInsensitive bool, unicodeMode bool) node {
	// Property escapes tokenized directly as tokenCharClass (e.g. \p{L}).
	if strings.HasPrefix(classStr, `\p{`) || strings.HasPrefix(classStr, `\P{`) {
		rs := []rune(classStr)
		i := 0
		n, ok := parseEscapedClassToken(ctx, rs, &i)
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
					appendRuneCaseFold(ctx, r, unicodeMode, &bytePred, &hasBytePred, &nodes)
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
			nodes = append(nodes, compileRuneRangeToBytes(ctx, r1, r2))
			continue
		}
		i = atomStart
		n := parseClassAtom(ctx, rs, &i)
		if lit, ok := n.(*literalNode); ok {
			if caseInsensitive {
				appendRuneCaseFold(ctx, rune(lit.Value), unicodeMode, &bytePred, &hasBytePred, &nodes)
			} else {
				bytePred[lit.Value] = true
				hasBytePred = true
			}
			continue
		}
		nodes = append(nodes, n)
	}

	if hasBytePred {
		nodes = append(nodes, newCharClassNode(ctx, "", bytePred))
	}
	if len(nodes) == 0 {
		return newFalseNode(ctx)
	}
	classNode := unionNodes(ctx, nodes...)
	if negate {
		classNode = newIntersectNode(ctx, newAnyRuneNode(ctx, false), newComplementNode(ctx, classNode))
	}
	return classNode
}

func appendRuneCaseFold(ctx context.Context, r rune, unicodeMode bool, bytePred *predicate, hasBytePred *bool, nodes *[]node) {
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
		*nodes = append(*nodes, lowerRuneLiteral(ctx, fr))
	}
}
