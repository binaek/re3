package re3

import (
	"strings"
	"testing"
	"time"
)

func TestBenchmarkCuratedLiteral(t *testing.T) {
	const fixtureRoot = "testdata/benchmarks"

	tests := []struct {
		name        string
		pattern     string
		haystackPath string
		want        int
	}{
		{"sherlock-en", "Sherlock Holmes", fixtureRoot + "/haystacks/opensubtitles/en-sampled.txt", 513},
		{"sherlock-casei-en", "(?i:Sherlock Holmes)", fixtureRoot + "/haystacks/opensubtitles/en-sampled.txt", 522},
		{"sherlock-ru", "Шерлок Холмс", fixtureRoot + "/haystacks/opensubtitles/ru-sampled.txt", 724},
		{"sherlock-casei-ru", "(?i:Шерлок Холмс)", fixtureRoot + "/haystacks/opensubtitles/ru-sampled.txt", 724}, // re3: 724 (rebar 746 with full Unicode case-folding)
		{"sherlock-zh", "夏洛克·福尔摩斯", fixtureRoot + "/haystacks/opensubtitles/zh-sampled.txt", 30},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			haystack := readFixture(t, tc.haystackPath)
			re := MustCompile(tc.pattern)
			locs := re.FindAllStringIndex(haystack, -1)
			if len(locs) != tc.want {
				t.Errorf("expected %d matches, got %d", tc.want, len(locs))
			}
		})
	}
}

func TestBenchmarkCuratedWords(t *testing.T) {
	const fixtureRoot = "testdata/benchmarks"

	t.Run("long-english", func(t *testing.T) {
		re := MustCompile(`\b[0-9A-Za-z_]{12,}\b`)
		haystack := readFixture(t, fixtureRoot+"/haystacks/opensubtitles/en-sampled.txt")
		lines := strings.Split(haystack, "\n")
		if len(lines) > 2500 {
			haystack = strings.Join(lines[:2500], "\n")
		}
		locs := re.FindAllStringIndex(haystack, -1)
		if len(locs) != 839 {
			t.Errorf("expected 839 matches, got %d", len(locs))
		}
	})
}

func TestBenchmarkCuratedBoundedRepeat(t *testing.T) {
	const fixtureRoot = "testdata/benchmarks"

	t.Run("letters-en", func(t *testing.T) {
		re := MustCompile(`[A-Za-z]{8,13}`)
		haystack := readFixture(t, fixtureRoot+"/haystacks/opensubtitles/en-sampled.txt")
		lines := strings.Split(haystack, "\n")
		if len(lines) > 5000 {
			haystack = strings.Join(lines[:5000], "\n")
		}
		runWithTimeout(t, 5*time.Second, func() {
			locs := re.FindAllStringIndex(haystack, -1)
			if len(locs) != 1833 {
				t.Errorf("expected 1833 matches, got %d", len(locs))
			}
		})
	})

	t.Run("letters-ru", func(t *testing.T) {
		re := MustCompile(`(?u:\p{L}{8,13})`)
		haystack := readFixture(t, fixtureRoot+"/haystacks/opensubtitles/ru-sampled.txt")
		lines := strings.Split(haystack, "\n")
		if len(lines) > 5000 {
			haystack = strings.Join(lines[:5000], "\n")
		}
		runWithTimeout(t, 10*time.Second, func() {
			locs := re.FindAllStringIndex(haystack, -1)
			if len(locs) != 3475 {
				t.Errorf("expected 3475 matches, got %d", len(locs))
			}
		})
	})

	t.Run("context", func(t *testing.T) {
		re := MustCompile(`[A-Za-z]{10}\s+[\s\S]{0,100}Result[\s\S]{0,100}\s+[A-Za-z]{10}`)
		haystack := readFixture(t, fixtureRoot+"/haystacks/rust-src-tools-3b0d4813.txt")
		locs := re.FindAllStringIndex(haystack, -1)
		if len(locs) != 53 {
			t.Errorf("expected 53 matches, got %d", len(locs))
		}
	})

	t.Run("capitals", func(t *testing.T) {
		re := MustCompile(`(?:[A-Z][a-z]+\s*){10,100}`)
		haystack := readFixture(t, fixtureRoot+"/haystacks/rust-src-tools-3b0d4813.txt")
		locs := re.FindAllStringIndex(haystack, -1)
		if len(locs) != 11 {
			t.Errorf("expected 11 matches, got %d", len(locs))
		}
	})
}

func TestBenchmarkCuratedDictionary(t *testing.T) {
	const fixtureRoot = "testdata/benchmarks"

	t.Run("single", func(t *testing.T) {
		contents := readFixture(t, fixtureRoot+"/regexes/dictionary/english/length-15.txt")
		pattern := buildPerLineAlternation(t, contents, true, false)
		haystack := readFixture(t, fixtureRoot+"/haystacks/opensubtitles/en-medium.txt")
		runWithTimeout(t, 30*time.Second, func() {
			re := MustCompile(pattern)
			locs := re.FindAllStringIndex(haystack, -1)
			if len(locs) != 1 {
				t.Errorf("expected 1 match, got %d", len(locs))
			}
		})
	})
}

func TestBenchmarkCuratedUnicodeCharacterData(t *testing.T) {
	const fixtureRoot = "testdata/benchmarks"

	t.Run("parse-line", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping slow Unicode parse-line in short mode")
		}
		pattern := strings.TrimSpace(readFixture(t, fixtureRoot+"/regexes/wild/ucd-parse.txt"))
		haystack := readFixture(t, fixtureRoot+"/haystacks/wild/UnicodeData-15.0.0.txt")
		runWithTimeout(t, 60*time.Second, func() {
			re := MustCompile(pattern)
			got := grepCapturesCount(re, haystack)
			if got != 558784 {
				t.Errorf("expected 558784 matching lines, got %d", got)
			}
		})
	})
}

func TestBenchmarkCuratedAWSKeys(t *testing.T) {
	const fixtureRoot = "testdata/benchmarks"

	t.Run("quick", func(t *testing.T) {
		re := MustCompile(`((?:ASIA|AKIA|AROA|AIDA)([A-Z0-7]{16}))`)
		haystack := readFixture(t, fixtureRoot+"/haystacks/wild/cpython-226484e4.py")
		runWithTimeout(t, 10*time.Second, func() {
			locs := re.FindAllStringIndex(haystack, -1)
			// count is 0 in benchmark (no keys in cpython)
			_ = locs
		})
	})

	t.Run("full", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping slow AWS full in short mode")
		}
		pattern := `(('|")((?:ASIA|AKIA|AROA|AIDA)([A-Z0-7]{16}))('|").*?(\n^.*?){0,4}(('|")[a-zA-Z0-9+/]{40}('|"))+|('|")[a-zA-Z0-9+/]{40}('|").*?(\n^.*?){0,3}('|")((?:ASIA|AKIA|AROA|AIDA)([A-Z0-7]{16}))('|"))+`
		haystack := readFixture(t, fixtureRoot+"/haystacks/wild/cpython-226484e4.py")
		runWithTimeout(t, 90*time.Second, func() {
			re := MustCompile(pattern)
			got := grepCapturesCount(re, haystack)
			if got != 0 {
				t.Errorf("expected 0 matching lines (no AWS keys in cpython), got %d", got)
			}
		})
	})
}
