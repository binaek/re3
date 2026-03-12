package re3

import (
	"unicode"
	"unicode/utf8"
)

type matchContext struct {
	AtStart                  bool
	AtEnd                    bool
	PrevIsWord               bool
	NextIsWord               bool
	PrevIsNewline            bool
	NextIsNewline            bool
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
	// If pos is inside a UTF-8 sequence, treat context as being within one rune.
	// This suppresses false word-boundary transitions at non-boundary byte offsets.
	if pos > 0 && pos < len(s) && !utf8.RuneStart(s[pos]) {
		start := pos
		for start > 0 && !utf8.RuneStart(s[start]) {
			start--
		}
		r, _ := utf8.DecodeRuneInString(s[start:])
		w := isWordRune(r)
		ctx.PrevIsWord = w
		ctx.NextIsWord = w
		ctx.PrevIsNewline = false
		ctx.NextIsNewline = false
		return ctx
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

