package re3

import (
	"strings"
	"unicode/utf8"
)

// regexpImpl is the default lock-free implementation of RegExp.
type regexpImpl struct {
	minterms     *mintermTable
	forward      *lazyDFA
	unanchored   *lazyDFA
	reverse      *lazyDFA
	prefix       string    // optional literal prefix for Find fast-forward; empty means none
	CaptureCount int       // number of capture groups (GroupNodes)
	forwardTDFA  *lazyTDFA // built lazily when a submatch API is used
}

// Match reports whether the byte slice b contains any match of the regular expression.
func (re *regexpImpl) Match(b []byte) bool {
	state := 0
	for pos := 0; pos < len(b); {
		r, size := utf8.DecodeRune(b[pos:])
		mintermID := re.minterms.runeToClass(r)
		state = re.forward.getNextState(state, mintermID)
		pos += size
	}
	return re.forward.isAccepting(state)
}

// MatchString reports whether the string s contains any match of the regular expression.
// regexpImpl is not safe for concurrent use; use Clone() per goroutine or Concurrent() for a thread-safe wrapper.
func (re *regexpImpl) MatchString(s string) bool {
	state := 0
	for pos := 0; pos < len(s); {
		r, size := utf8.DecodeRuneInString(s[pos:])
		mintermID := re.minterms.runeToClass(r)
		state = re.forward.getNextState(state, mintermID)
		pos += size
	}
	return re.forward.isAccepting(state)
}

// FindIndex returns a two-element slice of integers defining the location of the leftmost match in b.
// The match itself is at b[loc[0]:loc[1]]. A return value of nil indicates no match.
func (re *regexpImpl) FindIndex(b []byte) []int {
	return re.FindStringIndex(string(b))
}

// FindStringIndex returns a two-element slice of integers defining the location of the leftmost match in s.
// The match itself is at s[loc[0]:loc[1]]. A return value of nil indicates no match.
func (re *regexpImpl) FindStringIndex(s string) []int {
	if len(s) == 0 {
		if re.forward.isAccepting(0) {
			return []int{0, 0}
		}
		return nil
	}

	bytePos := 0
	if len(re.prefix) > 0 {
		idx := strings.Index(s[bytePos:], re.prefix)
		if idx < 0 {
			return nil
		}
		bytePos += idx
	}

	firstEnd := -1
	if re.unanchored.isAccepting(0) {
		firstEnd = 0
	}
	state := 0
	for firstEnd == -1 && bytePos < len(s) {
		r, size := utf8.DecodeRuneInString(s[bytePos:])
		mintermID := re.minterms.runeToClass(r)
		state = re.unanchored.getNextState(state, mintermID)

		if re.unanchored.isAccepting(state) {
			firstEnd = bytePos + size
			break
		}
		bytePos += size
	}

	if firstEnd == -1 {
		return nil
	}

	revState := 0
	leftmostStart := -1
	bytePos = firstEnd
	for bytePos > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:bytePos])
		bytePos -= size
		mintermID := re.minterms.runeToClass(r)
		revState = re.reverse.getNextState(revState, mintermID)
		if revState == re.reverse.deadStateID {
			break
		}
		if re.reverse.isAccepting(revState) {
			leftmostStart = bytePos
		}
	}
	if leftmostStart == -1 && re.reverse.isAccepting(0) {
		leftmostStart = firstEnd
	}

	fwdState := 0
	longestEnd := -1

	if re.forward.isAccepting(0) {
		longestEnd = leftmostStart
	}

	bytePos = leftmostStart
	for bytePos < len(s) {
		r, size := utf8.DecodeRuneInString(s[bytePos:])
		mintermID := re.minterms.runeToClass(r)
		fwdState = re.forward.getNextState(fwdState, mintermID)

		if re.forward.isAccepting(fwdState) {
			longestEnd = bytePos + size
		}

		isDead := !re.forward.isAccepting(fwdState)
		if isDead {
			for m := 0; m < re.minterms.NumClasses; m++ {
				if re.forward.getNextState(fwdState, m) != fwdState {
					isDead = false
					break
				}
			}
		}

		if isDead {
			break
		}
		bytePos += size
	}

	return []int{leftmostStart, longestEnd}
}

