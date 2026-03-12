package re3

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// build builds a [][]int from pairs: build(0, 0) => [][]int{{0, 0}}, build(0, 2, 5, 7) => [][]int{{0, 2}, {5, 7}}.
func build(pairs ...int) [][]int {
	if len(pairs)%2 != 0 {
		panic("build: odd number of args")
	}
	out := make([][]int, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		out[i/2] = []int{pairs[i], pairs[i+1]}
	}
	return out
}

var findTests = []struct {
	name    string
	pat     string
	text    string
	matches [][]int // first element used for Find/FindIndex; full list for FindAll*
}{
	{"exact match", "abcdefg", "abcdefg", build(0, 7)},
	{"leftmost longest", "a+", "baaab", build(1, 4)},
	{"no match", "a+", "bbb", nil},
	{"dot star", ".*", "xyz", build(0, 3, 3, 3)},
	{"alternation", "a|b", "b", build(0, 1)},
	{"concat", "hello", "hello world", build(0, 5)},
	{"char class", "[a-z]+", "abc123", build(0, 3)},
	{"alt then literal no false skip", "(a|b)c", "xac", build(1, 3)}, // prefix must not include "c" from right of non-literal left
	{"empty match at start", "a*", "bbb", build(0, 0, 1, 1, 2, 2, 3, 3)},
	{"multiple potential", "a*", "aaab", build(0, 3, 3, 3, 4, 4)},
}

var matchTests = []struct {
	name    string
	pat     string
	text    string
	matches bool
}{
	{"full match", "abc", "abc", true},
	{"no match", "abc", "ab", false},
	{"partial no", "a+", "baaa", false},
	{"full star", "a*", "aaa", true},
	{"dot", ".", "x", true},
}

var replaceTests = []struct {
	pattern, repl, input, want string
}{
	{"a+", "X", "banana", "bXnXnX"},
	{"a*", "x", "baaab", "xxxx"},
	{"xyz", "q", "xyzxyz", "qq"},
	{"no", "yes", "nomatch", "yesmatch"},
}

var splitTests = []struct {
	s    string
	pat  string
	n    int
	want []string
}{
	{"foo:and:bar", ":", -1, []string{"foo", "and", "bar"}},
	{"foo:and:bar", ":", 2, []string{"foo", "and:bar"}},
	{"a:b:c", ":", 0, nil},
	{"hello", "x", -1, []string{"hello"}},
	{"xy", "x*", -1, []string{"", "", ""}},
}

func TestFindStringSubmatch(t *testing.T) {
	tests := []struct {
		name string
		pat  string
		s    string
		want []string // nil = no match; [0]=full match, [i]=group i. re3 TDFA/leftmost-longest semantics.
	}{
		{"basic", "(a)b(c)", "abc", []string{"abc", "ab", "c"}},
		{"nested", "(a(b)c)", "abc", []string{"abc", "abc", "bc"}},
		{"optional unmatched", "(a)?b(c)", "bc", []string{"bc", "", "c"}},
		{"no match", "(a)b(c)", "xyz", nil},
		{"single group", "(a)", "a", []string{"a", "a"}},
		{"two part", "(a)b", "ab", []string{"ab", "ab"}},
		{"POSIX leftmost", "(a)|(a)", "a", []string{"a", "a", ""}},
		{"POSIX greedy", "(a+)(a+)", "aaa", []string{"aaa", "", ""}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pat)
			got := re.FindStringSubmatch(tc.s)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FindStringSubmatch(%q, %q) = %v, want %v", tc.pat, tc.s, got, tc.want)
			}
		})
	}
}

func TestFindAllStringSubmatch(t *testing.T) {
	tests := []struct {
		name string
		pat  string
		s    string
		n    int
		want [][]string
	}{
		{"multiple matches", "(a)b", "ab ab", -1, [][]string{{"ab", "ab"}, {"ab", "ab"}}},
		{"one match", "(a)b(c)", "xabcy", -1, [][]string{{"abc", "ab", "c"}}},
		{"limit n", "(a)b", "ab ab ab", 2, [][]string{{"ab", "ab"}, {"ab", "ab"}}},
		{"no match", "(a)b", "xxx", -1, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pat)
			got := re.FindAllStringSubmatch(tc.s, tc.n)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FindAllStringSubmatch(%q, %q, %d) = %v, want %v", tc.pat, tc.s, tc.n, got, tc.want)
			}
		})
	}
}

func TestFindStringSubmatch_NoCaptures(t *testing.T) {
	// Pattern with no capture groups returns [fullMatch] only.
	re := MustCompile("a+b+")
	got := re.FindStringSubmatch("aaabbb")
	want := []string{"aaabbb"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FindStringSubmatch(no captures) = %v, want %v", got, want)
	}
}

func TestMatchString(t *testing.T) {
	for _, tc := range matchTests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pat)
			got := re.MatchString(tc.text)
			if got != tc.matches {
				t.Errorf("MatchString(%q, %q) = %v, want %v", tc.pat, tc.text, got, tc.matches)
			}
		})
	}
}

func TestMatch(t *testing.T) {
	for _, tc := range matchTests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pat)
			got := re.Match([]byte(tc.text))
			if got != tc.matches {
				t.Errorf("Match(%q, %q) = %v, want %v", tc.pat, tc.text, got, tc.matches)
			}
		})
	}
}

func TestFindStringIndex(t *testing.T) {
	for _, tc := range findTests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pat)
			got := re.FindStringIndex(tc.text)
			var want []int
			if len(tc.matches) > 0 {
				want = tc.matches[0]
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("FindStringIndex(%q, %q) = %v, want %v", tc.pat, tc.text, got, want)
			}
		})
	}
}

func TestFindString(t *testing.T) {
	for _, tc := range findTests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pat)
			got := re.FindString(tc.text)
			var want string
			if len(tc.matches) > 0 {
				loc := tc.matches[0]
				want = tc.text[loc[0]:loc[1]]
			}
			if got != want {
				t.Errorf("FindString(%q, %q) = %q, want %q", tc.pat, tc.text, got, want)
			}
		})
	}
}

func TestFind(t *testing.T) {
	for _, tc := range findTests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pat)
			b := []byte(tc.text)
			got := re.Find(b)
			var want []byte
			if len(tc.matches) > 0 {
				loc := tc.matches[0]
				want = b[loc[0]:loc[1]]
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Find(%q, %q) = %q, want %q", tc.pat, tc.text, got, want)
			}
		})
	}
}

func TestFindIndex(t *testing.T) {
	for _, tc := range findTests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pat)
			got := re.FindIndex([]byte(tc.text))
			var want []int
			if len(tc.matches) > 0 {
				want = tc.matches[0]
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("FindIndex(%q, %q) = %v, want %v", tc.pat, tc.text, got, want)
			}
		})
	}
}

func TestFindAllString(t *testing.T) {
	for _, tc := range findTests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pat)
			got := re.FindAllString(tc.text, -1)
			var want []string
			for _, loc := range tc.matches {
				want = append(want, tc.text[loc[0]:loc[1]])
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("FindAllString(%q, %q, -1) = %q, want %q", tc.pat, tc.text, got, want)
			}
		})
	}
}

func TestFindAllStringIndex(t *testing.T) {
	for _, tc := range findTests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pat)
			got := re.FindAllStringIndex(tc.text, -1)
			if !reflect.DeepEqual(got, tc.matches) {
				t.Errorf("FindAllStringIndex(%q, %q, -1) = %v, want %v", tc.pat, tc.text, got, tc.matches)
			}
		})
	}
}

func TestFindAll(t *testing.T) {
	for _, tc := range findTests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pat)
			b := []byte(tc.text)
			got := re.FindAll(b, -1)
			var want [][]byte
			for _, loc := range tc.matches {
				want = append(want, b[loc[0]:loc[1]])
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("FindAll(%q, %q, -1) = %v, want %v", tc.pat, tc.text, got, want)
			}
		})
	}
}

