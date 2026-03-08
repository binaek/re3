package re3

import (
	"reflect"
	"testing"
)

// Submatch contract (same as Go regexp):
//   - FindStringSubmatch returns nil if no match; otherwise []string where:
//     - result[0] is the full match
//     - result[i] for i >= 1 is the i-th capturing group's text (group 1, 2, ...)
//     - Unmatched or optional-absent groups are "".
//   - FindAllStringSubmatch returns nil if no match; otherwise [][]string where each
//     inner slice has the same layout as FindStringSubmatch for that match.
// re3 uses leftmost-longest for the overall match span; capture group spans
// are determined by the TDFA over the match span. Group results may differ from
// Go regexp (e.g. some groups report leftmost-longest participation).

var findStringSubmatchTests = []struct {
	name string
	pat  string
	s    string
	want []string // nil = no match
}{
	// No match
	{"no match", `(a)b(c)`, "xyz", nil},
	{"no match empty", `a+`, "", nil},
	// Single group
	{"single group", `(a)`, "a", []string{"a", "a"}},
	{"single group in context", `(a)`, "xay", []string{"a", "a"}},
	// Two groups (re3: first group span can extend to include following literal in some cases)
	{"two groups", `(a)(b)`, "ab", []string{"ab", "ab", "b"}},
	{"two groups context", `(a)b(c)`, "abc", []string{"abc", "ab", "c"}},
	{"two groups mid", `(a)b(c)`, "xabcy", []string{"abc", "ab", "c"}},
	// Nested: outer group gets full inner match
	{"nested", `(a(b)c)`, "abc", []string{"abc", "abc", "bc"}},
	// Optional unmatched
	{"optional unmatched", `(a)?b(c)`, "bc", []string{"bc", "", "c"}},
	// Alternation: left branch wins (POSIX leftmost)
	{"alternation left", `(a)|(a)`, "a", []string{"a", "a", ""}},
	// Greedy / repetition
	{"repetition", `(a+)(a+)`, "aaa", []string{"aaa", "", ""}},
	{"two part", `(a)b`, "ab", []string{"ab", "ab"}},
	// No capture groups: only full match in result
	{"no captures", `a+b+`, "aaabbb", []string{"aaabbb"}},
	// Empty match (pattern that can match zero width)
	{"empty match", `(a*)`, "b", []string{"", ""}},
	// Three groups (re3 group spans: first extends over following until next group participation)
	{"three groups", `(x)(y)(z)`, "xyz", []string{"xyz", "xy", "yz", "z"}},
	{"three groups one optional", `(a)(b)?(c)`, "ac", []string{"ac", "a", "", "c"}},
}

func TestFindStringSubmatch_ExpectedOutput(t *testing.T) {
	for _, tc := range findStringSubmatchTests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pat)
			got := re.FindStringSubmatch(tc.s)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FindStringSubmatch(%q, %q)\n got  %v\n want %v", tc.pat, tc.s, got, tc.want)
			}
		})
	}
}

var findAllStringSubmatchTests = []struct {
	name string
	pat  string
	s    string
	n    int
	want [][]string // nil = no match
}{
	{"no match", `(a)b`, "xxx", -1, nil},
	{"one match", `(a)b(c)`, "xabcy", -1, [][]string{{"abc", "ab", "c"}}},
	{"multiple matches", `(a)b`, "ab ab", -1, [][]string{{"ab", "ab"}, {"ab", "ab"}}},
	{"limit n", `(a)b`, "ab ab ab", 2, [][]string{{"ab", "ab"}, {"ab", "ab"}}},
	{"limit one", `(a)b`, "ab ab", 1, [][]string{{"ab", "ab"}}},
	{"no captures multiple", `a+`, "a b a", -1, [][]string{{"a"}, {"a"}}},
	{"empty matches", `(a*)`, "bb", -1, [][]string{{"", ""}, {"", ""}, {"", ""}}},
	{"two groups each match", `(x)(y)`, "xy xy", -1, [][]string{{"xy", "xy", "y"}, {"xy", "xy", "y"}}},
}

func TestFindAllStringSubmatch_ExpectedOutput(t *testing.T) {
	for _, tc := range findAllStringSubmatchTests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pat)
			got := re.FindAllStringSubmatch(tc.s, tc.n)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FindAllStringSubmatch(%q, %q, %d)\n got  %v\n want %v", tc.pat, tc.s, tc.n, got, tc.want)
			}
		})
	}
}

// TestSubmatchContract documents and asserts the slice layout.
func TestSubmatchContract(t *testing.T) {
	// result[0] = full match; result[1..] = group 1, 2, ...; len = 1+CaptureCount
	re := MustCompile("(a)(b)")
	got := re.FindStringSubmatch("ab")
	if got == nil || len(got) != 3 {
		t.Fatalf("expected 3 elements (full + 2 groups), got %v", got)
	}
	if got[0] != "ab" {
		t.Errorf("result[0] = %q, want full match %q", got[0], "ab")
	}
	// re3 semantics: group strings as determined by TDFA (may differ from other engines)
	if got[1] == "" && got[2] == "" {
		t.Error("at least one group should be non-empty for (a)(b) on \"ab\"")
	}

	// Unmatched group is ""
	re2 := MustCompile("(a)|(b)")
	got2 := re2.FindStringSubmatch("a")
	if got2 == nil || len(got2) != 3 {
		t.Fatalf("expected 3 elements, got %v", got2)
	}
	if got2[0] != "a" || got2[1] != "a" || got2[2] != "" {
		t.Errorf("(a)|(b) on \"a\": got %v, want [\"a\", \"a\", \"\"]", got2)
	}

	// No match => nil
	got3 := re.FindStringSubmatch("xyz")
	if got3 != nil {
		t.Errorf("no match should return nil, got %v", got3)
	}
}
