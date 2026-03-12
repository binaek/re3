package re3

import (
	stdregexp "regexp"
	"strings"
	"unicode/utf8"
)

func advancePosAfterEmptyMatchString(s string, pos int) int {
	if pos >= len(s) {
		return pos + 1
	}
	_, size := utf8.DecodeRuneInString(s[pos:])
	if size <= 0 {
		return pos + 1
	}
	return pos + size
}

func advancePosAfterEmptyMatchBytes(b []byte, pos int) int {
	if pos >= len(b) {
		return pos + 1
	}
	_, size := utf8.DecodeRune(b[pos:])
	if size <= 0 {
		return pos + 1
	}
	return pos + size
}

// regexpImpl is the default lock-free implementation of RegExp.
type regexpImpl struct {
	minterms     *mintermTable
	forward      *lazyDFA
	unanchored   *lazyDFA
	reverse      *lazyDFA
	prefix       string    // optional literal prefix for Find fast-forward; empty means none
	CaptureCount int       // number of capture groups (GroupNodes)
	forwardTDFA  *lazyTDFA // built lazily when a submatch API is used
	hasAssertions bool
	stdlib       *stdregexp.Regexp
}

// Match reports whether the byte slice b contains any match of the regular expression.
func (re *regexpImpl) Match(b []byte) bool {
	if re.stdlib != nil {
		return re.stdlib.Match(b)
	}
	if re.hasAssertions {
		loc := re.FindStringIndex(string(b))
		return loc != nil && loc[0] == 0 && loc[1] == len(b)
	}
	state := 0
	for pos := 0; pos < len(b); pos++ {
		mintermID := re.minterms.ByteToClass[b[pos]]
		state = re.forward.getNextState(state, mintermID, matchContext{})
	}
	return re.forward.isAccepting(state)
}

// MatchString reports whether the string s contains any match of the regular expression.
// regexpImpl is not safe for concurrent use; use Clone() per goroutine or Concurrent() for a thread-safe wrapper.
func (re *regexpImpl) MatchString(s string) bool {
	if re.stdlib != nil {
		return re.stdlib.MatchString(s)
	}
	if re.hasAssertions {
		loc := re.FindStringIndex(s)
		return loc != nil && loc[0] == 0 && loc[1] == len(s)
	}
	state := 0
	for pos := 0; pos < len(s); pos++ {
		mintermID := re.minterms.ByteToClass[s[pos]]
		ctx := matchContext{}
		if re.hasAssertions {
			ctx = makeMatchContextString(s, pos)
		}
		state = re.forward.getNextState(state, mintermID, ctx)
	}
	if re.hasAssertions {
		return re.forward.isAcceptingWithContext(state, makeMatchContextString(s, len(s)))
	}
	return re.forward.isAccepting(state)
}

// FindIndex returns a two-element slice of integers defining the location of the leftmost match in b.
// The match itself is at b[loc[0]:loc[1]]. A return value of nil indicates no match.
func (re *regexpImpl) FindIndex(b []byte) []int {
	if re.stdlib != nil {
		return re.stdlib.FindIndex(b)
	}
	return re.FindStringIndex(string(b))
}

// FindStringIndex returns a two-element slice of integers defining the location of the leftmost match in s.
// The match itself is at s[loc[0]:loc[1]]. A return value of nil indicates no match.
func (re *regexpImpl) FindStringIndex(s string) []int {
	if re.stdlib != nil {
		return re.stdlib.FindStringIndex(s)
	}
	return re.findStringIndexFrom(s, 0)
}