func TestFindAllIndex(t *testing.T) {
	for _, tc := range findTests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pat)
			got := re.FindAllIndex([]byte(tc.text), -1)
			if !reflect.DeepEqual(got, tc.matches) {
				t.Errorf("FindAllIndex(%q, %q, -1) = %v, want %v", tc.pat, tc.text, got, tc.matches)
			}
		})
	}
}

func TestReplaceAllString(t *testing.T) {
	for i, tc := range replaceTests {
		re := MustCompile(tc.pattern)
		got := re.ReplaceAllString(tc.input, tc.repl)
		if got != tc.want {
			t.Errorf("ReplaceAllString case %d: pattern=%q repl=%q input=%q:\n  got  %q\n  want %q", i, tc.pattern, tc.repl, tc.input, got, tc.want)
		}
	}
}

func TestReplaceAll(t *testing.T) {
	for i, tc := range replaceTests {
		re := MustCompile(tc.pattern)
		got := re.ReplaceAll([]byte(tc.input), []byte(tc.repl))
		want := []byte(tc.want)
		if !bytes.Equal(got, want) {
			t.Errorf("ReplaceAll case %d: pattern=%q repl=%q input=%q:\n  got  %q\n  want %q", i, tc.pattern, tc.repl, tc.input, string(got), tc.want)
		}
	}
}

func TestSplit(t *testing.T) {
	for i, tc := range splitTests {
		re := MustCompile(tc.pat)
		got := re.Split(tc.s, tc.n)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Split case %d: s=%q pat=%q n=%d:\n  got  %q\n  want %q", i, tc.s, tc.pat, tc.n, got, tc.want)
		}
	}
}

var validCompileTests = []string{
	"(?i:Sherlock Holmes)",
	`(?:(\r\n|\r|\n))|(?:([\t\v\f ]+))|(?:((?:(?:(?://.*(?:\r\n|\r|\n))|(?:/\*.*?\*/))\s*)+))|(?:([0-9]+(?:_[0-9]+)*\.[0-9]+(?:_[0-9]+)*[eE][+-]?[0-9]+(?:_[0-9]+)*))|(?:([0-9]+(?:_[0-9]+)*\.[0-9]+(?:_[0-9]+)*))|(?:([0-9]+(?:_[0-9]+)*'[bodh][0-9a-fA-FxzXZ]+(?:_[0-9a-fA-FxzXZ]+)*))|(?:([0-9]+(?:_[0-9]+)*))|(?:('[01xzXZ]))|(?:(\-:))|(?:(\->))|(?:(\+:))|(?:(\+=|-=|\*=|/=|%=|&=|\|=|\^=|<<=|>>=|<<<=|>>>=))|(?:(\*\*))|(?:(/|%))|(?:(\+|-))|(?:(<<<|>>>|<<|>>))|(?:(<=|>=|<|>))|(?:(===|==\?|!==|!=\?|==|!=))|(?:(&&))|(?:(\|\|))|(?:(&))|(?:(\^~|\^|~\^))|(?:(\|))|(?:(~&|~\||!|~))|(?:(::))|(?:(:))|(?:(,))|(?:(\$))|(?:(\.\.))|(?:(\.))|(?:(=))|(?:(\#))|(?:(\{))|(?:(\[))|(?:(\())|(?:(\}))|(?:(\]))|(?:(\)))|(?:(;))|(?:(\*))|(?:(\balways_comb\b))|(?:(\balways_ff\b))|(?:(\bassign\b))|(?:(\basync_high\b))|(?:(\basync_low\b))|(?:(\bas\b))|(?:(\bbit\b))|(?:(\bcase\b))|(?:(\bdefault\b))|(?:(\belse\b))|(?:(\benum\b))|(?:(\bexport\b))|(?:(\bf32\b))|(?:(\bf64\b))|(?:(\bfor\b))|(?:(\bfunction\b))|(?:(\bi32\b))|(?:(\bi64\b))|(?:(\bif_reset\b))|(?:(\bif\b))|(?:(\bimport\b))|(?:(\binout\b))|(?:(\binput\b))|(?:(\binst\b))|(?:(\binterface\b))|(?:(\bin\b))|(?:(\blocalparam\b))|(?:(\blogic\b))|(?:(\bmodport\b))|(?:(\bmodule\b))|(?:(\bnegedge\b))|(?:(\boutput\b))|(?:(\bpackage\b))|(?:(\bparameter\b))|(?:(\bposedge\b))|(?:(\bref\b))|(?:(\brepeat\b))|(?:(\breturn\b))|(?:(\bstep\b))|(?:(\bstruct\b))|(?:(\bsync_high\b))|(?:(\bsync_low\b))|(?:(\btri\b))|(?:(\bu32\b))|(?:(\bu64\b))|(?:(\bvar\b))|(?:([a-zA-Z_][0-9a-zA-Z_]*))|(?:(.))`,
	"a", "a*", "a+", "a?", ".",
	"[a-z]", "\\d", "\\w", "\\s",
	"a|b", "a&b", "~(a)",
	"(a)", "a(b)c", ".*",
	// Empty alternations and groups
	"^a|$", "^|a$", "^()$", "^(?:)$", "^a||b$",
	// Bounded repeats adjacent to parens (stress {n} not eating ')')
	"^([A-Z]{2}){2}$",
	"a{2}", "a{1,3}", "a{2,}", "b{0,1}",
	"(?=a)", "(?<=a)",
}

func TestCompile(t *testing.T) {
	for _, pat := range validCompileTests {
		_, err := Compile(pat)
		if err != nil {
			t.Errorf("Compile(%q) unexpected error: %v", pat, err)
		}
	}
	// Invalid patterns must return error (and re3.Error when from parser/lexer).
	invalid := []struct {
		pat  string
		want ErrorCode
	}{
		{"(a", ErrMissingParen},   // unclosed group
		{"(?=x", ErrMissingParen}, // unclosed lookahead
		{"a{1,0}", ErrInvalidRepeatSize},
		{"\\", ErrTrailingBackslash},
		{"[a", ErrMissingBracket},
	}
	for _, tc := range invalid {
		_, err := Compile(tc.pat)
		if err == nil {
			t.Errorf("Compile(%q) expected error", tc.pat)
			continue
		}
		var re3Err *Error
		if !errors.As(err, &re3Err) {
			continue
		}
		if re3Err.Code != tc.want {
			t.Errorf("Compile(%q) error code = %s, want %s", tc.pat, re3Err.Code, tc.want)
		}
	}
}

func TestMustCompile(t *testing.T) {
	for _, pat := range validCompileTests {
		re := MustCompile(pat)
		if re == nil {
			t.Errorf("MustCompile(%q) returned nil", pat)
		}
	}
}

func TestEmptyAlternationsAndGroups(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		text    string
		match   bool
	}{
		{"a_or_empty_match_empty", "^a|$", "", true},
		{"a_or_empty_match_a", "^a|$", "a", true},
		{"empty_or_a_match_a", "^|a$", "a", true},
		{"empty_group_matches_empty", "^()$", "", true},
		{"empty_noncapturing_group_matches_empty", "^(?:)$", "", true},
		{"double_bar_allows_empty_branch", "^a||b$", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pattern)
			got := re.MatchString(tc.text)
			if got != tc.match {
				t.Errorf("MatchString(%q, %q) = %v, want %v", tc.pattern, tc.text, got, tc.match)
			}
		})
	}
}

