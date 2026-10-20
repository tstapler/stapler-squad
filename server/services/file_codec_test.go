package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func TestNewFileVersionToken_ShouldRoundTripParse_When_V1TokenFormat(t *testing.T) {
	t.Parallel()
	raw := []byte("hello\r\nworld\r\n")
	sum := sha256.Sum256(raw)
	tok := NewFileVersionToken(int64(len(raw)), 1700000000123456789, raw)

	want := fmt.Sprintf("v1:%d:%d:%s", len(raw), int64(1700000000123456789), hex.EncodeToString(sum[:]))
	assert.Equal(t, want, tok.String())
	assert.True(t, tok.Editable())

	parsed, err := ParseFileVersionToken(tok.String())
	require.NoError(t, err)
	assert.Equal(t, tok, parsed)

	// A well-formed "-" token parses (WriteFile refuses it later) but is not editable.
	dash := NonEditableToken(2<<20, 42)
	assert.Equal(t, "v1:2097152:42:-", dash.String())
	parsedDash, err := ParseFileVersionToken(dash.String())
	require.NoError(t, err)
	assert.False(t, parsedDash.Editable(), "a '-' token must never authorise a write")
	assert.Equal(t, dash, parsedDash)
}

func TestParseFileVersionToken_ShouldError_When_MalformedOrWrongVersion(t *testing.T) {
	t.Parallel()
	good := strings.Repeat("a", 64)
	for name, in := range map[string]string{
		"empty":           "",
		"wrong version":   "v2:1:2:" + good,
		"no version":      "1:2:" + good,
		"too few parts":   "v1:1:2",
		"too many parts":  "v1:1:2:" + good + ":x",
		"negative size":   "v1:-1:2:" + good,
		"non numeric":     "v1:a:2:" + good,
		"bad mtime":       "v1:1:b:" + good,
		"short hash":      "v1:1:2:abc",
		"uppercase hash":  "v1:1:2:" + strings.ToUpper(good),
		"non hex hash":    "v1:1:2:" + strings.Repeat("z", 64),
		"empty hash part": "v1:1:2:",
	} {
		_, err := ParseFileVersionToken(in)
		assert.Error(t, err, name)
	}
}

func TestDetectLineEnding_ShouldClassify_When_LfCrlfMixedAndLoneCr(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		in   string
		want sessionv1.LineEnding
	}{
		"empty":                {"", sessionv1.LineEnding_LINE_ENDING_LF},
		"no newline":           {"abc", sessionv1.LineEnding_LINE_ENDING_LF},
		"lf":                   {"a\nb\n", sessionv1.LineEnding_LINE_ENDING_LF},
		"crlf":                 {"a\r\nb\r\n", sessionv1.LineEnding_LINE_ENDING_CRLF},
		"mixed lf and crlf":    {"a\r\nb\nc\r\n", sessionv1.LineEnding_LINE_ENDING_MIXED},
		"lone cr in lf file":   {"a\nb\rc\n", sessionv1.LineEnding_LINE_ENDING_MIXED},
		"lone cr in crlf file": {"a\r\nb\rc\r\n", sessionv1.LineEnding_LINE_ENDING_MIXED},
		"trailing lone cr":     {"a\r\nb\r", sessionv1.LineEnding_LINE_ENDING_MIXED},
		"cr only":              {"a\rb\r", sessionv1.LineEnding_LINE_ENDING_MIXED},
	} {
		assert.Equal(t, tc.want, detectLineEnding([]byte(tc.in)), name)
	}
}

func TestDetectBOM_ShouldStripOnlyLeadingUtf8Bom(t *testing.T) {
	t.Parallel()
	has, rest := detectBOM([]byte("\xEF\xBB\xBFhi"))
	assert.True(t, has)
	assert.Equal(t, "hi", string(rest))
	has, rest = detectBOM([]byte("hi\xEF\xBB\xBF"))
	assert.False(t, has)
	assert.Equal(t, "hi\xEF\xBB\xBF", string(rest))
	has, _ = detectBOM(nil)
	assert.False(t, has)
}

func TestValidateEditableUTF8_ShouldRejectNulAndInvalidUtf8(t *testing.T) {
	t.Parallel()
	assert.NoError(t, validateEditableUTF8([]byte("ok é世")))
	assert.NoError(t, validateEditableUTF8(nil))
	assert.ErrorIs(t, validateEditableUTF8([]byte("a\x00b")), errContainsNUL)
	assert.ErrorIs(t, validateEditableUTF8([]byte("caf\xE9")), errNotUTF8)
}

