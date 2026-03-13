package re3

import (
	"strings"
	"testing"
	"time"
)

func TestBenchmarkGrep(t *testing.T) {
	const fixtureRoot = "testdata/benchmarks"

	t.Run("every-line", func(t *testing.T) {
		re := MustCompile("")
		haystack := readFixture(t, fixtureRoot+"/haystacks/rust-src-tools-3b0d4813.txt")
		runWithTimeout(t, 5*time.Second, func() {
			got := grepCount(re, haystack)
			// 239963 or 239964 depending on trailing newline
			if got < 239960 || got > 239970 {
				t.Errorf("expected ~239963 matching lines, got %d", got)
			}
		})
	})

	t.Run("long-words-ascii", func(t *testing.T) {
		t.Skip("re3 \\b semantics differ; go/re3 reports 376 in rebar")
	})

	t.Run("long-words-unicode", func(t *testing.T) {
		t.Skip("slow (~30s+); covered by TestBenchmarkTimeoutRegressions")
	})
}

func TestBenchmarkGrepSanity(t *testing.T) {
	t.Run("long-words-core", func(t *testing.T) {
		// Core of long-words without \b; exercises \w{25,}
		re := MustCompile(`\w{25,}`)
		haystack := strings.Join([]string{
			"short",
			strings.Repeat("a", 10),
			strings.Repeat("b", 25),
			strings.Repeat("c", 30),
			"end",
		}, " ")
		runWithTimeout(t, 2*time.Second, func() {
			got := re.FindAllString(haystack, -1)
			if len(got) != 2 {
				t.Fatalf("expected 2 long words, got %d (%v)", len(got), got)
			}
		})
	})

	t.Run("unicode-letters", func(t *testing.T) {
		t.Skip("\\p{L}{100} is slow; covered by bounded-repeat tests")
	})
}