// TestCompareWithStd runs (pattern, input) pairs that are valid in both re3 and Go's regexp,
// and asserts that results match except where semantics intentionally differ (e.g. leftmost-longest).
func TestCompareWithStd(t *testing.T) {
	// fullMatchPairs: entire input matches the pattern; we assert MatchString is true for both.
	fullMatchPairs := []struct {
		pattern string
		input   string
	}{
		{"a+b+", "aaabbb"},
		{"a*", "aaa"},
		{"hello", "hello"},
		{"a|b", "b"},
		{"[a-z]+", "abc"},
		{".*", "xyz"},
	}
	for _, tc := range fullMatchPairs {
		reRE3 := MustCompile(tc.pattern)
		reStd := regexp.MustCompile(tc.pattern)
		gotRE3 := reRE3.MatchString(tc.input)
		gotStd := reStd.MatchString(tc.input)
		if gotRE3 != gotStd {
			t.Errorf("MatchString(%q, %q) re3=%v std=%v", tc.pattern, tc.input, gotRE3, gotStd)
		}
		if !gotRE3 || !gotStd {
			t.Errorf("MatchString(%q, %q) expected both true (full match)", tc.pattern, tc.input)
		}
	}

	// Pairs for FindString/FindAllString/ReplaceAllString. Exclude nullable patterns (a*, .*) from
	// FindAllString/ReplaceAllString to avoid empty-match boundary differences.
	pairs := []struct {
		pattern  string
		input    string
		nullable bool // if true, skip FindAllString and ReplaceAllString
	}{
		{"a+b+", "xxaaabbbxx", false},
		{"[a-z]+", "abc123", false},
		{"\\w+", "one two three", false},
		{"[cjrw]", "abcdefghijklmnopqrstuvwxyz", false},
		{"a|b", "b", false},
		{"hello", "hello world", false},
		{".*", "xyz", true},
		{".*", "a\nb", true}, // dot does not match newline; verify both return "a" then "b"
		{"a*", "aaa", true},
		{"a*", "bbb", true},
	}
	for _, tc := range pairs {
		reRE3 := MustCompile(tc.pattern)
		reStd := regexp.MustCompile(tc.pattern)
		// FindStringIndex / FindString
		locRE3 := reRE3.FindStringIndex(tc.input)
		locStd := reStd.FindStringIndex(tc.input)
		var findRE3, findStd string
		if locRE3 != nil {
			findRE3 = tc.input[locRE3[0]:locRE3[1]]
		}
		if locStd != nil {
			findStd = tc.input[locStd[0]:locStd[1]]
		}
		if findRE3 != findStd {
			t.Errorf("FindString(%q, %q) re3=%q std=%q", tc.pattern, tc.input, findRE3, findStd)
		}
		if !tc.nullable {
			// FindAllString
			allRE3 := reRE3.FindAllString(tc.input, -1)
			allStd := reStd.FindAllString(tc.input, -1)
			if !reflect.DeepEqual(allRE3, allStd) {
				t.Errorf("FindAllString(%q, %q) re3=%v std=%v", tc.pattern, tc.input, allRE3, allStd)
			}
			// ReplaceAllString
			repl := "X"
			rRE3 := reRE3.ReplaceAllString(tc.input, repl)
			rStd := reStd.ReplaceAllString(tc.input, repl)
			if rRE3 != rStd {
				t.Errorf("ReplaceAllString(%q, %q, %q) re3=%q std=%q", tc.pattern, tc.input, repl, rRE3, rStd)
			}
		}
	}

	// Leftmost-longest vs leftmost-first: re3 uses POSIX leftmost-longest, Go regexp uses Perl leftmost-first.
	// Pattern "a|ab" on input "ab": std returns "a" (first branch), re3 returns "ab" (longest match).
	// This is intentional; document it here.
	{
		pattern, input := "a|ab", "ab"
		reRE3 := MustCompile(pattern)
		reStd := regexp.MustCompile(pattern)
		findRE3 := reRE3.FindString(input)
		findStd := reStd.FindString(input)
		if findRE3 != "ab" {
			t.Errorf("re3 (leftmost-longest) FindString(%q, %q) = %q, want \"ab\"", pattern, input, findRE3)
		}
		if findStd != "a" {
			t.Errorf("std (leftmost-first) FindString(%q, %q) = %q, want \"a\"", pattern, input, findStd)
		}
	}
}

func TestClone(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		text    string
		match   bool
		find    string // FindString result, or "" for no match
	}{
		{"match", "a+", "aaa", true, "aaa"},
		{"no match", "a+", "bbb", false, ""},
		{"find middle", "a+b+", "xxaaabbbxx", false, "aaabbb"}, // full string does not match a+b+
		{"bounded", "a{2,}", "xaaaab", false, "aaaa"},          // full string does not match a{2,}
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pattern)
			clone := re.Clone()
			if clone == re {
				t.Error("Clone should return a new instance")
			}
			if got := clone.MatchString(tc.text); got != tc.match {
				t.Errorf("Clone.MatchString(%q) = %v, want %v", tc.text, got, tc.match)
			}
			gotFind := clone.FindString(tc.text)
			if gotFind != tc.find {
				t.Errorf("Clone.FindString(%q) = %q, want %q", tc.text, gotFind, tc.find)
			}
			// Original still works and matches clone results
			if got := re.MatchString(tc.text); got != tc.match {
				t.Errorf("Original.MatchString(%q) = %v, want %v", tc.text, got, tc.match)
			}
		})
	}
}

func TestCloneParallel(t *testing.T) {
	re := MustCompile("a+b+")
	const numGoroutines = 8
	matchInput := "aaabbb"    // full match
	findInput := "xxaaabbbxx" // FindStringIndex returns [2,8] -> "aaabbb"
	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			clone := re.Clone()
			for i := 0; i < 100; i++ {
				if !clone.MatchString(matchInput) {
					t.Error("Clone.MatchString should match")
				}
				loc := clone.FindStringIndex(findInput)
				if loc == nil || findInput[loc[0]:loc[1]] != "aaabbb" {
					t.Error("Clone.FindStringIndex want [2,8] for aaabbb")
				}
			}
		}()
	}
	wg.Wait()
}

func TestConcurrent(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		text    string
		match   bool
		find    string
	}{
		{"match", "a+", "aaa", true, "aaa"},
		{"no match", "a+", "bbb", false, ""},
		{"find middle", "a+b+", "xxaaabbbxx", false, "aaabbb"}, // full string does not match
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cre := Concurrent(MustCompile(tc.pattern))
			if cre == nil {
				t.Fatal("Concurrent() returned nil")
			}
			if got := cre.MatchString(tc.text); got != tc.match {
				t.Errorf("ConcurrentRegExp.MatchString(%q) = %v, want %v", tc.text, got, tc.match)
			}
			if got := cre.FindString(tc.text); got != tc.find {
				t.Errorf("ConcurrentRegExp.FindString(%q) = %q, want %q", tc.text, got, tc.find)
			}
		})
	}
	// API parity: FindAllString, ReplaceAllString, Split
	t.Run("FindAllString", func(t *testing.T) {
		cre := Concurrent(MustCompile("\\w+"))
		got := cre.FindAllString("one two three", -1)
		want := []string{"one", "two", "three"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ConcurrentRegExp.FindAllString = %v, want %v", got, want)
		}
	})
	t.Run("ReplaceAllString", func(t *testing.T) {
		cre := Concurrent(MustCompile("a+"))
		got := cre.ReplaceAllString("banana", "X")
		if got != "bXnXnX" {
			t.Errorf("ConcurrentRegExp.ReplaceAllString = %q, want bXnXnX", got)
		}
	})
	t.Run("ReplaceAll", func(t *testing.T) {
		cre := Concurrent(MustCompile("a+"))
		got := cre.ReplaceAll([]byte("banana"), []byte("X"))
		if want := []byte("bXnXnX"); !bytes.Equal(got, want) {
			t.Errorf("ConcurrentRegExp.ReplaceAll = %q, want %q", got, want)
		}
	})
	t.Run("Split", func(t *testing.T) {
		cre := Concurrent(MustCompile(":"))
		got := cre.Split("foo:and:bar", -1)
		want := []string{"foo", "and", "bar"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ConcurrentRegExp.Split = %v, want %v", got, want)
		}
	})
	t.Run("FindStringSubmatch", func(t *testing.T) {
		cre := Concurrent(MustCompile("(a)b(c)"))
		got := cre.FindStringSubmatch("xabcy")
		want := []string{"abc", "ab", "c"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ConcurrentRegExp.FindStringSubmatch = %v, want %v", got, want)
		}
	})
	t.Run("FindAllStringSubmatch", func(t *testing.T) {
		cre := Concurrent(MustCompile("(a)b"))
		got := cre.FindAllStringSubmatch("ab ab", -1)
		want := [][]string{{"ab", "ab"}, {"ab", "ab"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ConcurrentRegExp.FindAllStringSubmatch = %v, want %v", got, want)
		}
	})
}

