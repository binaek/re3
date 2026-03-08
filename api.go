package re3

// RegExp is the interface implemented by both the default and thread-safe regexp implementations.
type RegExp interface {
	MatchString(s string) bool
	Match(b []byte) bool
	FindStringIndex(s string) []int
	FindString(s string) string
	Find(b []byte) []byte
	FindIndex(b []byte) []int
	FindAllStringIndex(s string, n int) [][]int
	FindAllString(s string, n int) []string
	FindAll(b []byte, n int) [][]byte
	FindAllIndex(b []byte, n int) [][]int
	FindStringSubmatch(s string) []string
	FindAllStringSubmatch(s string, n int) [][]string
	Split(s string, n int) []string
	ReplaceAllString(s, repl string) string
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
