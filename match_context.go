package re3

import (
	"unicode"
	"unicode/utf8"
)

// Context mask bits for assertion caching. Used as (stateID, effectiveMask) cache key.
const (
	ctxMaskAtStart                   uint16 = 1 << 0
	ctxMaskAtEnd                     uint16 = 1 << 1
	ctxMaskPrevIsWord                uint16 = 1 << 2
	ctxMaskNextIsWord                uint16 = 1 << 3
	ctxMaskPrevIsASCIIWord           uint16 = 1 << 4
	ctxMaskNextIsASCIIWord           uint16 = 1 << 5
	ctxMaskPrevIsNewline             uint16 = 1 << 6
	ctxMaskNextIsNewline             uint16 = 1 << 7
	ctxMaskAtEndAfterOptionalNewline uint16 = 1 << 8
)

type matchContext struct {
	AtStart                   bool
	AtEnd                     bool
	PrevIsWord                bool // Unicode-aware word classification.
	NextIsWord                bool // Unicode-aware word classification.
	PrevIsASCIIWord           bool // ASCII-only word classification.
	NextIsASCIIWord           bool // ASCII-only word classification.
	PrevIsNewline             bool
	NextIsNewline             bool
	AtEndAfterOptionalNewline bool
}

func isWordRune(r rune) bool {
	if r == '_' || r == 0x200C || r == 0x200D {
		return true
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || unicode.Is(unicode.Pc, r)
}

func isASCIIWordRune(r rune) bool {
	return r == '_' || ('0' <= r && r <= '9') || ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z')
}

func makeMatchContextString(s string, pos int) matchContext {
	if pos < 0 {
		pos = 0
	}
	if pos > len(s) {
		pos = len(s)
	}
	ctx := matchContext{
		AtStart: pos == 0,
		AtEnd:   pos >= len(s),
	}
	// If pos is at a UTF-8 continuation byte (10xxxxxx), we are inside a multi-byte
	// rune. Boundaries (^, $, \b, \B) cannot exist here, so we return a context
	// that makes \b false and \B true without doing any rune decoding.
	if pos > 0 && pos < len(s) {
		b := s[pos]
		if b&0xC0 == 0x80 { // continuation byte in UTF-8
			ctx.PrevIsWord = true
			ctx.NextIsWord = true
			ctx.PrevIsASCIIWord = true
			ctx.NextIsASCIIWord = true
			ctx.PrevIsNewline = false
			ctx.NextIsNewline = false
			return ctx
		}
	}
	if pos > 0 {
		prev, _ := utf8.DecodeLastRuneInString(s[:pos])
		ctx.PrevIsWord = isWordRune(prev)
		ctx.PrevIsASCIIWord = isASCIIWordRune(prev)
		ctx.PrevIsNewline = prev == '\n'
	}
	if pos < len(s) {
		next, size := utf8.DecodeRuneInString(s[pos:])
		ctx.NextIsWord = isWordRune(next)
		ctx.NextIsASCIIWord = isASCIIWordRune(next)
		ctx.NextIsNewline = next == '\n'
		if ctx.NextIsNewline && pos+size == len(s) {
			ctx.AtEndAfterOptionalNewline = true
		}
	}
	if ctx.AtEnd {
		ctx.AtEndAfterOptionalNewline = true
	}
	return ctx
}

// matchContextToMask packs the 9 context booleans into a uint16 for use as cache key.
func matchContextToMask(m matchContext) uint16 {
	var mask uint16
	if m.AtStart {
		mask |= ctxMaskAtStart
	}
	if m.AtEnd {
		mask |= ctxMaskAtEnd
	}
	if m.PrevIsWord {
		mask |= ctxMaskPrevIsWord
	}
	if m.NextIsWord {
		mask |= ctxMaskNextIsWord
	}
	if m.PrevIsASCIIWord {
		mask |= ctxMaskPrevIsASCIIWord
	}
	if m.NextIsASCIIWord {
		mask |= ctxMaskNextIsASCIIWord
	}
	if m.PrevIsNewline {
		mask |= ctxMaskPrevIsNewline
	}
	if m.NextIsNewline {
		mask |= ctxMaskNextIsNewline
	}
	if m.AtEndAfterOptionalNewline {
		mask |= ctxMaskAtEndAfterOptionalNewline
	}
	return mask
}
