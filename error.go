package re3

// An Error describes a failure to parse a regular expression
// and gives the offending expression. It mirrors regexp/syntax.Error.
type Error struct {
	Code ErrorCode
	Expr string
}

func (e *Error) Error() string {
	return "error parsing regexp: " + e.Code.String() + ": `" + e.Expr + "`"
}

// An ErrorCode describes a failure to parse a regular expression.
type ErrorCode string

const (
	ErrInternalError         ErrorCode = "invalid or unsupported syntax"
	ErrInvalidCharClass      ErrorCode = "invalid character class"
	ErrInvalidCharRange      ErrorCode = "invalid character class range"
	ErrInvalidEscape         ErrorCode = "invalid escape sequence"
	ErrInvalidNamedCapture   ErrorCode = "invalid named capture"
	ErrInvalidPerlOp         ErrorCode = "invalid or unsupported Perl syntax"
	ErrInvalidRepeatOp       ErrorCode = "invalid nested repetition operator"
	ErrInvalidRepeatSize     ErrorCode = "invalid repeat count"
	ErrInvalidUTF8           ErrorCode = "invalid UTF-8"
	ErrMissingBracket        ErrorCode = "missing closing ]"
	ErrMissingParen          ErrorCode = "missing closing )"
	ErrMissingRepeatArgument ErrorCode = "missing argument to repetition operator"
	ErrTrailingBackslash     ErrorCode = "trailing backslash at end of expression"
	ErrUnexpectedParen       ErrorCode = "unexpected )"
	ErrNestingDepth          ErrorCode = "expression nests too deeply"
	ErrLarge                 ErrorCode = "expression too large"
)

func (e ErrorCode) String() string {
	return string(e)
}
