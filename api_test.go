package re3

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sync"
	"testing"
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
	{"empty match at start", "a*", "bbb", build(0, 0, 1, 1, 2, 2, 3, 3)},
	{"multiple potential", "a*", "aaab", build(0, 3, 3, 3, 4, 4)},
	{"complement", "~(.*x.*)", "abc", build(0, 3, 3, 3)},
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
	"a", "a*", "a+", "a?", ".",
	"[a-z]", "\\d", "\\w", "\\s",
	"a|b", "a&b", "~(a)",
	"(a)", "a(b)c", ".*",
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
		{"(a", ErrMissingParen},       // unclosed group
		{"(?=x", ErrMissingParen},    // unclosed lookahead
		{"a{", ErrInvalidRepeatSize},
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
		pattern string
		input   string
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
		{"bounded", "a{2,}", "xaaaab", false, "aaaa"},         // full string does not match a{2,}
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
	matchInput := "aaabbb"        // full match
	findInput := "xxaaabbbxx"     // FindStringIndex returns [2,8] -> "aaabbb"
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
			cre := MustCompile(tc.pattern).Concurrent()
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
		cre := MustCompile("\\w+").Concurrent()
		got := cre.FindAllString("one two three", -1)
		want := []string{"one", "two", "three"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ConcurrentRegExp.FindAllString = %v, want %v", got, want)
		}
	})
	t.Run("ReplaceAllString", func(t *testing.T) {
		cre := MustCompile("a+").Concurrent()
		got := cre.ReplaceAllString("banana", "X")
		if got != "bXnXnX" {
			t.Errorf("ConcurrentRegExp.ReplaceAllString = %q, want bXnXnX", got)
		}
	})
	t.Run("Split", func(t *testing.T) {
		cre := MustCompile(":").Concurrent()
		got := cre.Split("foo:and:bar", -1)
		want := []string{"foo", "and", "bar"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ConcurrentRegExp.Split = %v, want %v", got, want)
		}
	})
}

func TestConcurrentParallel(t *testing.T) {
	cre := MustCompile("a+b+").Concurrent()
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
			clones := make([]*RegExp, g)
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
				go func(c *RegExp) {
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
	cre := MustCompile("a+b+").Concurrent()
	s := "xxaaabbbxx"
	cre.MatchString(s)   // warm cache
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