func (re *regexpImpl) findStringIndexFrom(s string, from int) []int {
	if len(s) == 0 {
		acceptsEmpty := re.forward.isAccepting(0)
		if re.hasAssertions {
			acceptsEmpty = re.forward.isAcceptingWithContext(0, makeMatchContextString(s, 0))
		}
		if acceptsEmpty {
			return []int{0, 0}
		}
		return nil
	}

	if from < 0 {
		from = 0
	}
	if from > len(s) {
		return nil
	}
	if from == len(s) {
		acceptsEmpty := re.forward.isAccepting(0)
		if re.hasAssertions {
			acceptsEmpty = re.forward.isAcceptingWithContext(0, makeMatchContextString(s, from))
		}
		if acceptsEmpty {
			return []int{from, from}
		}
		return nil
	}

	bytePos := from
	if len(re.prefix) > 0 {
		idx := strings.Index(s[bytePos:], re.prefix)
		if idx < 0 {
			return nil
		}
		bytePos += idx
	}

	firstEnd := -1
	unanchoredAccepts := re.unanchored.isAccepting(0)
	if re.hasAssertions {
		unanchoredAccepts = re.unanchored.isAcceptingWithContext(0, makeMatchContextString(s, from))
	}
	if unanchoredAccepts {
		firstEnd = from
	}
	state := 0
	for firstEnd == -1 && bytePos < len(s) {
		mintermID := re.minterms.ByteToClass[s[bytePos]]
		ctx := matchContext{}
		if re.hasAssertions {
			ctx = makeMatchContextString(s, bytePos)
		}
		state = re.unanchored.getNextState(state, mintermID, ctx)

		accepts := re.unanchored.isAccepting(state)
		if re.hasAssertions {
			accepts = re.unanchored.isAcceptingWithContext(state, makeMatchContextString(s, bytePos+1))
		}
		if accepts {
			firstEnd = bytePos + 1
			break
		}
		if state == re.unanchored.deadStateID {
			break
		}
		bytePos++
	}

	if firstEnd == -1 {
		return nil
	}

	revState := 0
	leftmostStart := -1
	bytePos = firstEnd
	for bytePos > 0 {
		bytePos--
		mintermID := re.minterms.ByteToClass[s[bytePos]]
		ctx := matchContext{}
		if re.hasAssertions {
			ctx = makeMatchContextString(s, bytePos)
		}
		revState = re.reverse.getNextState(revState, mintermID, ctx)
		if revState == re.reverse.deadStateID {
			break
		}
		accepts := re.reverse.isAccepting(revState)
		if re.hasAssertions {
			accepts = re.reverse.isAcceptingWithContext(revState, makeMatchContextString(s, bytePos))
		}
		if accepts {
			leftmostStart = bytePos
		}
	}
	revStartAccepts := re.reverse.isAccepting(0)
	if re.hasAssertions {
		revStartAccepts = re.reverse.isAcceptingWithContext(0, makeMatchContextString(s, firstEnd))
	}
	if leftmostStart == -1 && revStartAccepts {
		leftmostStart = firstEnd
	}
	if leftmostStart < 0 {
		return nil
	}

	fwdState := 0
	longestEnd := -1

	fwdStartAccepts := re.forward.isAccepting(0)
	if re.hasAssertions {
		fwdStartAccepts = re.forward.isAcceptingWithContext(0, makeMatchContextString(s, leftmostStart))
	}
	if fwdStartAccepts {
		longestEnd = leftmostStart
	}

	bytePos = leftmostStart
	for bytePos < len(s) {
		mintermID := re.minterms.ByteToClass[s[bytePos]]
		ctx := matchContext{}
		if re.hasAssertions {
			ctx = makeMatchContextString(s, bytePos)
		}
		fwdState = re.forward.getNextState(fwdState, mintermID, ctx)

		accepts := re.forward.isAccepting(fwdState)
		if re.hasAssertions {
			accepts = re.forward.isAcceptingWithContext(fwdState, makeMatchContextString(s, bytePos+1))
		}
		if accepts {
			longestEnd = bytePos + 1
		}
		if fwdState == re.forward.deadStateID {
			break
		}
		bytePos++
	}

	return []int{leftmostStart, longestEnd}
}

// Find returns a slice holding the text of the leftmost match in b. A return value of nil indicates no match.
func (re *regexpImpl) Find(b []byte) []byte {
	if re.stdlib != nil {
		return re.stdlib.Find(b)
	}
	loc := re.FindStringIndex(string(b))
	if loc == nil {
		return nil
	}
	return b[loc[0]:loc[1]]
}

// FindString returns a string holding the text of the leftmost match in s.
func (re *regexpImpl) FindString(s string) string {
	if re.stdlib != nil {
		return re.stdlib.FindString(s)
	}
	loc := re.FindStringIndex(s)
	if loc == nil {
		return ""
	}
	return s[loc[0]:loc[1]]
}

// FindAll is the 'All' version of Find; it returns a slice of all successive matches of the expression in b.
// A return value of nil indicates no match.
func (re *regexpImpl) FindAll(b []byte, n int) [][]byte {
	if re.stdlib != nil {
		return re.stdlib.FindAll(b, n)
	}
	s := string(b)
	locs := re.FindAllStringIndex(s, n)
	if len(locs) == 0 {
		return nil
	}
	out := make([][]byte, len(locs))
	for i, loc := range locs {
		out[i] = b[loc[0]:loc[1]]
	}
	return out
}

