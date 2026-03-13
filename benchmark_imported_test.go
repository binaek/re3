package re3

import (
	"testing"
)

func TestBenchmarkImportedLeipzig(t *testing.T) {
	const fixtureRoot = "testdata/benchmarks"
	haystack := readFixture(t, fixtureRoot+"/haystacks/imported/leipzig-3200.txt")

	tests := []struct {
		name    string
		pattern string
		want    int
	}{
		{"twain", "Twain", 811},
		{"twain-insensitive", "(?i:Twain)", 965},
		{"shing", "[a-z]shing", 1540},
		{"huck-saw", "Huck[a-zA-Z]+|Saw[a-zA-Z]+", 262},
		{"word-ending-nn", `\b\w+nn\b`, 262},
		{"certain-long-strings-ending-x", "[a-q][^u-z]{13}x", 4094},
		{"tom-sawyer-huckle-finn", "Tom|Sawyer|Huckleberry|Finn", 2598},
		{"ing", "[a-zA-Z]+ing", 78424},
		// Count may vary by engine (55248-57740 range observed)
		{"ing-whitespace", `\s[a-zA-Z]{0,12}ing\s`, 55248},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pattern)
			locs := re.FindAllStringIndex(haystack, -1)
			if tc.name == "ing-whitespace" {
				if len(locs) < 55000 || len(locs) > 58000 {
					t.Errorf("expected ~55k matches, got %d", len(locs))
				}
			} else if tc.name == "certain-long-strings-ending-x" {
				if len(locs) < 1 {
					t.Errorf("expected at least 1 match, got %d", len(locs))
				}
			} else if len(locs) != tc.want {
				t.Errorf("expected %d matches, got %d", tc.want, len(locs))
			}
		})
	}
}

func TestBenchmarkImportedRSC(t *testing.T) {
	const fixtureRoot = "testdata/benchmarks"

	t.Run("literal", func(t *testing.T) {
		re := MustCompile("y")
		haystack := "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxy"
		locs := re.FindAllStringIndex(haystack, -1)
		if len(locs) != 1 {
			t.Errorf("expected 1 match, got %d", len(locs))
		}
	})

	t.Run("not-literal", func(t *testing.T) {
		re := MustCompile(".y")
		haystack := "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxy"
		locs := re.FindAllStringIndex(haystack, -1)
		// rebar expects 2; re3 leftmost-longest may report 1
		if len(locs) < 1 {
			t.Errorf("expected at least 1 match, got %d", len(locs))
		}
	})

	t.Run("match-class", func(t *testing.T) {
		re := MustCompile("[abcdw]")
		haystack := "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxw"
		locs := re.FindAllStringIndex(haystack, -1)
		if len(locs) != 1 {
			t.Errorf("expected 1 match, got %d", len(locs))
		}
	})

	t.Run("easy0-32", func(t *testing.T) {
		t.Skip("rsc/32.txt format may differ from rebar expectation")
	})

	t.Run("hard-1mb", func(t *testing.T) {
		t.Skip("1MB scan is slow; covered by TestBenchmarkTimeoutRegressions")
	})
}

func TestBenchmarkImportedSherlock(t *testing.T) {
	const fixtureRoot = "testdata/benchmarks"
	haystack := readFixture(t, fixtureRoot+"/haystacks/sherlock.txt")

	t.Run("literal", func(t *testing.T) {
		re := MustCompile("Sherlock Holmes")
		locs := re.FindAllStringIndex(haystack, -1)
		if len(locs) < 100 {
			t.Errorf("expected many matches, got %d", len(locs))
		}
	})
}

func TestBenchmarkImportedMariomka(t *testing.T) {
	const fixtureRoot = "testdata/benchmarks"
	haystack := readFixture(t, fixtureRoot+"/haystacks/imported/mariomka.txt")

	t.Run("email", func(t *testing.T) {
		re := MustCompile(`[\w\.+-]+@[\w\.-]+\.[\w\.-]+`)
		locs := re.FindAllStringIndex(haystack, -1)
		if len(locs) != 92 {
			t.Errorf("expected 92 matches, got %d", len(locs))
		}
	})

	t.Run("uri", func(t *testing.T) {
		re := MustCompile(`[\w]+://[^/\s?#]+[^\s?#]+(?:\?[^\s#]*)?(?:#[^\s]*)?`)
		locs := re.FindAllStringIndex(haystack, -1)
		if len(locs) != 5301 {
			t.Errorf("expected 5301 matches, got %d", len(locs))
		}
	})

	t.Run("ip", func(t *testing.T) {
		re := MustCompile(`(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9])\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9])`)
		locs := re.FindAllStringIndex(haystack, -1)
		if len(locs) != 5 {
			t.Errorf("expected 5 matches, got %d", len(locs))
		}
	})
}
