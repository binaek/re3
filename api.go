package re3

import "unicode/utf8"

// MatchString reports whether the string s exactly matches the regular expression.
// RegExp is not safe for concurrent use; use Clone() per goroutine or Concurrent() for a thread-safe wrapper.
func (re *RegExp) MatchString(s string) bool {
	state := 0
	for pos := 0; pos < len(s); {
		r, size := utf8.DecodeRuneInString(s[pos:])
		mintermID := re.minterms.RuneToClass(r)
		state = re.forward.getNextState(state, mintermID)
		pos += size
	}
	return re.forward.isAccepting(state)
}

// Match reports whether the byte slice b exactly matches the regular expression.
func (re *RegExp) Match(b []byte) bool {
	state := 0
	for pos := 0; pos < len(b); {
		r, size := utf8.DecodeRune(b[pos:])
		mintermID := re.minterms.RuneToClass(r)
		state = re.forward.getNextState(state, mintermID)
		pos += size
	}
	return re.forward.isAccepting(state)
}

// FindStringIndex returns the [start, end] of the leftmost-longest match in O(n) time.
func (re *RegExp) FindStringIndex(s string) []int {
	if len(s) == 0 {
		if re.forward.isAccepting(0) {
			return []int{0, 0}
		}
		return nil
	}

	firstEnd := -1
	if re.unanchored.isAccepting(0) {
		firstEnd = 0
	}
	state := 0
	bytePos := 0
	for firstEnd == -1 && bytePos < len(s) {
		r, size := utf8.DecodeRuneInString(s[bytePos:])
		mintermID := re.minterms.RuneToClass(r)
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
		mintermID := re.minterms.RuneToClass(r)
		revState = re.reverse.getNextState(revState, mintermID)

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
		mintermID := re.minterms.RuneToClass(r)
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

// FindString returns a string holding the text of the leftmost-longest match.
func (re *RegExp) FindString(s string) string {
	loc := re.FindStringIndex(s)
	if loc == nil {
		return ""
	}
	return s[loc[0]:loc[1]]
}

// Find returns a slice holding the text of the leftmost-longest match in b.
func (re *RegExp) Find(b []byte) []byte {
	loc := re.FindStringIndex(string(b))
	if loc == nil {
		return nil
	}
	return b[loc[0]:loc[1]]
}

// FindIndex returns the [start, end] of the leftmost-longest match in b.
func (re *RegExp) FindIndex(b []byte) []int {
	return re.FindStringIndex(string(b))
}

// FindAllStringIndex returns a slice of all successive matches of the expression.
// If n >= 0, the function returns at most n matches. Each match is [start, end].
func (re *RegExp) FindAllStringIndex(s string, n int) [][]int {
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

// FindAll returns a slice of all successive matches of the expression in b.
// If n >= 0, the function returns at most n matches.
func (re *RegExp) FindAll(b []byte, n int) [][]byte {
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

// FindAllIndex returns a slice of all successive matches of the expression in b.
// If n >= 0, the function returns at most n matches.
func (re *RegExp) FindAllIndex(b []byte, n int) [][]int {
	return re.FindAllStringIndex(string(b), n)
}

// Split slices s into substrings separated by the expression and returns a slice of
// the substrings between those expression matches. If n > 0, at most n substrings are
// returned; the last substring will be the unsplit remainder. If n <= 0, there is no limit.
func (re *RegExp) Split(s string, n int) []string {
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

// ReplaceAllString returns a copy of s, replacing matches of the expression with repl.
func (re *RegExp) ReplaceAllString(s, repl string) string {
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

// FindAllString returns a slice of all successive matches of the expression.
// If n >= 0, the function returns at most n matches.
func (re *RegExp) FindAllString(s string, n int) []string {
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

// --- ConcurrentRegExp: thread-safe wrapper with double-checked locking ---

func (c *ConcurrentRegExp) MatchString(s string) bool {
	c.mu.RLock()
	state := 0
	cacheMiss := false
	for pos := 0; pos < len(s); {
		r, size := utf8.DecodeRuneInString(s[pos:])
		mintermID := c.re.minterms.RuneToClass(r)
		next, ok := c.re.forward.getNextStateCached(state, mintermID)
		if !ok {
			cacheMiss = true
			break
		}
		state = next
		pos += size
	}
	if cacheMiss {
		c.mu.RUnlock()
		c.mu.Lock()
		result := c.re.MatchString(s)
		c.mu.Unlock()
		return result
	}
	result := c.re.forward.isAccepting(state)
	c.mu.RUnlock()
	return result
}

func (c *ConcurrentRegExp) Match(b []byte) bool {
	c.mu.RLock()
	state := 0
	cacheMiss := false
	for pos := 0; pos < len(b); {
		r, size := utf8.DecodeRune(b[pos:])
		mintermID := c.re.minterms.RuneToClass(r)
		next, ok := c.re.forward.getNextStateCached(state, mintermID)
		if !ok {
			cacheMiss = true
			break
		}
		state = next
		pos += size
	}
	if cacheMiss {
		c.mu.RUnlock()
		c.mu.Lock()
		result := c.re.Match(b)
		c.mu.Unlock()
		return result
	}
	result := c.re.forward.isAccepting(state)
	c.mu.RUnlock()
	return result
}

func (c *ConcurrentRegExp) FindStringIndex(s string) []int {
	c.mu.RLock()
	loc, cacheMiss := c.findStringIndexCached(s)
	if !cacheMiss {
		c.mu.RUnlock()
		return loc
	}
	c.mu.RUnlock()
	c.mu.Lock()
	loc = c.re.FindStringIndex(s)
	c.mu.Unlock()
	return loc
}

// findStringIndexCached runs the 3-phase FindStringIndex using only getNextStateCached.
// Returns (result, false) on success, (nil, true) on cache miss.
func (c *ConcurrentRegExp) findStringIndexCached(s string) ([]int, bool) {
	re := c.re
	if len(s) == 0 {
		if re.forward.isAccepting(0) {
			return []int{0, 0}, false
		}
		return nil, false
	}
	firstEnd := -1
	if re.unanchored.isAccepting(0) {
		firstEnd = 0
	}
	state := 0
	bytePos := 0
	for firstEnd == -1 && bytePos < len(s) {
		r, size := utf8.DecodeRuneInString(s[bytePos:])
		mintermID := re.minterms.RuneToClass(r)
		next, ok := re.unanchored.getNextStateCached(state, mintermID)
		if !ok {
			return nil, true
		}
		state = next
		if re.unanchored.isAccepting(state) {
			firstEnd = bytePos + size
			break
		}
		bytePos += size
	}
	if firstEnd == -1 {
		return nil, false
	}
	revState := 0
	leftmostStart := -1
	bytePos = firstEnd
	for bytePos > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:bytePos])
		bytePos -= size
		mintermID := re.minterms.RuneToClass(r)
		next, ok := re.reverse.getNextStateCached(revState, mintermID)
		if !ok {
			return nil, true
		}
		revState = next
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
		mintermID := re.minterms.RuneToClass(r)
		next, ok := re.forward.getNextStateCached(fwdState, mintermID)
		if !ok {
			return nil, true
		}
		fwdState = next
		if re.forward.isAccepting(fwdState) {
			longestEnd = bytePos + size
		}
		isDead := !re.forward.isAccepting(fwdState)
		if isDead {
			for m := 0; m < re.minterms.NumClasses; m++ {
				next, ok := re.forward.getNextStateCached(fwdState, m)
				if !ok {
					return nil, true
				}
				if next != fwdState {
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
	return []int{leftmostStart, longestEnd}, false
}

func (c *ConcurrentRegExp) FindString(s string) string {
	loc := c.FindStringIndex(s)
	if loc == nil {
		return ""
	}
	return s[loc[0]:loc[1]]
}

func (c *ConcurrentRegExp) Find(b []byte) []byte {
	loc := c.FindStringIndex(string(b))
	if loc == nil {
		return nil
	}
	return b[loc[0]:loc[1]]
}

func (c *ConcurrentRegExp) FindIndex(b []byte) []int {
	return c.FindStringIndex(string(b))
}

func (c *ConcurrentRegExp) FindAllString(s string, n int) []string {
	var matches []string
	pos := 0
	for pos <= len(s) && (n < 0 || len(matches) < n) {
		loc := c.FindStringIndex(s[pos:])
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

func (c *ConcurrentRegExp) FindAllStringIndex(s string, n int) [][]int {
	var matches [][]int
	pos := 0
	for pos <= len(s) && (n < 0 || len(matches) < n) {
		loc := c.FindStringIndex(s[pos:])
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

func (c *ConcurrentRegExp) FindAll(b []byte, n int) [][]byte {
	s := string(b)
	locs := c.FindAllStringIndex(s, n)
	if len(locs) == 0 {
		return nil
	}
	out := make([][]byte, len(locs))
	for i, loc := range locs {
		out[i] = b[loc[0]:loc[1]]
	}
	return out
}

func (c *ConcurrentRegExp) FindAllIndex(b []byte, n int) [][]int {
	return c.FindAllStringIndex(string(b), n)
}

func (c *ConcurrentRegExp) Split(s string, n int) []string {
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
		loc := c.FindStringIndex(s[pos:])
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

func (c *ConcurrentRegExp) ReplaceAllString(s, repl string) string {
	var buf []byte
	pos := 0
	for pos <= len(s) {
		loc := c.FindStringIndex(s[pos:])
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
