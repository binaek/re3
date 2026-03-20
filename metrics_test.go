package re3

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestCompileContextWorks verifies CompileContext and context-based APIs work.
func TestCompileContextWorks(t *testing.T) {
	ctx := context.Background()
	re, err := CompileContext(ctx, `(?u:\p{L}{3,5})`)
	if err != nil {
		t.Fatal(err)
	}
	haystack := strings.Repeat("hello world ", 1000)
	runWithTimeout(t, 5*time.Second, func() {
		_ = re.FindAllStringIndex(haystack, -1)
	})
	runWithTimeout(t, 5*time.Second, func() {
		_ = re.FindAllStringIndexContext(ctx, haystack, -1)
	})
}
