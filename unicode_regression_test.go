package re3

import (
	"strings"
	"testing"
	"time"
)

func TestUnicodeBenchmarkRegressionSubset(t *testing.T) {
	tests := []struct {
		name  string
		pat   string
		input string
		want  int
	}{
		{
			name:  "test/unicode/letter/pL-matches-bmp-delta",
			pat:   `\p{L}+`,
			input: "123 Δelta 456",
			want:  1,
		},
		{
			name:  "test/unicode/decimal/unicode",
			pat:   `\p{Nd}+`,
			input: "x१२३y",
			want:  1,
		},
		{
			name:  "test/unicode/case/unicode",
			pat:   `(?iu)привет`,
			input: "Привет",
			want:  1,
		},
		{
			name:  "unicode/codepoints/contiguous-greek",
			pat:   `[α-ω]+`,
			input: "abc αβγ δεζ xyz",
			want:  2,
		},
		{
			name:  "test/unicode/invalid-utf8/dot-no-matches-xFF",
			pat:   `(?u:.)`,
			input: string([]byte{0xFF}),
			want:  0,
		},
		{
			name:  "test/unicode/word-boundary/unicode-connector-punctuation",
			pat:   `(?u:\b)`,
			input: "⁀",
			want:  2,
		},
		{
			name: "wild/bibleref/short-real-world-shape",
			pat: `(?P<Book>(([1234]|I{1,4})[\t\f\pZ]*)?\pL+\.?)[\t\f\pZ]+` +
				`(?P<Locations>((?P<Chapter>1?[0-9]?[0-9])(-(?P<ChapterEnd>\d+)|,\s*(?P<ChapterNext>\\d+))*` +
				`(:\s*(?P<Verse>\d+))?(-(?P<VerseEnd>\d+)|,\s*(?P<VerseNext>\d+))*\s?)+)`,
			input: "Gen 1:1, 2\n3 King 1:3-4\nII Ki. 3:12-14, 25\n",
			want:  3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			re := MustCompile(tc.pat)
			got := re.FindAllStringIndex(tc.input, -1)
			if len(got) != tc.want {
				t.Fatalf("FindAllStringIndex(%q, %q) count = %d, want %d", tc.pat, tc.input, len(got), tc.want)
			}
		})
	}
}

func TestInlineUnicodeCompileRegressions(t *testing.T) {
	patterns := []string{
		`(?u:\pL)`,
		`(?u:[^a])`,
		`(?iu:\p{Greek}+)`,
	}
	for _, pat := range patterns {
		t.Run(pat, func(t *testing.T) {
			if _, err := Compile(pat); err != nil {
				t.Fatalf("Compile(%q) returned error: %v", pat, err)
			}
		})
	}
}

func TestDictionaryLikeUnicodeAlternationCompiles(t *testing.T) {
	words := []string{"Zubeneschamali", "ZwickauerDamm", "Zephyranthes", "Zimmerpflanze"}
	alts := make([]string, 0, 2048)
	for i := 0; i < 512; i++ {
		alts = append(alts, words[i%len(words)])
	}
	pat := `(?u:(?:` + strings.Join(alts, "|") + `))`
	runWithTimeout(t, 2*time.Second, func() {
		if _, err := Compile(pat); err != nil {
			t.Fatalf("Compile(dictionary-like pattern) returned error: %v", err)
		}
	})
}

func TestLowerOrUpperPropertyBehavior(t *testing.T) {
	re := MustCompile(`(?u:\p{LlOrLu}+)`)
	if got := re.FindString("abc XYZ"); got != "abc" {
		t.Fatalf("expected lower-case segment to match, got %q", got)
	}

	// U+01C5 LATIN CAPITAL LETTER D WITH SMALL LETTER Z WITH CARON is titlecase.
	if got := MustCompile(`(?u:\p{LlOrLu}+)`).FindString("ǅ"); got != "ǅ" {
		t.Fatalf("expected titlecase letter to match LlOrLu, got %q", got)
	}
}