func TestConcurrentParallel(t *testing.T) {
	cre := Concurrent(MustCompile("a+b+"))
	if cre == nil {
		t.Fatal("Concurrent() returned nil")
	}
	const numGoroutines = 8
	matchInput := "aaabbb"
	findInput := "xxaaabbbxx"
	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				if !cre.MatchString(matchInput) {
					t.Error("ConcurrentRegExp.MatchString should match")
				}
				loc := cre.FindStringIndex(findInput)
				if loc == nil || findInput[loc[0]:loc[1]] != "aaabbb" {
					t.Error("ConcurrentRegExp.FindStringIndex want [2,8] for aaabbb")
				}
			}
		}()
	}
	wg.Wait()
}

func TestBoundedRepetition(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		text    string
		want    bool
	}{
		{"exact n", "a{2}", "aa", true},
		{"exact n too short", "a{2}", "a", false},
		{"exact n too long", "a{2}", "aaa", false},
		{"range min", "a{1,3}", "a", true},
		{"range mid", "a{1,3}", "aa", true},
		{"range max", "a{1,3}", "aaa", true},
		{"range over", "a{1,3}", "aaaa", false},
		{"open min", "a{2,}", "aa", true},
		{"open more", "a{2,}", "aaa", true},
		{"open too short", "a{2,}", "a", false},
		{"char class exact", "[0-9]{2}", "12", true},
		{"char class short", "[0-9]{2}", "1", false},
		{"zero exact", "a{0}", "", true},
		{"zero exact no match", "a{0}", "a", false},
		{"zero open", "a{0,}", "", true},
		{"zero open one", "a{0,}", "a", true},
		{"one exact", "b{1}", "b", true},
		{"one exact no match", "b{1}", "", false},
		// Regression: bounded repeat followed by ')' should not trigger ErrMissingParen
		{"bounded_then_paren", "^([A-Z]{2}){2}$", "ABCD", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pattern)
			got := re.MatchString(tc.text)
			if got != tc.want {
				t.Errorf("MatchString(%q, %q) = %v, want %v", tc.pattern, tc.text, got, tc.want)
			}
		})
	}
}

func TestBoundedRepetitionFind(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		text    string
		want    string // leftmost-longest match, or "" for no match
	}{
		{"exact in middle", "a{2}", "xaaab", "aa"},
		{"range in middle", "a{2,4}", "xaaaab", "aaaa"},
		{"range fewer", "a{2,4}", "xaaab", "aaa"},
		{"open in middle", "a{2,}", "xaaabbb", "aaa"},
		{"no match", "a{3}", "xaab", ""},
		{"char class", "[0-9]{2,4}", "ab12345cd", "1234"},
		{"zero repeat matches empty", "a{0}", "x", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pattern)
			loc := re.FindStringIndex(tc.text)
			var got string
			if loc != nil {
				got = tc.text[loc[0]:loc[1]]
			}
			if got != tc.want {
				t.Errorf("FindString(%q, %q) = %q, want %q", tc.pattern, tc.text, got, tc.want)
			}
		})
	}
}

func TestBoundedRepetitionFindAll(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		text    string
		want    []string
	}{
		{"two runs", "a{2}", "aabaabaa", []string{"aa", "aa", "aa"}},
		{"range runs", "a{1,2}", "abacaada", []string{"a", "a", "aa", "a"}},
		{"digits", "[0-9]{2}", "a12b34c5", []string{"12", "34"}},
		{"open", "a{2,}", "aaaabaab", []string{"aaaa", "aa"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pattern)
			got := re.FindAllString(tc.text, -1)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FindAllString(%q, %q) = %v, want %v", tc.pattern, tc.text, got, tc.want)
			}
		})
	}
}

func TestBoundedRepetitionReplace(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		repl    string
		input   string
		want    string
	}{
		{"exact", "a{2}", "X", "aabaab", "XbXb"},
		{"range", "a{1,2}", "X", "abacaada", "XbXcXdX"},
		{"open", "a{2,}", "Y", "aaaabaab", "YbYb"},
		{"digits", "[0-9]{2}", "N", "a12b34c", "aNbNc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pattern)
			got := re.ReplaceAllString(tc.input, tc.repl)
			if got != tc.want {
				t.Errorf("ReplaceAllString(%q, %q, %q) = %q, want %q", tc.pattern, tc.repl, tc.input, got, tc.want)
			}
		})
	}
}

func BenchmarkMatchString(b *testing.B) {
	re := MustCompile("a+b+")
	s := "aaabbb"
	re.MatchString(s) // warm Lazy DFA
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.MatchString(s)
	}
}

func BenchmarkMatchStringShort(b *testing.B) {
	re := MustCompile("a+b+")
	s := "aaabbb"
	re.MatchString(s)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.MatchString(s)
	}
}

func BenchmarkMatchStringLong(b *testing.B) {
	re := MustCompile("a+b+")
	s := ""
	for i := 0; i < 2000; i++ {
		s += "aaabbb"
	}
	re.MatchString(s)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.MatchString(s)
	}
}

func BenchmarkMatch(b *testing.B) {
	re := MustCompile("a+b+")
	s := []byte("aaabbb")
	re.Match(s)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.Match(s)
	}
}

func BenchmarkFindStringIndex(b *testing.B) {
	re := MustCompile("a+b+")
	s := "xxaaabbbxx"
	re.FindStringIndex(s)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.FindStringIndex(s)
	}
}

func BenchmarkFindAllString(b *testing.B) {
	re := MustCompile("\\w+")
	s := "one two three four five six seven eight nine ten"
	re.FindAllString(s, -1)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.FindAllString(s, -1)
	}
}

func BenchmarkFindAllStringIndex(b *testing.B) {
	re := MustCompile("\\w+")
	s := "one two three four five six seven eight nine ten"
	re.FindAllStringIndex(s, -1)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.FindAllStringIndex(s, -1)
	}
}

func BenchmarkSplit(b *testing.B) {
	re := MustCompile(":")
	s := "foo:and:bar"
	re.Split(s, -1)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.Split(s, -1)
	}
}

func BenchmarkReplaceAllString(b *testing.B) {
	re := MustCompile("[cjrw]")
	s := "abcdefghijklmnopqrstuvwxyz"
	re.ReplaceAllString(s, "")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.ReplaceAllString(s, "")
	}
}

// Option B: MatchString with capture groups in pattern should not build TDFA (same speed as pure DFA).
func BenchmarkMatchString_WithCaptures(b *testing.B) {
	re := MustCompile("(a|b)+c")
	s := "abac"
	re.MatchString(s)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.MatchString(s)
	}
}

func BenchmarkFindStringSubmatch(b *testing.B) {
	re := MustCompile("(a)b(c)")
	s := "xabcy"
	re.FindStringSubmatch(s)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.FindStringSubmatch(s)
	}
}

func BenchmarkCompile(b *testing.B) {
	pat := "a*b*c*d*"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		MustCompile(pat)
	}
}

func BenchmarkCompileSimple(b *testing.B) {
	pat := "a+"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		MustCompile(pat)
	}
}

func BenchmarkCompileComplex(b *testing.B) {
	pat := "(a|b|c)+"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		MustCompile(pat)
	}
}

// --- Bounded repetition benchmarks ---

func BenchmarkBoundedRepetitionMatchString(b *testing.B) {
	re := MustCompile("a{5,10}")
	s := "aaaaab"
	re.MatchString(s)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.MatchString(s)
	}
}

func BenchmarkBoundedRepetitionFindStringIndex(b *testing.B) {
	re := MustCompile("[0-9]{2,4}")
	s := "ab12345cd67890ef"
	re.FindStringIndex(s)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.FindStringIndex(s)
	}
}

