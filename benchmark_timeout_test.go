package re3

import (
	"strings"
	"testing"
	"time"
)

// TestBenchmarkTimeoutRegressions runs heavy benchmark patterns with a timeout
// to catch performance regressions (e.g. infinite loops, quadratic behavior).
func TestBenchmarkTimeoutRegressions(t *testing.T) {
	const fixtureRoot = "testdata/benchmarks"

	type timeoutCase struct {
		name     string
		timeout  time.Duration
		pattern  func(*testing.T) string
		haystack func(*testing.T) string
		run      func(RegExp, string)
	}

	cases := []timeoutCase{
		{
			name:    "dictionary/search/english-15",
			timeout: 30 * time.Second,
			pattern: func(t *testing.T) string {
				contents := readFixture(t, fixtureRoot+"/regexes/dictionary/english/length-15.txt")
				return buildPerLineAlternation(t, contents, false, true)
			},
			haystack: func(t *testing.T) string {
				return readFixture(t, fixtureRoot+"/haystacks/opensubtitles/en-medium.txt")
			},
			run: func(re RegExp, h string) { _ = re.FindAllStringIndex(h, -1) },
		},
		{
			name:    "grep/long-words-unicode",
			timeout: 15 * time.Second,
			pattern: func(t *testing.T) string { return `(?u:\b\w{25,}\b)` },
			haystack: func(t *testing.T) string {
				return readFixture(t, fixtureRoot+"/haystacks/rust-src-tools-3b0d4813.txt")
			},
			run: func(re RegExp, h string) { _ = grepCount(re, h) },
		},
		{
			name:    "imported/rsc/hard-1mb",
			timeout: 15 * time.Second,
			pattern: func(t *testing.T) string { return `[ -~]*ABCDEFGHIJKLMNOPQRSTUVWXYZ$` },
			haystack: func(t *testing.T) string {
				base := strings.TrimSpace(readFixture(t, fixtureRoot+"/haystacks/imported/rsc/1MB.txt"))
				return base + "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
			},
			run: func(re RegExp, h string) { _ = re.FindAllStringIndex(h, -1) },
		},
		{
			name:    "curated/07-unicode-character-data/parse-line",
			timeout: 60 * time.Second,
			pattern: func(t *testing.T) string {
				return strings.TrimSpace(readFixture(t, fixtureRoot+"/regexes/wild/ucd-parse.txt"))
			},
			haystack: func(t *testing.T) string {
				return readFixture(t, fixtureRoot+"/haystacks/wild/UnicodeData-15.0.0.txt")
			},
			run: func(re RegExp, h string) { _ = grepCapturesCount(re, h) },
		},
		{
			name:    "curated/09-aws-keys/full",
			timeout: 90 * time.Second,
			pattern: func(t *testing.T) string {
				return `(('|")((?:ASIA|AKIA|AROA|AIDA)([A-Z0-7]{16}))('|").*?(\n^.*?){0,4}(('|")[a-zA-Z0-9+/]{40}('|"))+|('|")[a-zA-Z0-9+/]{40}('|").*?(\n^.*?){0,3}('|")((?:ASIA|AKIA|AROA|AIDA)([A-Z0-7]{16}))('|"))+`
			},
			haystack: func(t *testing.T) string {
				return readFixture(t, fixtureRoot+"/haystacks/wild/cpython-226484e4.py")
			},
			run: func(re RegExp, h string) { _ = grepCapturesCount(re, h) },
		},
		{
			name:    "curated/10-bounded-repeat/letters-ru",
			timeout: 15 * time.Second,
			pattern: func(t *testing.T) string { return `(?u:\p{L}{8,13})` },
			haystack: func(t *testing.T) string {
				haystack := readFixture(t, fixtureRoot+"/haystacks/opensubtitles/ru-sampled.txt")
				lines := strings.Split(haystack, "\n")
				if len(lines) > 5000 {
					haystack = strings.Join(lines[:5000], "\n")
				}
				return haystack
			},
			run: func(re RegExp, h string) { _ = re.FindAllStringIndex(h, -1) },
		},
		{
			name:    "curated/12-dictionary/single",
			timeout: 30 * time.Second,
			pattern: func(t *testing.T) string {
				contents := readFixture(t, fixtureRoot+"/regexes/dictionary/english/length-15.txt")
				return buildPerLineAlternation(t, contents, true, false)
			},
			haystack: func(t *testing.T) string {
				return readFixture(t, fixtureRoot+"/haystacks/opensubtitles/en-medium.txt")
			},
			run: func(re RegExp, h string) { _ = re.FindAllStringIndex(h, -1) },
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if testing.Short() && (tc.name == "curated/09-aws-keys/full" || tc.name == "curated/07-unicode-character-data/parse-line") {
				t.Skip("skipping slow tests in short mode")
			}
			runWithTimeout(t, tc.timeout, func() {
				re := MustCompile(tc.pattern(t))
				tc.run(re, tc.haystack(t))
			})
		})
	}
}