// FindAllString is the 'All' version of FindString; it returns a slice of all successive matches of the expression.
// A return value of nil indicates no match.
func (re *regexpImpl) FindAllString(s string, n int) []string {
	if re.stdlib != nil {
		return re.stdlib.FindAllString(s, n)
	}
	var matches []string
	pos := 0

	for pos <= len(s) && (n < 0 || len(matches) < n) {
		loc := re.findStringIndexFrom(s, pos)
		if loc == nil {
			break
		}

		start := loc[0]
		end := loc[1]
		matches = append(matches, s[start:end])

		if end == start {
			nextPos := advancePosAfterEmptyMatchString(s, start)
			if nextPos <= pos {
				nextPos = pos + 1
			}
			pos = nextPos
		} else {
			pos = end
		}
	}

	return matches
}

// FindAllIndex is the 'All' version of FindIndex; it returns a slice of all successive matches of the expression in b.
// A return value of nil indicates no match.
func (re *regexpImpl) FindAllIndex(b []byte, n int) [][]int {
	if re.stdlib != nil {
		return re.stdlib.FindAllIndex(b, n)
	}
	return re.FindAllStringIndex(string(b), n)
}

// FindAllStringIndex is the 'All' version of FindStringIndex; it returns a slice of all successive matches
// of the expression. A return value of nil indicates no match.
func (re *regexpImpl) FindAllStringIndex(s string, n int) [][]int {
	if re.stdlib != nil {
		return re.stdlib.FindAllStringIndex(s, n)
	}
	var matches [][]int
	pos := 0

	for pos <= len(s) && (n < 0 || len(matches) < n) {
		loc := re.findStringIndexFrom(s, pos)
		if loc == nil {
			break
		}

		start := loc[0]
		end := loc[1]
		matches = append(matches, []int{start, end})

		if end == start {
			nextPos := advancePosAfterEmptyMatchString(s, start)
			if nextPos <= pos {
				nextPos = pos + 1
			}
			pos = nextPos
		} else {
			pos = end
		}
	}

	return matches
}

func (re *regexpImpl) FindSubmatch(b []byte) [][]byte {
	if re.stdlib != nil {
		return re.stdlib.FindSubmatch(b)
	}
	loc := re.FindStringIndex(string(b))
	if loc == nil {
		return nil
	}
	if re.CaptureCount == 0 {
		return [][]byte{b[loc[0]:loc[1]]}
	}
	if re.forwardTDFA == nil {
		re.forwardTDFA = newLazyTDFA(injectCaptureTags(re.forward.root), re.minterms, re.CaptureCount)
	}
	span := string(b[loc[0]:loc[1]])
	capture := re.forwardTDFA.runTDFA(span)
	if capture == nil {
		return nil
	}
	out := make([][]byte, 1+re.CaptureCount)
	out[0] = b[loc[0]:loc[1]]
	for i := 1; i <= re.CaptureCount; i++ {
		start, end := capture[2*i], capture[2*i+1]
		if start >= 0 && end >= start {
			out[i] = b[loc[0]+start : loc[0]+end]
		}
	}
	return out
}

// FindStringSubmatch returns a slice of strings holding the text of the leftmost match
// and its capture groups. result[0] is the full match, result[1] the first subgroup, etc.
// Unmatched groups are empty strings. Uses two-pass: DFA for match span, then TDFA for submatches.
func (re *regexpImpl) FindStringSubmatch(s string) []string {
	if re.stdlib != nil {
		return re.stdlib.FindStringSubmatch(s)
	}
	loc := re.FindStringIndex(s)
	if loc == nil {
		return nil
	}
	match := s[loc[0]:loc[1]]
	if re.CaptureCount == 0 {
		return []string{match}
	}
	if re.forwardTDFA == nil {
		re.forwardTDFA = newLazyTDFA(injectCaptureTags(re.forward.root), re.minterms, re.CaptureCount)
	}
	span := s[loc[0]:loc[1]]
	capture := re.forwardTDFA.runTDFA(span)
	if capture == nil {
		return nil
	}
	out := make([]string, 1+re.CaptureCount)
	out[0] = match
	for i := 1; i <= re.CaptureCount; i++ {
		start, end := capture[2*i], capture[2*i+1]
		if start >= 0 && end >= start {
			out[i] = span[start:end]
		} else {
			out[i] = ""
		}
	}
	return out
}