// Find returns a slice holding the text of the leftmost match in b. A return value of nil indicates no match.
func (re *regexpImpl) Find(b []byte) []byte {
	loc := re.FindStringIndex(string(b))
	if loc == nil {
		return nil
	}
	return b[loc[0]:loc[1]]
}

// FindString returns a string holding the text of the leftmost match in s.
func (re *regexpImpl) FindString(s string) string {
	loc := re.FindStringIndex(s)
	if loc == nil {
		return ""
	}
	return s[loc[0]:loc[1]]
}

// FindAll is the 'All' version of Find; it returns a slice of all successive matches of the expression in b.
// A return value of nil indicates no match.
func (re *regexpImpl) FindAll(b []byte, n int) [][]byte {
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
	var matches []string
	pos := 0

	for pos <= len(s) && (n < 0 || len(matches) < n) {
		loc := re.FindStringIndex(s[pos:])
		if loc == nil {
			break
		}

		start := pos + loc[0]
		end := pos + loc[1]
		matches = append(matches, s[start:end])

		if end == start {
			pos++
		} else {
			pos = end
		}
	}

	return matches
}

// FindAllIndex is the 'All' version of FindIndex; it returns a slice of all successive matches of the expression in b.
// A return value of nil indicates no match.
func (re *regexpImpl) FindAllIndex(b []byte, n int) [][]int {
	return re.FindAllStringIndex(string(b), n)
}

// FindAllStringIndex is the 'All' version of FindStringIndex; it returns a slice of all successive matches
// of the expression. A return value of nil indicates no match.
func (re *regexpImpl) FindAllStringIndex(s string, n int) [][]int {
	var matches [][]int
	pos := 0

	for pos <= len(s) && (n < 0 || len(matches) < n) {
		loc := re.FindStringIndex(s[pos:])
		if loc == nil {
			break
		}

		start := pos + loc[0]
		end := pos + loc[1]
		matches = append(matches, []int{start, end})

		if end == start {
			pos++
		} else {
			pos = end
		}
	}

	return matches
}

func (re *regexpImpl) FindSubmatch(b []byte) [][]byte {
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
	var out [][]string
	pos := 0
	for pos <= len(s) && (n < 0 || len(out) < n) {
		loc := re.FindStringIndex(s[pos:])
		if loc == nil {
			break
		}
		start := pos + loc[0]
		end := pos + loc[1]
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
			pos++
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
	return re.FindStringSubmatchIndex(string(b))
}

func (re *regexpImpl) FindStringSubmatchIndex(s string) []int {
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
	return re.FindAllStringSubmatchIndex(string(b), n)
}

func (re *regexpImpl) FindAllStringSubmatchIndex(s string, n int) [][]int {
	var out [][]int
	pos := 0
	for pos <= len(s) && (n < 0 || len(out) < n) {
		loc := re.FindStringIndex(s[pos:])
		if loc == nil {
			break
		}
		start := pos + loc[0]
		end := pos + loc[1]

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
			pos++
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
		loc := re.FindStringIndex(s[pos:])
		if loc == nil {
			result = append(result, s[pos:])
			break
		}

		start := pos + loc[0]
		end := pos + loc[1]

		result = append(result, s[pos:start])
		splits++
		if n > 0 && splits >= maxSplits {
			result = append(result, s[end:])
			break
		}

		if end == start {
			pos++
		} else {
			pos = end
		}
	}

	return result
}

// ReplaceAll returns a copy of src, replacing matches of the expression with repl.
func (re *regexpImpl) ReplaceAll(src, repl []byte) []byte {
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
			pos++
		} else {
			pos = end
		}
	}

	return buf
}

// ReplaceAllString returns a copy of s, replacing matches of the expression with repl.
func (re *regexpImpl) ReplaceAllString(s, repl string) string {
	var buf []byte
	pos := 0

	for pos <= len(s) {
		loc := re.FindStringIndex(s[pos:])
		if loc == nil {
			buf = append(buf, s[pos:]...)
			break
		}

		start := pos + loc[0]
		end := pos + loc[1]

		buf = append(buf, s[pos:start]...)
		buf = append(buf, repl...)

		if end == start {
			pos++
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
		// forwardTDFA not copied; built on first submatch use
	}
}
