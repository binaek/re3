package re3

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// runWithTimeout runs fn in a goroutine and fails the test if it exceeds timeout.
// If onTimeout is non-nil and a timeout occurs, it is called before failing (e.g. to log metrics).
func runWithTimeout(t *testing.T, timeout time.Duration, fn func(), onTimeout ...func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		for _, f := range onTimeout {
			f()
		}
		t.Fatalf("operation exceeded timeout %s", timeout)
	}
}

// readFixture reads a file from testdata (relative to package root).
func readFixture(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return string(b)
}

// buildPerLineAlternation builds (?:a|b|c) from per-line file contents.
// If literal, each line is regex-escaped. If unicode, wraps in (?u:...).
func buildPerLineAlternation(t *testing.T, contents string, literal, unicode bool) string {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(contents), "\n")
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		part := strings.TrimSpace(line)
		if part == "" {
			continue
		}
		if literal {
			part = regexp.QuoteMeta(part)
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		t.Fatalf("expected non-empty per-line regex fixture")
	}
	body := "(?:" + strings.Join(parts, "|") + ")"
	if unicode {
		return "(?u:" + body + ")"
	}
	return body
}

// grepCount counts lines where re matches (grep-style).
func grepCount(re RegExp, haystack string) int {
	count := 0
	start := 0
	for start <= len(haystack) {
		rel := strings.IndexByte(haystack[start:], '\n')
		var line string
		if rel < 0 {
			line = haystack[start:]
			start = len(haystack) + 1
		} else {
			line = haystack[start : start+rel]
			start += rel + 1
		}
		if re.FindStringIndex(line) != nil {
			count++
		}
	}
	return count
}

// grepCapturesCount counts lines where re has a capturing match.
func grepCapturesCount(re RegExp, haystack string) int {
	count := 0
	start := 0
	for start <= len(haystack) {
		rel := strings.IndexByte(haystack[start:], '\n')
		var line string
		if rel < 0 {
			line = haystack[start:]
			start = len(haystack) + 1
		} else {
			line = haystack[start : start+rel]
			start += rel + 1
		}
		if re.FindStringSubmatchIndex(line) != nil {
			count++
		}
	}
	return count
}
