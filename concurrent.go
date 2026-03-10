package re3

import (
	"strings"
	"sync"
	"unicode/utf8"
)

// concurrentRegExpImpl is a thread-safe wrapper that implements RegExp.
type concurrentRegExpImpl struct {
	mu sync.RWMutex
	re *regexpImpl
}

// --- ConcurrentRegExp: thread-safe wrapper with double-checked locking ---

func (c *concurrentRegExpImpl) Match(b []byte) bool {
	c.mu.RLock()
	state := 0
	cacheMiss := false
	for pos := 0; pos < len(b); {
		r, size := utf8.DecodeRune(b[pos:])
		mintermID := c.re.minterms.runeToClass(r)
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

func (c *concurrentRegExpImpl) MatchString(s string) bool {
	c.mu.RLock()
	state := 0
	cacheMiss := false
	for pos := 0; pos < len(s); {
		r, size := utf8.DecodeRuneInString(s[pos:])
		mintermID := c.re.minterms.runeToClass(r)
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

func (c *concurrentRegExpImpl) FindIndex(b []byte) []int {
	return c.FindStringIndex(string(b))
}

func (c *concurrentRegExpImpl) FindStringIndex(s string) []int {
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
func (c *concurrentRegExpImpl) findStringIndexCached(s string) ([]int, bool) {
	re := c.re
	if len(s) == 0 {
		if re.forward.isAccepting(0) {
			return []int{0, 0}, false
		}
		return nil, false
	}
	bytePos := 0
	if len(re.prefix) > 0 {
		idx := strings.Index(s[bytePos:], re.prefix)
		if idx < 0 {
			return nil, false
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
		mintermID := re.minterms.runeToClass(r)
		next, ok := re.reverse.getNextStateCached(revState, mintermID)
		if !ok {
			return nil, true
		}
		revState = next
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

func (c *concurrentRegExpImpl) Find(b []byte) []byte {
	loc := c.FindStringIndex(string(b))
	if loc == nil {
		return nil
	}
	return b[loc[0]:loc[1]]
}

func (c *concurrentRegExpImpl) FindString(s string) string {
	loc := c.FindStringIndex(s)
	if loc == nil {
		return ""
	}
	return s[loc[0]:loc[1]]
}

func (c *concurrentRegExpImpl) FindAll(b []byte, n int) [][]byte {
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

func (c *concurrentRegExpImpl) FindAllString(s string, n int) []string {
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

func (c *concurrentRegExpImpl) FindAllIndex(b []byte, n int) [][]int {
	return c.FindAllStringIndex(string(b), n)
}

func (c *concurrentRegExpImpl) FindAllStringIndex(s string, n int) [][]int {
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

func (c *concurrentRegExpImpl) FindSubmatch(b []byte) [][]byte {
	c.mu.RLock()
	loc, cacheMiss := c.findStringIndexCached(string(b))
	if cacheMiss {
		c.mu.RUnlock()
		c.mu.Lock()
		out := c.re.FindSubmatch(b)
		c.mu.Unlock()
		return out
	}
	if loc == nil {
		c.mu.RUnlock()
		return nil
	}
	if c.re.CaptureCount == 0 {
		c.mu.RUnlock()
		return [][]byte{b[loc[0]:loc[1]]}
	}
	c.mu.RUnlock()
	c.mu.Lock()
	out := c.re.FindSubmatch(b)
	c.mu.Unlock()
	return out
}

func (c *concurrentRegExpImpl) FindStringSubmatch(s string) []string {
	c.mu.RLock()
	loc, cacheMiss := c.findStringIndexCached(s)
	if cacheMiss {
		c.mu.RUnlock()
		c.mu.Lock()
		out := c.re.FindStringSubmatch(s)
		c.mu.Unlock()
		return out
	}
	if loc == nil {
		c.mu.RUnlock()
		return nil
	}
	span := s[loc[0]:loc[1]]
	if c.re.CaptureCount == 0 {
		c.mu.RUnlock()
		return []string{span}
	}
	// Need TDFA for capture groups; may build or mutate lazyTDFA.
	c.mu.RUnlock()
	c.mu.Lock()
	out := c.re.FindStringSubmatch(s)
	c.mu.Unlock()
	return out
}

func (c *concurrentRegExpImpl) FindAllSubmatch(b []byte, n int) [][][]byte {
	locs := c.FindAllStringSubmatchIndex(string(b), n)
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

func (c *concurrentRegExpImpl) FindAllStringSubmatch(s string, n int) [][]string {
	c.mu.RLock()
	out, cacheMiss := c.findAllStringSubmatchCached(s, n)
	if !cacheMiss {
		c.mu.RUnlock()
		return out
	}
	c.mu.RUnlock()
	c.mu.Lock()
	out = c.re.FindAllStringSubmatch(s, n)
	c.mu.Unlock()
	return out
}

// findAllStringSubmatchCached runs FindAllStringSubmatch using only cached DFA lookups.
// Returns (result, false) when the whole run used cache; (nil, true) on first cache miss
// or when capture groups require TDFA.
func (c *concurrentRegExpImpl) findAllStringSubmatchCached(s string, n int) ([][]string, bool) {
	re := c.re
	if re.CaptureCount > 0 {
		return nil, true
	}
	var out [][]string
	pos := 0
	for pos <= len(s) && (n < 0 || len(out) < n) {
		loc, cacheMiss := c.findStringIndexCached(s[pos:])
		if cacheMiss {
			return nil, true
		}
		if loc == nil {
			break
		}
		start := pos + loc[0]
		end := pos + loc[1]
		out = append(out, []string{s[start:end]})
		if end == start {
			pos++
		} else {
			pos = end
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, false
}

func (c *concurrentRegExpImpl) FindSubmatchIndex(b []byte) []int {
	return c.FindStringSubmatchIndex(string(b))
}

func (c *concurrentRegExpImpl) FindStringSubmatchIndex(s string) []int {
	c.mu.RLock()
	loc, cacheMiss := c.findStringIndexCached(s)
	if cacheMiss {
		c.mu.RUnlock()
		c.mu.Lock()
		out := c.re.FindStringSubmatchIndex(s)
		c.mu.Unlock()
		return out
	}
	if loc == nil {
		c.mu.RUnlock()
		return nil
	}
	if c.re.CaptureCount == 0 {
		c.mu.RUnlock()
		return []int{loc[0], loc[1]}
	}
	c.mu.RUnlock()
	c.mu.Lock()
	out := c.re.FindStringSubmatchIndex(s)
	c.mu.Unlock()
	return out
}

func (c *concurrentRegExpImpl) FindAllSubmatchIndex(b []byte, n int) [][]int {
	return c.FindAllStringSubmatchIndex(string(b), n)
}

func (c *concurrentRegExpImpl) FindAllStringSubmatchIndex(s string, n int) [][]int {
	c.mu.RLock()
	if c.re.CaptureCount > 0 {
		c.mu.RUnlock()
		c.mu.Lock()
		out := c.re.FindAllStringSubmatchIndex(s, n)
		c.mu.Unlock()
		return out
	}
	out, cacheMiss := c.findAllStringSubmatchIndexCached(s, n)
	if !cacheMiss {
		c.mu.RUnlock()
		return out
	}
	c.mu.RUnlock()
	c.mu.Lock()
	res := c.re.FindAllStringSubmatchIndex(s, n)
	c.mu.Unlock()
	return res
}

func (c *concurrentRegExpImpl) findAllStringSubmatchIndexCached(s string, n int) ([][]int, bool) {
	if c.re.CaptureCount > 0 {
		return nil, true
	}
	var out [][]int
	pos := 0
	for pos <= len(s) && (n < 0 || len(out) < n) {
		loc, cacheMiss := c.findStringIndexCached(s[pos:])
		if cacheMiss {
			return nil, true
		}
		if loc == nil {
			break
		}
		start := pos + loc[0]
		end := pos + loc[1]
		out = append(out, []int{start, end})
		if end == start {
			pos++
		} else {
			pos = end
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, false
}

func (c *concurrentRegExpImpl) Split(s string, n int) []string {
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

func (c *concurrentRegExpImpl) ReplaceAll(src, repl []byte) []byte {
	var buf []byte
	pos := 0
	for pos <= len(src) {
		loc := c.FindIndex(src[pos:])
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

func (c *concurrentRegExpImpl) ReplaceAllString(s, repl string) string {
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

func (c *concurrentRegExpImpl) Clone() RegExp {
	c.mu.RLock()
	re := c.re.Clone()
	c.mu.RUnlock()
	return re
}