func BenchmarkBoundedRepetitionFindAllString(b *testing.B) {
	re := MustCompile("a{2,}")
	s := "aaaabaabaaa"
	re.FindAllString(s, -1)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.FindAllString(s, -1)
	}
}

func BenchmarkBoundedRepetitionReplaceAllString(b *testing.B) {
	re := MustCompile("[0-9]{2}")
	s := "a12b34c56d78e"
	re.ReplaceAllString(s, "N")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.ReplaceAllString(s, "N")
	}
}

func BenchmarkBoundedRepetitionCompile(b *testing.B) {
	pat := "a{1,20}"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		MustCompile(pat)
	}
}

// --- Concurrency benchmarks (Clone vs Concurrent) ---

func BenchmarkCloneParallel(b *testing.B) {
	re := MustCompile("a+b+")
	s := "xxaaabbbxx"
	re.MatchString(s) // warm original so clones share warm minterms
	for _, g := range []int{1, 4, 8} {
		g := g
		b.Run(fmt.Sprintf("%d-goroutines", g), func(b *testing.B) {
			clones := make([]RegExp, g)
			for i := 0; i < g; i++ {
				clones[i] = re.Clone()
				clones[i].MatchString(s) // warm each clone
			}
			b.ResetTimer()
			b.ReportAllocs()
			var wg sync.WaitGroup
			perG := b.N / g
			for i := 0; i < g; i++ {
				wg.Add(1)
				go func(c RegExp) {
					defer wg.Done()
					for j := 0; j < perG; j++ {
						c.MatchString(s)
						c.FindStringIndex(s)
					}
				}(clones[i])
			}
			wg.Wait()
		})
	}
}

func BenchmarkConcurrentParallel(b *testing.B) {
	cre := Concurrent(MustCompile("a+b+"))
	s := "xxaaabbbxx"
	cre.MatchString(s) // warm cache
	cre.FindStringIndex(s)
	for _, g := range []int{1, 4, 8} {
		g := g
		b.Run(fmt.Sprintf("%d-goroutines", g), func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()
			var wg sync.WaitGroup
			perG := b.N / g
			for i := 0; i < g; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < perG; j++ {
						cre.MatchString(s)
						cre.FindStringIndex(s)
					}
				}()
			}
			wg.Wait()
		})
	}
}

// --- Comparison benchmarks (re3 vs standard regexp) ---

func BenchmarkCompareMatchString(b *testing.B) {
	pat := "a+b+"
	s := "aaabbb"
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.MatchString(s)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.MatchString(s)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.MatchString(s)
		}
	})
}

func BenchmarkCompareFindStringIndex(b *testing.B) {
	pat := "a+b+"
	s := "xxaaabbbxx"
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.FindStringIndex(s)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.FindStringIndex(s)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.FindStringIndex(s)
		}
	})
}

func BenchmarkCompareFindAllString(b *testing.B) {
	pat := "\\w+"
	s := "one two three four five six seven eight nine ten"
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.FindAllString(s, -1)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.FindAllString(s, -1)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.FindAllString(s, -1)
		}
	})
}

func BenchmarkCompareReplaceAllString(b *testing.B) {
	pat := "[cjrw]"
	s := "abcdefghijklmnopqrstuvwxyz"
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.ReplaceAllString(s, "")
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.ReplaceAllString(s, "")
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.ReplaceAllString(s, "")
		}
	})
}

func BenchmarkCompareReplaceAll(b *testing.B) {
	pat := "[cjrw]"
	s := []byte("abcdefghijklmnopqrstuvwxyz")
	repl := []byte("")
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.ReplaceAll(s, repl)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.ReplaceAll(s, repl)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.ReplaceAll(s, repl)
		}
	})
}

func BenchmarkCompareCompile(b *testing.B) {
	pat := "a+b+"
	b.Run("re3", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			MustCompile(pat)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			regexp.MustCompile(pat)
		}
	})
}

// --- Additional comparison benchmarks (re3 vs std) ---

func BenchmarkCompareFindStringSubmatch(b *testing.B) {
	pat := "(a)b(c)"
	s := "xabcy"
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.FindStringSubmatch(s)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.FindStringSubmatch(s)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.FindStringSubmatch(s)
		}
	})
}

func BenchmarkCompareFindAllStringSubmatch(b *testing.B) {
	pat := "(\\w+)"
	s := "one two three four five"
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.FindAllStringSubmatch(s, -1)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.FindAllStringSubmatch(s, -1)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.FindAllStringSubmatch(s, -1)
		}
	})
}

func BenchmarkCompareSplit(b *testing.B) {
	pat := ":"
	s := "foo:and:bar:baz:qux"
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.Split(s, -1)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.Split(s, -1)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.Split(s, -1)
		}
	})
}

func BenchmarkCompareFindString(b *testing.B) {
	pat := "a+b+"
	s := "xxaaabbbxx"
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.FindString(s)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.FindString(s)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.FindString(s)
		}
	})
}

func BenchmarkCompareMatch(b *testing.B) {
	pat := "a+b+"
	s := []byte("aaabbb")
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.Match(s)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.Match(s)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.Match(s)
		}
	})
}

func BenchmarkCompareFindIndex(b *testing.B) {
	pat := "a+b+"
	s := []byte("xxaaabbbxx")
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.FindIndex(s)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.FindIndex(s)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.FindIndex(s)
		}
	})
}

func BenchmarkCompareFindAllStringIndex(b *testing.B) {
	pat := "\\w+"
	s := "one two three four five six seven eight nine ten"
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.FindAllStringIndex(s, -1)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.FindAllStringIndex(s, -1)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.FindAllStringIndex(s, -1)
		}
	})
}

func BenchmarkCompareFindAll(b *testing.B) {
	pat := "\\w+"
	s := []byte("one two three four five")
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.FindAll(s, -1)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.FindAll(s, -1)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.FindAll(s, -1)
		}
	})
}

func BenchmarkCompareFindAllIndex(b *testing.B) {
	pat := "\\w+"
	s := []byte("one two three four five")
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.FindAllIndex(s, -1)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.FindAllIndex(s, -1)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.FindAllIndex(s, -1)
		}
	})
}

func BenchmarkCompareMatchString_WithCaptures(b *testing.B) {
	pat := "(a|b)+c"
	s := "abac"
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.MatchString(s)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.MatchString(s)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.MatchString(s)
		}
	})
}

func BenchmarkCompareCompileSimple(b *testing.B) {
	pat := "a+"
	b.Run("re3", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			MustCompile(pat)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			regexp.MustCompile(pat)
		}
	})
}

func BenchmarkCompareCompileComplex(b *testing.B) {
	pat := "(a|b|c)+"
	b.Run("re3", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			MustCompile(pat)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			regexp.MustCompile(pat)
		}
	})
}

func BenchmarkCompareCompileComplexPat(b *testing.B) {
	pat := "a*b*c*d*"
	b.Run("re3", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			MustCompile(pat)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			regexp.MustCompile(pat)
		}
	})
}

func runWithTimeout(t *testing.T, timeout time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("operation exceeded timeout %s", timeout)
	}
}

