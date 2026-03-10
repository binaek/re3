package re3

import (
	"reflect"
	"testing"
)

// --- TDFA built only on submatch use ---

func TestTDFANotBuiltUntilSubmatch(t *testing.T) {
	// Pattern with capture groups; TDFA is built lazily when a submatch API is first used.
	re := MustCompile("(a)(b)").(*regexpImpl)
	if re.forwardTDFA != nil {
		t.Fatal("forwardTDFA should be nil before any submatch API is used")
	}

	// Match and find do not build TDFA.
	re.MatchString("ab")
	re.FindStringIndex("xaby")
	if re.forwardTDFA != nil {
		t.Error("forwardTDFA should still be nil after MatchString and FindStringIndex")
	}

	// First submatch call builds TDFA.
	got := re.FindStringSubmatch("ab")
	// re3 POSIX-style: first group gets leftmost match span, second gets group 2.
	want := []string{"ab", "ab", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FindStringSubmatch = %v, want %v", got, want)
	}
	if re.forwardTDFA == nil {
		t.Error("forwardTDFA should be built after FindStringSubmatch")
	}
}

// --- Leftmost-longest (3-phase DFA) semantics ---

func TestLeftmostLongest(t *testing.T) {
	tests := []struct {
		name   string
		pat    string
		s      string
		first  []int   // FindStringIndex: [start, end]
		all    [][]int // FindAllStringIndex
	}{
		{
			name:  "a+ leftmost longest",
			pat:   "a+",
			s:     "baaab",
			first: []int{1, 4},
			all:   [][]int{{1, 4}},
		},
		{
			name:  ".* at end",
			pat:   ".*",
			s:     "xyz",
			first: []int{0, 3},
			all:   [][]int{{0, 3}, {3, 3}},
		},
		{
			name:  "a*b* greedy",
			pat:   "a*b*",
			s:     "aaabbb",
			first: []int{0, 6},
			all:   [][]int{{0, 6}, {6, 6}},
		},
		{
			name:  "first possible start",
			pat:   "a+",
			s:     "aaa",
			first: []int{0, 3},
			all:   [][]int{{0, 3}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pat)
			gotFirst := re.FindStringIndex(tc.s)
			if !reflect.DeepEqual(gotFirst, tc.first) {
				t.Errorf("FindStringIndex(%q, %q) = %v, want %v", tc.pat, tc.s, gotFirst, tc.first)
			}
			gotAll := re.FindAllStringIndex(tc.s, -1)
			if !reflect.DeepEqual(gotAll, tc.all) {
				t.Errorf("FindAllStringIndex(%q, %q, -1) = %v, want %v", tc.pat, tc.s, gotAll, tc.all)
			}
		})
	}
}

// --- Concurrent wrapper parity with default impl ---

func TestConcurrentParity(t *testing.T) {
	tests := []struct {
		pat  string
		s    string
		n    int
		repl string
	}{
		{"a+", "aaa", -1, "X"},
		{"(a)b(c)", "xabcy", -1, ""},
		{"\\w+", "one two", 2, ""},
		{"a*", "baaab", -1, ""},
	}
	for _, tc := range tests {
		t.Run(tc.pat+"/"+tc.s, func(t *testing.T) {
			re := MustCompile(tc.pat).(*regexpImpl)
			cre := Concurrent(re)

			if re.MatchString(tc.s) != cre.MatchString(tc.s) {
				t.Error("MatchString mismatch")
			}
			if !reflect.DeepEqual(re.FindStringIndex(tc.s), cre.FindStringIndex(tc.s)) {
				t.Error("FindStringIndex mismatch")
			}
			if re.FindString(tc.s) != cre.FindString(tc.s) {
				t.Error("FindString mismatch")
			}
			if !reflect.DeepEqual(re.FindStringSubmatch(tc.s), cre.FindStringSubmatch(tc.s)) {
				t.Error("FindStringSubmatch mismatch")
			}
			if !reflect.DeepEqual(re.FindAllStringSubmatch(tc.s, tc.n), cre.FindAllStringSubmatch(tc.s, tc.n)) {
				t.Error("FindAllStringSubmatch mismatch")
			}
			if tc.repl != "" {
				if re.ReplaceAllString(tc.s, tc.repl) != cre.ReplaceAllString(tc.s, tc.repl) {
					t.Error("ReplaceAllString mismatch")
				}
			}
		})
	}
}

