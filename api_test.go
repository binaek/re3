package re3

import (
	"errors"
	"reflect"
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

func TestClone(t *testing.T) {
	re := MustCompile("a+")
	clone := re.Clone()
	if clone == re {
		t.Error("Clone should return a new instance")
	}
	if got := clone.MatchString("aaa"); !got {
		t.Error("Clone should match same pattern")
	}
}

func TestConcurrent(t *testing.T) {
	cre := MustCompile("a+").Concurrent()
	if cre == nil {
		t.Fatal("Concurrent() returned nil")
	}
	if !cre.MatchString("aaa") {
		t.Error("ConcurrentRegExp.MatchString should match")
	}
	if loc := cre.FindStringIndex("xaaab"); loc == nil || loc[0] != 1 || loc[1] != 4 {
		t.Errorf("ConcurrentRegExp.FindStringIndex want [1,4], got %v", loc)
	}
}

func TestBoundedRepetition(t *testing.T) {
	tests := []struct {
		pattern string
		text    string
		want    bool
	}{
		{"a{2}", "aa", true},
		{"a{2}", "a", false},
		{"a{2}", "aaa", false},
		{"a{1,3}", "a", true},
		{"a{1,3}", "aa", true},
		{"a{1,3}", "aaa", true},
		{"a{1,3}", "aaaa", false},
		{"a{2,}", "aa", true},
		{"a{2,}", "aaa", true},
		{"a{2,}", "a", false},
		{"[0-9]{2}", "12", true},
		{"[0-9]{2}", "1", false},
	}
	for _, tc := range tests {
		re := MustCompile(tc.pattern)
		got := re.MatchString(tc.text)
		if got != tc.want {
			t.Errorf("MatchString(%q, %q) = %v, want %v", tc.pattern, tc.text, got, tc.want)
		}
	}
}

func BenchmarkMatchString(b *testing.B) {
	re := MustCompile("a+b+")
	s := "aaabbb"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.MatchString(s)
	}
}

func BenchmarkMatch(b *testing.B) {
	re := MustCompile("a+b+")
	s := []byte("aaabbb")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.Match(s)
	}
}

func BenchmarkFindStringIndex(b *testing.B) {
	re := MustCompile("a+b+")
	s := "xxaaabbbxx"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.FindStringIndex(s)
	}
}

func BenchmarkFindAllString(b *testing.B) {
	re := MustCompile("\\w+")
	s := "one two three four five six seven eight nine ten"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re.FindAllString(s, -1)
	}
}

func BenchmarkReplaceAllString(b *testing.B) {
	re := MustCompile("[cjrw]")
	s := "abcdefghijklmnopqrstuvwxyz"
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