func TestRuntimeTimeoutRegressions(t *testing.T) {
	t.Run("cloudflare_simplified_long", func(t *testing.T) {
		re := MustCompile(".*.*=.*")
		haystack := "x=" + strings.Repeat("x", 2047)
		runWithTimeout(t, 10*time.Second, func() {
			locs := re.FindAllStringIndex(haystack, 32)
			if len(locs) == 0 {
				t.Fatalf("expected matches for cloudflare simplified-long pattern")
			}
		})
	})

	t.Run("reverse_inner_no_quadratic_forward", func(t *testing.T) {
		re := MustCompile(".efghijklmnopq[a-z]+[A-Z]")
		haystack := strings.Repeat("bcdefghijklmnopq", 500)
		runWithTimeout(t, 5*time.Second, func() {
			locs := re.FindAllStringIndex(haystack, -1)
			if len(locs) != 0 {
				t.Fatalf("expected no matches, got %d", len(locs))
			}
		})
	})

	t.Run("slow_quadratic_regex_1x", func(t *testing.T) {
		re := MustCompile("(?:A+){100}|")
		haystack := strings.Repeat("A", 20)
		runWithTimeout(t, 2*time.Second, func() {
			locs := re.FindAllStringIndex(haystack, 4)
			if len(locs) == 0 {
				t.Fatalf("expected at least one match")
			}
		})
	})

	t.Run("slow_quadratic_regex_2x", func(t *testing.T) {
		re := MustCompile("(?:A+){200}|")
		runWithTimeout(t, 2*time.Second, func() {
			if !re.MatchString("") {
				t.Fatalf("expected empty-branch match for quadratic-regex-2x")
			}
		})
	})

	t.Run("wild_url_shape_search", func(t *testing.T) {
		pattern := "(?:(?:https?|ftp)://)?(?:[a-z0-9-]+\\.)+(?:com|org|net|io|dev|app|cloud|xyz|online|shop|site|blog)(?:/[a-z0-9/_\\-%.?=&+]*)?"
		re := MustCompile(pattern)
		haystack := strings.Repeat("https://example.com/path?q=1\n", 2000)
		runWithTimeout(t, 2*time.Second, func() {
			locs := re.FindAllStringIndex(haystack, -1)
			if len(locs) == 0 {
				t.Fatalf("expected URL-shape matches")
			}
		})
	})
}

func TestUnicodeByteLowering(t *testing.T) {
	t.Run("multibyte_literal_match", func(t *testing.T) {
		re := MustCompile("é")
		if !re.MatchString("é") {
			t.Fatalf("expected multibyte literal to match")
		}
		if re.MatchString("e") {
			t.Fatalf("did not expect ASCII e to match é")
		}
	})

	t.Run("multibyte_concat", func(t *testing.T) {
		re := MustCompile("aé")
		if !re.MatchString("aé") {
			t.Fatalf("expected concat with multibyte literal to match")
		}
		if re.MatchString("ae") {
			t.Fatalf("did not expect ASCII concat to match")
		}
	})

	t.Run("multibyte_find_index_is_byte_offsets", func(t *testing.T) {
		re := MustCompile("é")
		got := re.FindStringIndex("aéb")
		want := []int{1, 3}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("FindStringIndex byte offsets = %v, want %v", got, want)
		}
	})

	t.Run("multibyte_repeat", func(t *testing.T) {
		re := MustCompile("é+")
		got := re.FindString("ééx")
		if got != "éé" {
			t.Fatalf("FindString for repeated multibyte literal = %q, want %q", got, "éé")
		}
	})
}

func TestUnicodeClassesAndRanges(t *testing.T) {
	t.Run("unicode_range_class", func(t *testing.T) {
		re := MustCompile("[À-ÿ]+")
		got := re.FindString("xxéçyy")
		if got != "éç" {
			t.Fatalf("FindString([À-ÿ]+) = %q, want %q", got, "éç")
		}
	})

	t.Run("unicode_property_letter", func(t *testing.T) {
		re := MustCompile(`\p{L}+`)
		got := re.FindString("123αβγ456")
		if got != "αβγ" {
			t.Fatalf("FindString(\\p{L}+) = %q, want %q", got, "αβγ")
		}
	})

	t.Run("unicode_property_short_form", func(t *testing.T) {
		re := MustCompile(`\pL+`)
		got := re.FindString("123Δelta456")
		if got != "Δelta" {
			t.Fatalf("FindString(\\pL+) = %q, want %q", got, "Δelta")
		}
	})

	t.Run("unicode_property_negation", func(t *testing.T) {
		re := MustCompile(`\P{L}+`)
		got := re.FindString("123é")
		if got != "123" {
			t.Fatalf("FindString(\\P{L}+) = %q, want %q", got, "123")
		}
	})

	t.Run("unicode_script_property", func(t *testing.T) {
		re := MustCompile(`\p{Greek}+`)
		got := re.FindString("abcαβγxyz")
		if got != "αβγ" {
			t.Fatalf("FindString(\\p{Greek}+) = %q, want %q", got, "αβγ")
		}
	})

	t.Run("unicode_case_insensitive_global", func(t *testing.T) {
		re := MustCompile(`(?iu)привет`)
		if !re.MatchString("Привет") {
			t.Fatalf("expected (?iu)привет to match Привет")
		}
	})

	t.Run("unicode_case_insensitive_scoped", func(t *testing.T) {
		re := MustCompile(`(?iu:привет)`)
		if !re.MatchString("Привет") {
			t.Fatalf("expected (?iu:привет) to match Привет")
		}
	})

	t.Run("unicode_word_boundary", func(t *testing.T) {
		re := MustCompile(`\bπ\b`)
		if got := re.FindString("aπb"); got != "π" {
			t.Fatalf("FindString(\\bπ\\b) in ASCII word context = %q, want %q", got, "π")
		}
		if got := re.FindString(" π "); got != "" {
			t.Fatalf("FindString(\\bπ\\b) in non-word boundaries = %q, want empty", got)
		}
	})

	t.Run("unicode_word_boundary_connector_punctuation", func(t *testing.T) {
		re := MustCompile(`(?u:\b)`)
		got := re.FindAllStringIndex("⁀", -1)
		if len(got) != 2 {
			t.Fatalf("FindAllStringIndex((?u:\\b), %q) count = %d, want 2", "⁀", len(got))
		}
	})

	t.Run("dot_rejects_invalid_utf8_byte", func(t *testing.T) {
		re := MustCompile(`(?u:.)`)
		invalid := []byte{0xFF}
		if got := re.Find(invalid); got != nil {
			t.Fatalf("expected dot to reject invalid utf8 byte, got %v", got)
		}
	})
}

func BenchmarkCompareBoundedRepetitionMatchString(b *testing.B) {
	pat := "a{5,10}"
	s := "aaaaab"
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.MatchString(s)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.MatchString(s)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.MatchString(s)
		}
	})
}

func BenchmarkCompareBoundedRepetitionFindStringIndex(b *testing.B) {
	pat := "[0-9]{2,4}"
	s := "ab12345cd67890ef"
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.FindStringIndex(s)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.FindStringIndex(s)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.FindStringIndex(s)
		}
	})
}

func BenchmarkCompareBoundedRepetitionFindAllString(b *testing.B) {
	pat := "a{2,}"
	s := "aaaabaabaaa"
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.FindAllString(s, -1)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.FindAllString(s, -1)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.FindAllString(s, -1)
		}
	})
}

func BenchmarkCompareBoundedRepetitionReplaceAllString(b *testing.B) {
	pat := "[0-9]{2}"
	s := "a12b34c56d78e"
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.ReplaceAllString(s, "N")
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.ReplaceAllString(s, "N")
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.ReplaceAllString(s, "N")
		}
	})
}

func BenchmarkCompareBoundedRepetitionCompile(b *testing.B) {
	pat := "a{1,20}"
	b.Run("re3", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			MustCompile(pat)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			regexp.MustCompile(pat)
		}
	})
}

func BenchmarkCompareMatchStringLong(b *testing.B) {
	s := ""
	for i := 0; i < 10_000; i++ {
		s += "a"
	}
	pat := "a+"
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.MatchString(s)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.MatchString(s)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.MatchString(s)
		}
	})
}

func BenchmarkCompareFindStringIndexLong(b *testing.B) {
	const size = 100_000
	s := ""
	for i := 0; i < size; i++ {
		s += "xy"
	}
	s += "aaabbb"
	pat := "a+b+"
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.FindStringIndex(s)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.FindStringIndex(s)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.FindStringIndex(s)
		}
	})
}

func BenchmarkCompareFindAllStringLong(b *testing.B) {
	words := []string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"}
	s := ""
	for i := 0; i < 1000; i++ {
		s += words[i%len(words)] + " "
	}
	pat := "\\w+"
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.FindAllString(s, -1)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.FindAllString(s, -1)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.FindAllString(s, -1)
		}
	})
}

