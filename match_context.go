package re3

import (
	"unicode"
	"unicode/utf8"
)

type matchContext struct {
	AtStart                   bool
	AtEnd                     bool
	PrevIsWord                bool
	NextIsWord                bool
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
			ctx.PrevIsNewline = false
			ctx.NextIsNewline = false
			return ctx
		}
	}
	if pos > 0 {
		prev, _ := utf8.DecodeLastRuneInString(s[:pos])
		ctx.PrevIsWord = isWordRune(prev)
		ctx.PrevIsNewline = prev == '\n'
	}
	if pos < len(s) {
		next, size := utf8.DecodeRuneInString(s[pos:])
		ctx.NextIsWord = isWordRune(next)
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
