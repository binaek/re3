package re3

import (
	"context"
	"strings"
	"sync"
	"unicode/utf8"
)

func advancePosAfterEmptyMatchStringConcurrent(s string, pos int) int {
	if pos >= len(s) {
		return pos + 1
	}
	_, size := utf8.DecodeRuneInString(s[pos:])
	if size <= 0 {
		return pos + 1
	}
	return pos + size
}

func advancePosAfterEmptyMatchBytesConcurrent(b []byte, pos int) int {
	if pos >= len(b) {
		return pos + 1
	}
	_, size := utf8.DecodeRune(b[pos:])
	if size <= 0 {
		return pos + 1
	}
	return pos + size
}

// concurrentRegExpImpl is a thread-safe wrapper that implements RegExp.
type concurrentRegExpImpl struct {
	mu sync.RWMutex
	re *regexpImpl
}

// --- ConcurrentRegExp: thread-safe wrapper with double-checked locking ---

func (c *concurrentRegExpImpl) Match(b []byte) bool {
	c.mu.RLock()
	if c.re.hasAssertions {
		c.mu.RUnlock()
		c.mu.Lock()
		result := c.re.Match(b)
		c.mu.Unlock()
		return result
	}
	state := 0
	cacheMiss := false
	for pos := 0; pos < len(b); pos++ {
		mintermID := c.re.minterms.ByteToClass[b[pos]]
		next, ok := c.re.forward.getNextStateCached(state, mintermID, matchContext{})
		if !ok {
			cacheMiss = true
			break
		}
		state = next
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
	if c.re.hasAssertions {
		c.mu.RUnlock()
		c.mu.Lock()
		result := c.re.MatchString(s)
		c.mu.Unlock()
		return result
	}
	state := 0
	cacheMiss := false
	for pos := 0; pos < len(s); pos++ {
		mintermID := c.re.minterms.ByteToClass[s[pos]]
		next, ok := c.re.forward.getNextStateCached(state, mintermID, matchContext{})
		if !ok {
			cacheMiss = true
			break
		}
		state = next
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

func (c *concurrentRegExpImpl) findStringIndexFrom(s string, from int) []int {
	c.mu.Lock()
	loc := c.re.findStringIndexFrom(context.Background(), s, from)
	c.mu.Unlock()
	return loc
}

// findStringIndexCached runs the 3-phase FindStringIndex using only getNextStateCached.
// Returns (result, false) on success, (nil, true) on cache miss.
func (c *concurrentRegExpImpl) findStringIndexCached(s string) ([]int, bool) {
	re := c.re
	if re.hasAssertions {
		return nil, true
	}
	if re.llOrLuRepeat > 0 {
		return nil, true
	}
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
		mintermID := re.minterms.ByteToClass[s[bytePos]]
		next, ok := re.unanchored.getNextStateCached(state, mintermID, matchContext{})
		if !ok {
			return nil, true
		}
		state = next
		if re.unanchored.isAccepting(state) {
			firstEnd = bytePos + 1
			break
		}
		if state == re.unanchored.deadStateID {
			break
		}
		bytePos++
	}
	if firstEnd == -1 {
		return nil, false
	}
	revState := 0
	leftmostStart := -1
	bytePos = firstEnd
	for bytePos > 0 {
		bytePos--
		mintermID := re.minterms.ByteToClass[s[bytePos]]
		next, ok := re.reverse.getNextStateCached(revState, mintermID, matchContext{})
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
		mintermID := re.minterms.ByteToClass[s[bytePos]]
		next, ok := re.forward.getNextStateCached(fwdState, mintermID, matchContext{})
		if !ok {
			return nil, true
		}
		fwdState = next
		if re.forward.isAccepting(fwdState) {
			longestEnd = bytePos + 1
		}
		if fwdState == re.forward.deadStateID {
			break
		}
		bytePos++
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
		loc := c.findStringIndexFrom(s, pos)
		if loc == nil {
			break
		}
		start := loc[0]
		end := loc[1]
		matches = append(matches, s[start:end])
		if end == start {
			nextPos := advancePosAfterEmptyMatchStringConcurrent(s, start)
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

func (c *concurrentRegExpImpl) FindAllIndex(b []byte, n int) [][]int {
	return c.FindAllStringIndex(string(b), n)
}

func (c *concurrentRegExpImpl) FindAllStringIndex(s string, n int) [][]int {
	var matches [][]int
	pos := 0
	for pos <= len(s) && (n < 0 || len(matches) < n) {
		loc := c.findStringIndexFrom(s, pos)
		if loc == nil {
			break
		}
		start := loc[0]
		end := loc[1]
		matches = append(matches, []int{start, end})
		if end == start {
			nextPos := advancePosAfterEmptyMatchStringConcurrent(s, start)
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
			nextPos := advancePosAfterEmptyMatchStringConcurrent(s, start)
			if nextPos <= pos {
				nextPos = pos + 1
			}
			pos = nextPos
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
			nextPos := advancePosAfterEmptyMatchStringConcurrent(s, start)
			if nextPos <= pos {
				nextPos = pos + 1
			}
			pos = nextPos
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
		loc := c.findStringIndexFrom(s, pos)
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
			nextPos := advancePosAfterEmptyMatchStringConcurrent(s, start)
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
			nextPos := advancePosAfterEmptyMatchBytesConcurrent(src, start)
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

func (c *concurrentRegExpImpl) ReplaceAllString(s, repl string) string {
	var buf []byte
	pos := 0
	for pos <= len(s) {
		loc := c.findStringIndexFrom(s, pos)
		if loc == nil {
			buf = append(buf, s[pos:]...)
			break
		}
		start := loc[0]
		end := loc[1]
		buf = append(buf, s[pos:start]...)
		buf = append(buf, repl...)
		if end == start {
			nextPos := advancePosAfterEmptyMatchStringConcurrent(s, start)
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

func (c *concurrentRegExpImpl) Clone() RegExp {
	c.mu.RLock()
	re := c.re.Clone()
	c.mu.RUnlock()
	return re
}

func (c *concurrentRegExpImpl) InstanceID() uint64 {
	c.mu.RLock()
	id := c.re.instanceID
	c.mu.RUnlock()
	return id
}

// RegExpContext implementation: delegate to underlying regexpImpl with lock.
func (c *concurrentRegExpImpl) MatchContext(ctx context.Context, b []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.MatchContext(ctx, b)
}
func (c *concurrentRegExpImpl) MatchStringContext(ctx context.Context, s string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.MatchStringContext(ctx, s)
}
func (c *concurrentRegExpImpl) FindIndexContext(ctx context.Context, b []byte) []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.FindIndexContext(ctx, b)
}
func (c *concurrentRegExpImpl) FindStringIndexContext(ctx context.Context, s string) []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.FindStringIndexContext(ctx, s)
}
func (c *concurrentRegExpImpl) FindContext(ctx context.Context, b []byte) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.FindContext(ctx, b)
}
func (c *concurrentRegExpImpl) FindStringContext(ctx context.Context, s string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.FindStringContext(ctx, s)
}
func (c *concurrentRegExpImpl) FindAllContext(ctx context.Context, b []byte, n int) [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.FindAllContext(ctx, b, n)
}
func (c *concurrentRegExpImpl) FindAllStringContext(ctx context.Context, s string, n int) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.FindAllStringContext(ctx, s, n)
}
func (c *concurrentRegExpImpl) FindAllIndexContext(ctx context.Context, b []byte, n int) [][]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.FindAllIndexContext(ctx, b, n)
}
func (c *concurrentRegExpImpl) FindAllStringIndexContext(ctx context.Context, s string, n int) [][]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.FindAllStringIndexContext(ctx, s, n)
}
func (c *concurrentRegExpImpl) FindSubmatchContext(ctx context.Context, b []byte) [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.FindSubmatchContext(ctx, b)
}
func (c *concurrentRegExpImpl) FindStringSubmatchContext(ctx context.Context, s string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.FindStringSubmatchContext(ctx, s)
}
func (c *concurrentRegExpImpl) FindAllSubmatchContext(ctx context.Context, b []byte, n int) [][][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.FindAllSubmatchContext(ctx, b, n)
}
func (c *concurrentRegExpImpl) FindAllStringSubmatchContext(ctx context.Context, s string, n int) [][]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.FindAllStringSubmatchContext(ctx, s, n)
}
func (c *concurrentRegExpImpl) FindSubmatchIndexContext(ctx context.Context, b []byte) []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.FindSubmatchIndexContext(ctx, b)
}
func (c *concurrentRegExpImpl) FindStringSubmatchIndexContext(ctx context.Context, s string) []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.FindStringSubmatchIndexContext(ctx, s)
}
func (c *concurrentRegExpImpl) FindAllSubmatchIndexContext(ctx context.Context, b []byte, n int) [][]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.FindAllSubmatchIndexContext(ctx, b, n)
}
func (c *concurrentRegExpImpl) FindAllStringSubmatchIndexContext(ctx context.Context, s string, n int) [][]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.FindAllStringSubmatchIndexContext(ctx, s, n)
}
func (c *concurrentRegExpImpl) SplitContext(ctx context.Context, s string, n int) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.SplitContext(ctx, s, n)
}
func (c *concurrentRegExpImpl) ReplaceAllContext(ctx context.Context, src, repl []byte) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.ReplaceAllContext(ctx, src, repl)
}
func (c *concurrentRegExpImpl) ReplaceAllStringContext(ctx context.Context, s, repl string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.re.ReplaceAllStringContext(ctx, s, repl)
}