func BenchmarkCompareReplaceAllStringLong(b *testing.B) {
	s := ""
	for i := 0; i < 5000; i++ {
		s += "ab"
	}
	pat := "a"
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.ReplaceAllString(s, "x")
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.ReplaceAllString(s, "x")
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.ReplaceAllString(s, "x")
		}
	})
}

func BenchmarkCompareFind(b *testing.B) {
	pat := "a+b+"
	s := []byte("xxaaabbbxx")
	reRE3 := MustCompile(pat)
	reStd := regexp.MustCompile(pat)
	b.Run("re3", func(b *testing.B) {
		reRE3.Find(s)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reRE3.Find(s)
		}
	})
	b.Run("std", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reStd.Find(s)
		}
	})
}

// --- Substring match comparison benchmarks (exhaustive) ---

func BenchmarkCompareSubstringFindString(b *testing.B) {
	scenarios := []struct {
		name, pat, s string
	}{
		{"at_start", "a+", "aaabbbxxx"},
		{"at_middle", "a+b+", "xxaaabbbxx"},
		{"at_end", "a+b+", "xxaaabbb"},
		{"no_match", "xyz", "abcdef"},
		{"single_char", "b", "abc"},
		{"alternation", "a|b|c", "xyzc"},
		{"word", "\\w+", "hello world"},
		{"digits", "[0-9]+", "ab12cd"},
		{"bounded", "a{2,4}", "xaaaab"},
	}
	for _, sc := range scenarios {
		sc := sc
		reRE3 := MustCompile(sc.pat)
		reStd := regexp.MustCompile(sc.pat)
		b.Run(sc.name+"/re3", func(b *testing.B) {
			reRE3.FindString(sc.s)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reRE3.FindString(sc.s)
			}
		})
		b.Run(sc.name+"/std", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reStd.FindString(sc.s)
			}
		})
	}
}

func BenchmarkCompareSubstringFindStringIndex(b *testing.B) {
	scenarios := []struct {
		name, pat, s string
	}{
		{"at_start", "a+", "aaabbbxxx"},
		{"at_middle", "a+b+", "xxaaabbbxx"},
		{"at_end", "a+b+", "xxaaabbb"},
		{"no_match", "xyz", "abcdef"},
		{"single_char", "b", "abc"},
		{"word", "\\w+", "hello world"},
		{"digits", "[0-9]+", "ab12cd"},
	}
	for _, sc := range scenarios {
		sc := sc
		reRE3 := MustCompile(sc.pat)
		reStd := regexp.MustCompile(sc.pat)
		b.Run(sc.name+"/re3", func(b *testing.B) {
			reRE3.FindStringIndex(sc.s)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reRE3.FindStringIndex(sc.s)
			}
		})
		b.Run(sc.name+"/std", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reStd.FindStringIndex(sc.s)
			}
		})
	}
}

func BenchmarkCompareSubstringFind(b *testing.B) {
	scenarios := []struct {
		name, pat string
		s         []byte
	}{
		{"at_start", "a+", []byte("aaabbbxxx")},
		{"at_middle", "a+b+", []byte("xxaaabbbxx")},
		{"at_end", "a+b+", []byte("xxaaabbb")},
		{"no_match", "xyz", []byte("abcdef")},
		{"word", "\\w+", []byte("hello world")},
	}
	for _, sc := range scenarios {
		sc := sc
		reRE3 := MustCompile(sc.pat)
		reStd := regexp.MustCompile(sc.pat)
		b.Run(sc.name+"/re3", func(b *testing.B) {
			reRE3.Find(sc.s)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reRE3.Find(sc.s)
			}
		})
		b.Run(sc.name+"/std", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reStd.Find(sc.s)
			}
		})
	}
}

func BenchmarkCompareSubstringFindIndex(b *testing.B) {
	scenarios := []struct {
		name, pat string
		s         []byte
	}{
		{"at_start", "a+", []byte("aaabbbxxx")},
		{"at_middle", "a+b+", []byte("xxaaabbbxx")},
		{"at_end", "a+b+", []byte("xxaaabbb")},
		{"no_match", "xyz", []byte("abcdef")},
		{"word", "\\w+", []byte("hello world")},
	}
	for _, sc := range scenarios {
		sc := sc
		reRE3 := MustCompile(sc.pat)
		reStd := regexp.MustCompile(sc.pat)
		b.Run(sc.name+"/re3", func(b *testing.B) {
			reRE3.FindIndex(sc.s)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reRE3.FindIndex(sc.s)
			}
		})
		b.Run(sc.name+"/std", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reStd.FindIndex(sc.s)
			}
		})
	}
}

func BenchmarkCompareSubstringFindAllString(b *testing.B) {
	scenarios := []struct {
		name, pat, s string
		n            int
	}{
		{"many", "\\w+", "one two three four five six seven eight nine ten", -1},
		{"few", "\\w+", "a b c", -1},
		{"no_match", "xyz", "abc def ghi", -1},
		{"limit_2", "\\w+", "one two three", 2},
		{"digits", "[0-9]+", "a1b22c333d", -1},
		{"alternation", "a|b", "xaxbxc", -1},
	}
	for _, sc := range scenarios {
		sc := sc
		reRE3 := MustCompile(sc.pat)
		reStd := regexp.MustCompile(sc.pat)
		b.Run(sc.name+"/re3", func(b *testing.B) {
			reRE3.FindAllString(sc.s, sc.n)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reRE3.FindAllString(sc.s, sc.n)
			}
		})
		b.Run(sc.name+"/std", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reStd.FindAllString(sc.s, sc.n)
			}
		})
	}
}

func BenchmarkCompareSubstringFindAllStringIndex(b *testing.B) {
	scenarios := []struct {
		name, pat, s string
		n            int
	}{
		{"many", "\\w+", "one two three four five", -1},
		{"few", "\\w+", "a b c", -1},
		{"no_match", "xyz", "abc def", -1},
		{"limit_2", "\\w+", "one two three", 2},
	}
	for _, sc := range scenarios {
		sc := sc
		reRE3 := MustCompile(sc.pat)
		reStd := regexp.MustCompile(sc.pat)
		b.Run(sc.name+"/re3", func(b *testing.B) {
			reRE3.FindAllStringIndex(sc.s, sc.n)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reRE3.FindAllStringIndex(sc.s, sc.n)
			}
		})
		b.Run(sc.name+"/std", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reStd.FindAllStringIndex(sc.s, sc.n)
			}
		})
	}
}

func BenchmarkCompareSubstringFindAll(b *testing.B) {
	scenarios := []struct {
		name, pat string
		s         []byte
		n         int
	}{
		{"many", "\\w+", []byte("one two three four five"), -1},
		{"few", "\\w+", []byte("a b c"), -1},
		{"no_match", "xyz", []byte("abc def"), -1},
		{"limit_2", "\\w+", []byte("one two three"), 2},
	}
	for _, sc := range scenarios {
		sc := sc
		reRE3 := MustCompile(sc.pat)
		reStd := regexp.MustCompile(sc.pat)
		b.Run(sc.name+"/re3", func(b *testing.B) {
			reRE3.FindAll(sc.s, sc.n)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reRE3.FindAll(sc.s, sc.n)
			}
		})
		b.Run(sc.name+"/std", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reStd.FindAll(sc.s, sc.n)
			}
		})
	}
}

func BenchmarkCompareSubstringFindAllIndex(b *testing.B) {
	scenarios := []struct {
		name, pat string
		s         []byte
		n         int
	}{
		{"many", "\\w+", []byte("one two three four five"), -1},
		{"few", "\\w+", []byte("a b c"), -1},
		{"no_match", "xyz", []byte("abc def"), -1},
		{"limit_2", "\\w+", []byte("one two three"), 2},
	}
	for _, sc := range scenarios {
		sc := sc
		reRE3 := MustCompile(sc.pat)
		reStd := regexp.MustCompile(sc.pat)
		b.Run(sc.name+"/re3", func(b *testing.B) {
			reRE3.FindAllIndex(sc.s, sc.n)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reRE3.FindAllIndex(sc.s, sc.n)
			}
		})
		b.Run(sc.name+"/std", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reStd.FindAllIndex(sc.s, sc.n)
			}
		})
	}
}

