package re3

// RegExp is the interface implemented by both the default and thread-safe regexp implementations.
type RegExp interface {
	// Match reports whether the byte slice b contains any match of the regular expression.
	Match(b []byte) bool
	// MatchString reports whether the string s contains any match of the regular expression.
	MatchString(s string) bool

	// FindIndex returns a two-element slice of integers defining the location of the leftmost
	// match in b. The match itself is at b[loc[0]:loc[1]]. A return value of nil indicates no match.
	FindIndex(b []byte) []int
	// FindStringIndex returns a two-element slice of integers defining the location of the leftmost
	// match in s. The match itself is at s[loc[0]:loc[1]]. A return value of nil indicates no match.
	FindStringIndex(s string) []int

	// Find returns a slice holding the text of the leftmost match in b. A return value of nil indicates no match.
	Find(b []byte) []byte
	// FindString returns a string holding the text of the leftmost match in s. If there is no match,
	// the return value is an empty string, but it will also be empty if the expression matches an empty
	// string. Use FindStringIndex or FindStringSubmatch to distinguish these cases.
	FindString(s string) string

	// FindAll is the 'All' version of Find; it returns a slice of all successive matches of the
	// expression. A return value of nil indicates no match.
	FindAll(b []byte, n int) [][]byte
	// FindAllString is the 'All' version of FindString; it returns a slice of all successive
	// matches of the expression. A return value of nil indicates no match.
	FindAllString(s string, n int) []string

	// FindAllIndex is the 'All' version of FindIndex; it returns a slice of all successive matches
	// of the expression. A return value of nil indicates no match.
	FindAllIndex(b []byte, n int) [][]int
	// FindAllStringIndex is the 'All' version of FindStringIndex; it returns a slice of all
	// successive matches of the expression. A return value of nil indicates no match.
	FindAllStringIndex(s string, n int) [][]int

	// FindSubmatch returns a slice of slices holding the text of the leftmost match in b and the
	// matches, if any, of its subexpressions. A return value of nil indicates no match.
	FindSubmatch(b []byte) [][]byte
	// FindStringSubmatch returns a slice of strings holding the text of the leftmost match in s
	// and the matches, if any, of its subexpressions. A return value of nil indicates no match.
	FindStringSubmatch(s string) []string

	// FindAllSubmatch is the 'All' version of FindSubmatch; it returns a slice of all successive
	// matches of the expression. A return value of nil indicates no match.
	FindAllSubmatch(b []byte, n int) [][][]byte
	// FindAllStringSubmatch is the 'All' version of FindStringSubmatch; it returns a slice of all
	// successive matches of the expression. A return value of nil indicates no match.
	FindAllStringSubmatch(s string, n int) [][]string

	// FindSubmatchIndex returns a slice holding the index pairs identifying the leftmost match in b
	// and the matches, if any, of its subexpressions. A return value of nil indicates no match.
	FindSubmatchIndex(b []byte) []int
	// FindStringSubmatchIndex returns a slice holding the index pairs identifying the leftmost match
	// in s and the matches, if any, of its subexpressions. A return value of nil indicates no match.
	FindStringSubmatchIndex(s string) []int

	// FindAllSubmatchIndex is the 'All' version of FindSubmatchIndex; it returns a slice of all
	// successive matches of the expression. A return value of nil indicates no match.
	FindAllSubmatchIndex(b []byte, n int) [][]int
	// FindAllStringSubmatchIndex is the 'All' version of FindStringSubmatchIndex; it returns a
	// slice of all successive matches of the expression. A return value of nil indicates no match.
	FindAllStringSubmatchIndex(s string, n int) [][]int

	// Split slices s into substrings separated by the expression and returns a slice of the
	// substrings between those expression matches. The count n determines the number of substrings
	// to return: n > 0 at most n substrings (last is the unsplit remainder); n == 0 the result is
	// nil; n < 0 all substrings.
	Split(s string, n int) []string

	// ReplaceAll returns a copy of src, replacing matches of the regular expression with the
	// replacement bytes repl. Inside repl, $ signs are interpreted as in Expand.
	ReplaceAll(src, repl []byte) []byte
	// ReplaceAllString returns a copy of src, replacing matches of the regular expression with
	// the replacement string repl. Inside repl, $ signs are interpreted as in Expand.
	ReplaceAllString(s, repl string) string

	// Clone returns a copy of the regular expression; it is safe for concurrent use by multiple goroutines.
	Clone() RegExp
}

func MustCompile(expr string) RegExp {
	re, err := Compile(expr)
	if err != nil {
		panic(err)
	}
	return re
}

// Concurrent returns a thread-safe RegExp implementation. If re is already
// a ConcurrentRegExp it is returned unchanged; otherwise re must be from
// Compile/MustCompile and is wrapped in a new ConcurrentRegExp.
func Concurrent(re RegExp) RegExp {
	if c, ok := re.(*concurrentRegExpImpl); ok {
		return c
	}
	if impl, ok := re.(*regexpImpl); ok {
		return &concurrentRegExpImpl{re: impl}
	}
	return re
}

// Compile compiles a regular expression into a RegExp.
func Compile(expr string) (RegExp, error) {
	return compile(expr)
}