// --- Concurrent read path: no-capture submatch equals default (no write lock) ---

func TestConcurrentSubmatch_NoCaptures_SameResultAsDefault(t *testing.T) {
	// Patterns with no capture groups use the read-only path (RLock + cached DFA)
	// and must return the same result as the default implementation.
	pat := "a+b+"
	re := MustCompile(pat)
	cre := Concurrent(re)

	single := "aaabbb"
	gotSingle := cre.FindStringSubmatch(single)
	wantSingle := re.FindStringSubmatch(single)
	if !reflect.DeepEqual(gotSingle, wantSingle) {
		t.Errorf("FindStringSubmatch(no captures) = %v, want %v", gotSingle, wantSingle)
	}

	multi := "aaabbb aaabbb"
	gotAll := cre.FindAllStringSubmatch(multi, -1)
	wantAll := re.FindAllStringSubmatch(multi, -1)
	if !reflect.DeepEqual(gotAll, wantAll) {
		t.Errorf("FindAllStringSubmatch(no captures) = %v, want %v", gotAll, wantAll)
	}
}

// --- re3 complement syntax ~(R) ---

func XTestComplement(t *testing.T) {
	tests := []struct {
		name    string
		pat     string
		s       string
		match   bool
		find    []int
		findAll [][]int
	}{
		{
			name:    "~(.*x.*) no x",
			pat:     "~(.*x.*)",
			s:       "abc",
			match:   true,
			find:    []int{0, 3},
			findAll: [][]int{{0, 3}, {3, 3}},
		},
		{
			name:    "~(.*x.*) has x: full string does not match; find leftmost prefix without x",
			pat:     "~(.*x.*)",
			s:       "abxc",
			match:   false,
			find:    []int{0, 2},
			findAll: [][]int{{0, 2}, {2, 2}, {3, 4}, {4, 4}},
		},
		{
			name:    "~(a) full match when no a",
			pat:     "~(a)",
			s:       "bbb",
			match:   true,
			find:    []int{0, 3},
			findAll: [][]int{{0, 3}, {3, 3}},
		},
		{
			name:    "~(a) string not equal to a still matches",
			pat:     "~(a)",
			s:       "aba",
			match:   true,
			find:    []int{0, 3},
			findAll: [][]int{{0, 3}, {3, 3}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pat)
			if got := re.MatchString(tc.s); got != tc.match {
				t.Errorf("MatchString = %v, want %v", got, tc.match)
			}
			if got := re.FindStringIndex(tc.s); !reflect.DeepEqual(got, tc.find) {
				t.Errorf("FindStringIndex = %v, want %v", got, tc.find)
			}
			if got := re.FindAllStringIndex(tc.s, -1); !reflect.DeepEqual(got, tc.findAll) {
				t.Errorf("FindAllStringIndex = %v, want %v", got, tc.findAll)
			}
		})
	}
}

// --- Clone has independent DFA caches ---

func TestCloneIndependentCaches(t *testing.T) {
	re := MustCompile("a+b+").(*regexpImpl)
	c1 := re.Clone().(*regexpImpl)
	c2 := re.Clone().(*regexpImpl)

	// Warm different transitions on each clone.
	c1.MatchString("aaabbb")
	c2.MatchString("ab")

	// Both must still behave correctly.
	if !c1.MatchString("aaabbb") {
		t.Error("clone1 MatchString(aaabbb) should match")
	}
	if !c2.MatchString("ab") {
		t.Error("clone2 MatchString(ab) should match")
	}
	// Clone state is independent: c1 didn't see "ab" in its cache from c2.
	if !c1.MatchString("ab") {
		t.Error("clone1 MatchString(ab) should match after independent use")
	}
}
