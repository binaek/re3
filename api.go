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