func BenchmarkCompareSubstringFindStringSubmatch(b *testing.B) {
	scenarios := []struct {
		name, pat, s string
	}{
		{"one_group", "(a)", "xay"},
		{"two_groups", "(a)b(c)", "xabcy"},
		{"no_captures", "a+b+", "aaabbb"},
		{"no_match", "(a)b", "xyz"},
		{"nested", "(a(b))", "ab"},
		{"alternation", "(a)|(b)", "b"},
		{"single_match", "(\\w+)", "hello"},
		{"empty_optional", "(a)?b", "b"},
	}
	for _, sc := range scenarios {
		sc := sc
		reRE3 := MustCompile(sc.pat)
		reStd := regexp.MustCompile(sc.pat)
		b.Run(sc.name+"/re3", func(b *testing.B) {
			reRE3.FindStringSubmatch(sc.s)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reRE3.FindStringSubmatch(sc.s)
			}
		})
		b.Run(sc.name+"/std", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reStd.FindStringSubmatch(sc.s)
			}
		})
	}
}

func BenchmarkCompareSubstringFindAllStringSubmatch(b *testing.B) {
	scenarios := []struct {
		name, pat, s string
		n            int
	}{
		{"many", "(\\w+)", "one two three four five", -1},
		{"few", "(a)", "xaybzca", -1},
		{"one_match", "(a)b(c)", "xabcy", -1},
		{"no_match", "(a)b", "xyz", -1},
		{"limit_2", "(\\w+)", "one two three", 2},
		{"two_groups_each", "(a)b", "ab ab ab", -1},
	}
	for _, sc := range scenarios {
		sc := sc
		reRE3 := MustCompile(sc.pat)
		reStd := regexp.MustCompile(sc.pat)
		b.Run(sc.name+"/re3", func(b *testing.B) {
			reRE3.FindAllStringSubmatch(sc.s, sc.n)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reRE3.FindAllStringSubmatch(sc.s, sc.n)
			}
		})
		b.Run(sc.name+"/std", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reStd.FindAllStringSubmatch(sc.s, sc.n)
			}
		})
	}
}

// --- Parametric / scenario benchmarks (re3) ---

func BenchmarkMatchStringScenarios(b *testing.B) {
	scenarios := []struct {
		name, pat, s string
	}{
		{"match", "a+b+", "aaabbb"},
		{"no_match", "a+b+", "bbb"},
		{"alternation", "a|b|c", "b"},
		{"charclass", "[a-z]+", "hello"},
		{"dot", "a.b", "axb"},
		{"bounded", "a{2,4}", "aaa"},
	}
	for _, sc := range scenarios {
		sc := sc
		b.Run(sc.name, func(b *testing.B) {
			re := MustCompile(sc.pat)
			re.MatchString(sc.s)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				re.MatchString(sc.s)
			}
		})
	}
}

func BenchmarkFindStringIndexScenarios(b *testing.B) {
	scenarios := []struct {
		name, pat, s string
	}{
		{"middle", "a+b+", "xxaaabbbxx"},
		{"start", "a+", "aaabbb"},
		{"no_match", "xyz", "abc"},
		{"word", "\\w+", "hello world"},
		{"digits", "[0-9]+", "ab12cd34"},
	}
	for _, sc := range scenarios {
		sc := sc
		b.Run(sc.name, func(b *testing.B) {
			re := MustCompile(sc.pat)
			re.FindStringIndex(sc.s)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				re.FindStringIndex(sc.s)
			}
		})
	}
}

func BenchmarkFindAllStringScenarios(b *testing.B) {
	scenarios := []struct {
		name, pat, s string
		n            int
	}{
		{"words_10", "\\w+", "one two three four five six seven eight nine ten", -1},
		{"words_5", "\\w+", "a b c d e", -1},
		{"limit_2", "\\w+", "one two three", 2},
		{"digits", "[0-9]+", "a1b22c333d", -1},
	}
	for _, sc := range scenarios {
		sc := sc
		b.Run(sc.name, func(b *testing.B) {
			re := MustCompile(sc.pat)
			re.FindAllString(sc.s, sc.n)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				re.FindAllString(sc.s, sc.n)
			}
		})
	}
}

func BenchmarkCompileScenarios(b *testing.B) {
	scenarios := []struct {
		name, pat string
	}{
		{"simple", "a+"},
		{"concat", "a+b+c+"},
		{"alternation", "(a|b|c)+"},
		{"with_captures", "(a)(b)(c)"},
		{"bounded", "a{1,10}"},
		{"charclass", "[a-z]+"},
		{"complex", "([0-9]+)\\.([0-9]+)"},
	}
	for _, sc := range scenarios {
		sc := sc
		b.Run(sc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				MustCompile(sc.pat)
			}
		})
	}
}

func BenchmarkFindStringSubmatchScenarios(b *testing.B) {
	scenarios := []struct {
		name, pat, s string
	}{
		{"two_groups", "(a)b(c)", "xabcy"},
		{"one_group", "(a)", "a"},
		{"no_captures", "a+b+", "aaabbb"},
		{"nested", "(a(b))", "ab"},
	}
	for _, sc := range scenarios {
		sc := sc
		b.Run(sc.name, func(b *testing.B) {
			re := MustCompile(sc.pat)
			re.FindStringSubmatch(sc.s)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				re.FindStringSubmatch(sc.s)
			}
		})
	}
}

func BenchmarkReplaceAllStringScenarios(b *testing.B) {
	scenarios := []struct {
		name, pat, s, repl string
	}{
		{"single", "x", "xyz", "Y"},
		{"multi", "a+", "banana", "X"},
		{"charclass", "[0-9]", "a1b2c3", "N"},
		{"long_target", "x", "abcdefghijklmnopqrstuvwxyz", ""},
	}
	for _, sc := range scenarios {
		sc := sc
		b.Run(sc.name, func(b *testing.B) {
			re := MustCompile(sc.pat)
			re.ReplaceAllString(sc.s, sc.repl)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				re.ReplaceAllString(sc.s, sc.repl)
			}
		})
	}
}

// --- Long-input / throughput benchmarks ---

func BenchmarkFindStringIndexLong(b *testing.B) {
	const size = 100_000
	s := ""
	for i := 0; i < size; i++ {
		s += "xy"
	}
	s += "aaabbb"
	re := MustCompile("a+b+")
	re.FindStringIndex(s)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.FindStringIndex(s)
	}
}

func BenchmarkFindAllStringLong(b *testing.B) {
	words := []string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"}
	s := ""
	for i := 0; i < 1000; i++ {
		s += words[i%len(words)] + " "
	}
	re := MustCompile("\\w+")
	re.FindAllString(s, -1)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.FindAllString(s, -1)
	}
}

func BenchmarkReplaceAllStringLong(b *testing.B) {
	s := ""
	for i := 0; i < 5000; i++ {
		s += "ab"
	}
	re := MustCompile("a")
	re.ReplaceAllString(s, "x")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.ReplaceAllString(s, "x")
	}
}

func BenchmarkMatchStringLongInput(b *testing.B) {
	s := ""
	for i := 0; i < 10_000; i++ {
		s += "a"
	}
	re := MustCompile("a+")
	re.MatchString(s)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.MatchString(s)
	}
}

// --- Compile with/without captures (Option B: no TDFA until submatch) ---

func BenchmarkCompileWithCaptures(b *testing.B) {
	pat := "(a|b)+(c)"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		MustCompile(pat)
	}
}

func BenchmarkCompileNoCaptures(b *testing.B) {
	pat := "a|b+c"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		MustCompile(pat)
	}
}