func (re *regexpImpl) FindAllSubmatch(b []byte, n int) [][][]byte {
	if re.stdlib != nil {
		return re.stdlib.FindAllSubmatch(b, n)
	}
	locs := re.FindAllStringSubmatchIndex(string(b), n)
	if len(locs) == 0 {
		return nil
	}
	var out [][][]byte
	for _, loc := range locs {
		match := make([][]byte, len(loc)/2)
		for i := 0; i < len(loc)/2; i++ {
			if loc[2*i] >= 0 {
				match[i] = b[loc[2*i]:loc[2*i+1]]
			}
		}
		out = append(out, match)
	}
	return out
}

// FindAllStringSubmatch returns a slice of slices of strings: each inner slice is
// the result of FindStringSubmatch for one match. If n >= 0, at most n matches are returned.
func (re *regexpImpl) FindAllStringSubmatch(s string, n int) [][]string {
	if re.stdlib != nil {
		return re.stdlib.FindAllStringSubmatch(s, n)
	}
	var out [][]string
	pos := 0
	for pos <= len(s) && (n < 0 || len(out) < n) {
		loc := re.findStringIndexFrom(s, pos)
		if loc == nil {
			break
		}
		start := loc[0]
		end := loc[1]
		span := s[start:end]
		if re.CaptureCount == 0 {
			out = append(out, []string{span})
		} else {
			if re.forwardTDFA == nil {
				re.forwardTDFA = newLazyTDFA(injectCaptureTags(re.forward.root), re.minterms, re.CaptureCount)
			}
			capture := re.forwardTDFA.runTDFA(span)
			if capture == nil {
				out = append(out, []string{span})
			} else {
				row := make([]string, 1+re.CaptureCount)
				row[0] = span
				for i := 1; i <= re.CaptureCount; i++ {
					start, end := capture[2*i], capture[2*i+1]
					if start >= 0 && end >= start {
						row[i] = span[start:end]
					} else {
						row[i] = ""
					}
				}
				out = append(out, row)
			}
		}
		if end == start {
			nextPos := advancePosAfterEmptyMatchString(s, start)
			if nextPos <= pos {
				nextPos = pos + 1
			}
			pos = nextPos
		} else {
			pos = end
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (re *regexpImpl) FindSubmatchIndex(b []byte) []int {
	if re.stdlib != nil {
		return re.stdlib.FindSubmatchIndex(b)
	}
	return re.FindStringSubmatchIndex(string(b))
}

func (re *regexpImpl) FindStringSubmatchIndex(s string) []int {
	if re.stdlib != nil {
		return re.stdlib.FindStringSubmatchIndex(s)
	}
	loc := re.FindStringIndex(s)
	if loc == nil {
		return nil
	}
	if re.CaptureCount == 0 {
		return []int{loc[0], loc[1]}
	}
	if re.forwardTDFA == nil {
		re.forwardTDFA = newLazyTDFA(injectCaptureTags(re.forward.root), re.minterms, re.CaptureCount)
	}
	span := s[loc[0]:loc[1]]
	capture := re.forwardTDFA.runTDFA(span)
	if capture == nil {
		return nil
	}
	// Mutate the fresh capture slice in-place to offset it to the global string index
	capture[0] = loc[0]
	capture[1] = loc[1]
	for i := 1; i <= re.CaptureCount; i++ {
		if capture[2*i] >= 0 {
			capture[2*i] += loc[0]
			capture[2*i+1] += loc[0]
		}
	}
	return capture
}

func (re *regexpImpl) FindAllSubmatchIndex(b []byte, n int) [][]int {
	if re.stdlib != nil {
		return re.stdlib.FindAllSubmatchIndex(b, n)
	}
	return re.FindAllStringSubmatchIndex(string(b), n)
}

func (re *regexpImpl) FindAllStringSubmatchIndex(s string, n int) [][]int {
	if re.stdlib != nil {
		return re.stdlib.FindAllStringSubmatchIndex(s, n)
	}
	var out [][]int
	pos := 0
	for pos <= len(s) && (n < 0 || len(out) < n) {
		loc := re.findStringIndexFrom(s, pos)
		if loc == nil {
			break
		}
		start := loc[0]
		end := loc[1]

		if re.CaptureCount == 0 {
			out = append(out, []int{start, end})
		} else {
			if re.forwardTDFA == nil {
				re.forwardTDFA = newLazyTDFA(injectCaptureTags(re.forward.root), re.minterms, re.CaptureCount)
			}
			span := s[start:end]
			capture := re.forwardTDFA.runTDFA(span)
			if capture == nil {
				out = append(out, []int{start, end})
			} else {
				capture[0] = start
				capture[1] = end
				for i := 1; i <= re.CaptureCount; i++ {
					if capture[2*i] >= 0 {
						capture[2*i] += start
						capture[2*i+1] += start
					}
				}
				out = append(out, capture)
			}
		}

		if end == start {
			nextPos := advancePosAfterEmptyMatchString(s, start)
			if nextPos <= pos {
				nextPos = pos + 1
			}
			pos = nextPos
		} else {
			pos = end
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Split slices s into substrings separated by the expression and returns a slice of
// the substrings between those expression matches. If n > 0, at most n substrings are
// returned; the last substring will be the unsplit remainder. If n <= 0, there is no limit.
func (re *regexpImpl) Split(s string, n int) []string {
	if re.stdlib != nil {
		return re.stdlib.Split(s, n)
	}
	if n == 0 {
		return nil
	}

	if n == 1 {
		return []string{s}
	}

	var result []string
	pos := 0
	splits := 0
	maxSplits := n - 1

	for pos <= len(s) {
		loc := re.findStringIndexFrom(s, pos)
		if loc == nil {
			result = append(result, s[pos:])
			break
		}

		start := loc[0]
		end := loc[1]

		result = append(result, s[pos:start])
		splits++
		if n > 0 && splits >= maxSplits {
			result = append(result, s[end:])
			break
		}

		if end == start {
			nextPos := advancePosAfterEmptyMatchString(s, start)
			if nextPos <= pos {
				nextPos = pos + 1
			}
			pos = nextPos
		} else {
			pos = end
		}
	}

	return result
}

// ReplaceAll returns a copy of src, replacing matches of the expression with repl.
func (re *regexpImpl) ReplaceAll(src, repl []byte) []byte {
	if re.stdlib != nil {
		return re.stdlib.ReplaceAll(src, repl)
	}
	var buf []byte
	pos := 0

	for pos <= len(src) {
		loc := re.FindIndex(src[pos:])
		if loc == nil {
			buf = append(buf, src[pos:]...)
			break
		}

		start := pos + loc[0]
		end := pos + loc[1]

		buf = append(buf, src[pos:start]...)
		buf = append(buf, repl...)

		if end == start {
			nextPos := advancePosAfterEmptyMatchBytes(src, start)
			if nextPos <= pos {
				nextPos = pos + 1
			}
			pos = nextPos
		} else {
			pos = end
		}
	}

	return buf
}

// ReplaceAllString returns a copy of s, replacing matches of the expression with repl.
func (re *regexpImpl) ReplaceAllString(s, repl string) string {
	if re.stdlib != nil {
		return re.stdlib.ReplaceAllString(s, repl)
	}
	var buf []byte
	pos := 0

	for pos <= len(s) {
		loc := re.findStringIndexFrom(s, pos)
		if loc == nil {
			buf = append(buf, s[pos:]...)
			break
		}

		start := loc[0]
		end := loc[1]

		buf = append(buf, s[pos:start]...)
		buf = append(buf, repl...)

		if end == start {
			nextPos := advancePosAfterEmptyMatchString(s, start)
			if nextPos <= pos {
				nextPos = pos + 1
			}
			pos = nextPos
		} else {
			pos = end
		}
	}

	return string(buf)
}

// Clone returns a new RegExp that shares minterms and root ASTs but has fresh
// lazy DFA caches. Safe to use the original and clone in different goroutines.
func (re *regexpImpl) Clone() RegExp {
	return &regexpImpl{
		minterms:     re.minterms,
		forward:      newLazyDFA(re.forward.root, re.minterms),
		unanchored:   newLazyDFA(re.unanchored.root, re.minterms),
		reverse:      newLazyDFA(re.reverse.root, re.minterms),
		prefix:       re.prefix,
		CaptureCount: re.CaptureCount,
		hasAssertions: re.hasAssertions,
		stdlib:       re.stdlib,
		// forwardTDFA not copied; built on first submatch use
	}
}
