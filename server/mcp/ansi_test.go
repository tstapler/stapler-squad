package mcp

import (
	crand "crypto/rand"
	"math/rand"
	"testing"
	"unicode/utf8"
)

func TestANSIStripDefault(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"\x1b[32mgreen text\x1b[0m", "green text"},
		{"\x1b[1;31mbold red\x1b[0m", "bold red"},
	}
	for _, tc := range cases {
		got := string(stripANSI([]byte(tc.input)))
		if got != tc.want {
			t.Errorf("stripANSI(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestANSIPreservedWhenRaw(t *testing.T) {
	raw := []byte("\x1b[32mtext\x1b[0m")

	// Verify the source has escape bytes before stripping.
	hasESC := false
	for _, b := range raw {
		if b == 0x1b {
			hasESC = true
			break
		}
	}
	if !hasESC {
		t.Fatal("expected raw bytes to contain ESC (0x1b)")
	}

	result := stripANSI(raw)
	for _, b := range result {
		if b == 0x1b {
			t.Errorf("stripANSI result still contains ESC byte; got %q", result)
			break
		}
	}
}

func TestPartialEscapeSequenceAtBoundary(t *testing.T) {
	cases := []struct {
		name                string
		input               string
		want                string
		wantReplacementChar bool
	}{
		{name: "incomplete CSI", input: "\x1b[", want: ""},
		{name: "bare ESC", input: "\x1b", want: ""},
		{name: "invalid UTF-8", input: "\xff\xfe", wantReplacementChar: true},
		{name: "mixed ANSI and text", input: "hello\x1b[32mworld\x1b[0m!", want: "helloworld!"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := stripANSI([]byte(tc.input))
			if tc.wantReplacementChar {
				if !utf8.Valid(result) {
					t.Errorf("result is not valid UTF-8: %q", result)
				}
				// U+FFFD replacement character encoded as 0xEF 0xBF 0xBD
				replacement := "\uFFFD"
				if !containsSubstring(result, []byte(replacement)) {
					t.Errorf("expected replacement character U+FFFD in result %q", result)
				}
			} else {
				got := string(result)
				if got != tc.want {
					t.Errorf("stripANSI(%q) = %q, want %q", tc.input, got, tc.want)
				}
			}
		})
	}
}

func containsSubstring(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func TestANSIOSCSequences(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "BEL-terminated OSC", input: "\x1b]0;title\x07text", want: "text"},
		{name: "ST-terminated OSC", input: "\x1b]0;title\x1b\\text", want: "text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(stripANSI([]byte(tc.input)))
			if got != tc.want {
				t.Errorf("stripANSI(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "LF", input: "a\nb\nc", want: []string{"a", "b", "c"}},
		{name: "CRLF", input: "a\r\nb\r\nc", want: []string{"a", "b", "c"}},
		{name: "bare CR", input: "a\rb\rc", want: []string{"a", "b", "c"}},
		{name: "trailing newline", input: "a\n", want: []string{"a"}},
		{name: "empty", input: "", want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := splitLines([]byte(tc.input))
			if len(result) != len(tc.want) {
				t.Fatalf("splitLines(%q) returned %d lines, want %d; got %v", tc.input, len(result), len(tc.want), result)
			}
			for i, line := range result {
				got := string(line)
				if got != tc.want[i] {
					t.Errorf("splitLines(%q)[%d] = %q, want %q", tc.input, i, got, tc.want[i])
				}
			}
		})
	}
}

func TestANSIStripNeverPanics(t *testing.T) {
	for i := 0; i < 1000; i++ {
		size := rand.Intn(256)
		buf := make([]byte, size)
		_, _ = crand.Read(buf)
		result := stripANSI(buf)
		if !utf8.Valid(result) {
			t.Errorf("iteration %d: result is not valid UTF-8", i)
		}
	}
}
