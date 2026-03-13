package re3

import (
	"testing"
	"time"
)

func TestBenchmarkCaptures(t *testing.T) {
	const fixtureRoot = "testdata/benchmarks"

	t.Run("contiguous-letters", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping slow captures in short mode")
		}
		re := MustCompile("(?:(a+)|(b+)|(c+)|(d+)|(e+)|(f+)|(g+)|(h+)|(i+)|(j+)|(k+)|(l+)|(m+)|(n+)|(o+)|(p+)|(q+)|(r+)|(s+)|(t+)|(u+)|(v+)|(w+)|(x+)|(y+)|(z+))")
		haystack := readFixture(t, fixtureRoot+"/haystacks/opensubtitles/en-medium.txt")
		runWithTimeout(t, 15*time.Second, func() {
			locs := re.FindAllStringSubmatchIndex(haystack, -1)
			// rebar expects 81_494; allow some variance
			if len(locs) < 80000 || len(locs) > 83000 {
				t.Errorf("expected ~81494 capture matches, got %d", len(locs))
			}
		})
	})
}
