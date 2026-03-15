package re3

import "context"

// RegExpContext is the context-aware extension of RegExp. All methods accept ctx for
// tracing, cancellation, and propagation. RegExp implementations delegate to these
// methods with context.Background() when the non-Context API is used.
type RegExpContext interface {
	RegExp

	MatchContext(ctx context.Context, b []byte) bool
	MatchStringContext(ctx context.Context, s string) bool

	FindIndexContext(ctx context.Context, b []byte) []int
	FindStringIndexContext(ctx context.Context, s string) []int

	FindContext(ctx context.Context, b []byte) []byte
	FindStringContext(ctx context.Context, s string) string

	FindAllContext(ctx context.Context, b []byte, n int) [][]byte
	FindAllStringContext(ctx context.Context, s string, n int) []string

	FindAllIndexContext(ctx context.Context, b []byte, n int) [][]int
	FindAllStringIndexContext(ctx context.Context, s string, n int) [][]int

	FindSubmatchContext(ctx context.Context, b []byte) [][]byte
	FindStringSubmatchContext(ctx context.Context, s string) []string

	FindAllSubmatchContext(ctx context.Context, b []byte, n int) [][][]byte
	FindAllStringSubmatchContext(ctx context.Context, s string, n int) [][]string

	FindSubmatchIndexContext(ctx context.Context, b []byte) []int
	FindStringSubmatchIndexContext(ctx context.Context, s string) []int

	FindAllSubmatchIndexContext(ctx context.Context, b []byte, n int) [][]int
	FindAllStringSubmatchIndexContext(ctx context.Context, s string, n int) [][]int

	SplitContext(ctx context.Context, s string, n int) []string

	ReplaceAllContext(ctx context.Context, src, repl []byte) []byte
	ReplaceAllStringContext(ctx context.Context, s, repl string) string
}