func TestClassifyReadOnly_ShouldMapFixtures_When_BinaryLatin1MixedLoneCrSymlink(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		facts readFacts
		want  sessionv1.ReadOnlyReason
	}{
		"editable":        {readFacts{Body: []byte("a\nb\n")}, sessionv1.ReadOnlyReason_READ_ONLY_REASON_NONE},
		"editable crlf":   {readFacts{Body: []byte("a\r\nb\r\n")}, sessionv1.ReadOnlyReason_READ_ONLY_REASON_NONE},
		"binary":          {readFacts{Binary: true}, sessionv1.ReadOnlyReason_READ_ONLY_REASON_BINARY},
		"late nul":        {readFacts{Body: []byte("text\x00x")}, sessionv1.ReadOnlyReason_READ_ONLY_REASON_BINARY},
		"truncated":       {readFacts{Truncated: true, Body: []byte("abc")}, sessionv1.ReadOnlyReason_READ_ONLY_REASON_TRUNCATED},
		"latin-1":         {readFacts{Body: []byte("caf\xE9\n")}, sessionv1.ReadOnlyReason_READ_ONLY_REASON_NON_UTF8},
		"mixed eol":       {readFacts{Body: []byte("a\r\nb\nc\n")}, sessionv1.ReadOnlyReason_READ_ONLY_REASON_MIXED_EOL},
		"lone cr":         {readFacts{Body: []byte("a\nb\rc\n")}, sessionv1.ReadOnlyReason_READ_ONLY_REASON_MIXED_EOL},
		"in-tree symlink": {readFacts{ViaLink: true, Body: []byte("a\n")}, sessionv1.ReadOnlyReason_READ_ONLY_REASON_SYMLINK},
		"binary beats truncated": {readFacts{Binary: true, Truncated: true},
			sessionv1.ReadOnlyReason_READ_ONLY_REASON_BINARY},
	} {
		assert.Equal(t, tc.want, classifyReadOnly(tc.facts), name)
	}
}

func TestTruncateOnRuneBoundary_ShouldNotSplitRune_When_CutInsideMultibyte(t *testing.T) {
	t.Parallel()
	s := "ab世界" // 2 + 3 + 3 bytes
	for cut := 0; cut <= len(s); cut++ {
		got := truncateOnRuneBoundary([]byte(s[:cut]))
		assert.True(t, utf8.Valid(got), "cut=%d", cut)
		assert.True(t, strings.HasPrefix(s, string(got)), "cut=%d", cut)
		assert.LessOrEqual(t, cut-len(got), utf8.UTFMax-1, "cut=%d dropped too much", cut)
	}
	assert.Equal(t, "ab世", string(truncateOnRuneBoundary([]byte(s[:7]))))
	assert.Empty(t, truncateOnRuneBoundary(nil))
}

func FuzzTruncateOnRuneBoundary(f *testing.F) {
	f.Add("ab世界", 4)
	f.Add("\xff\xfe", 1)
	f.Fuzz(func(t *testing.T, s string, cut int) {
		if cut < 0 || cut > len(s) {
			return
		}
		in := []byte(s[:cut])
		got := truncateOnRuneBoundary(in)
		if !strings.HasPrefix(string(in), string(got)) || len(in)-len(got) > utf8.UTFMax-1 {
			t.Fatalf("not a short prefix: in=%q got=%q", in, got)
		}
		if utf8.Valid(in[:0]) && utf8.ValidString(s[:cut]) && !utf8.Valid(got) {
			t.Fatalf("valid input became invalid: %q", got)
		}
	})
}

func FuzzDetectLineEnding(f *testing.F) {
	f.Add([]byte("a\r\nb\n"))
	f.Add([]byte("a\rb"))
	f.Fuzz(func(t *testing.T, b []byte) {
		le := detectLineEnding(b)
		if le == sessionv1.LineEnding_LINE_ENDING_UNSPECIFIED {
			t.Fatal("never unspecified")
		}
		// LF and CRLF files must survive a normalize/re-apply round trip byte for byte.
		if le == sessionv1.LineEnding_LINE_ENDING_CRLF {
			lf := strings.ReplaceAll(string(b), "\r\n", "\n")
			if strings.Contains(lf, "\r") || strings.ReplaceAll(lf, "\n", "\r\n") != string(b) {
				t.Fatalf("CRLF round trip changed bytes: %q", b)
			}
		}
		if le == sessionv1.LineEnding_LINE_ENDING_LF && strings.Contains(string(b), "\r") {
			t.Fatalf("LF file contains CR: %q", b)
		}
	})
}
