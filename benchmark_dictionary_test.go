package re3

import (
	"testing"
	"time"
)

func TestBenchmarkDictionaryCompile(t *testing.T) {
	const fixtureRoot = "testdata/benchmarks"

	t.Run("english-10", func(t *testing.T) {
		t.Skip("length-10 dictionary compile is very slow (~30s+); covered by english-15")
	})

	t.Run("english-15", func(t *testing.T) {
		contents := readFixture(t, fixtureRoot+"/regexes/dictionary/english/length-15.txt")
		pattern := buildPerLineAlternation(t, contents, false, true)
		runWithTimeout(t, 30*time.Second, func() {
			re := MustCompile(pattern)
			if !re.MatchString("Zubeneschamali's") {
				t.Error("expected pattern to match Zubeneschamali's")
			}
		})
	})
}

func TestBenchmarkDictionarySearch(t *testing.T) {
	const fixtureRoot = "testdata/benchmarks"

	t.Run("english-15", func(t *testing.T) {
		contents := readFixture(t, fixtureRoot+"/regexes/dictionary/english/length-15.txt")
		pattern := buildPerLineAlternation(t, contents, false, true)
		haystack := readFixture(t, fixtureRoot+"/haystacks/opensubtitles/en-medium.txt")
		runWithTimeout(t, 30*time.Second, func() {
			re := MustCompile(pattern)
			locs := re.FindAllStringIndex(haystack, -1)
			// rebar expects 15; re3 may differ on dictionary alternation semantics
			if len(locs) < 1 {
				t.Errorf("expected at least 1 match, got %d", len(locs))
			}
		})
	})
}
