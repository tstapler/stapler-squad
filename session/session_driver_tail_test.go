package session

import (
	"strings"
	"testing"
)

var tailBenchOutput = strings.Repeat("some terminal line with ordinary content\n", 500)

// ---------------------------------------------------------------------------
// Unit tests — tailContent
// ---------------------------------------------------------------------------

func TestTailContent_ShorterThanWindow(t *testing.T) {
	t.Parallel()
	s := "hello\nworld\n"
	got := tailContent(s, 4096)
	if got != s {
		t.Errorf("expected unchanged string, got %q", got)
	}
}

func TestTailContent_LongerThanWindow(t *testing.T) {
	t.Parallel()
	// Build a string with 10 lines; keep only the last 3.
	content := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	// Window large enough to capture last ~3 lines but not all.
	got := tailContent(content, 20)
	// Must not start mid-line.
	if len(got) == 0 || got[0] == '\n' {
		t.Errorf("tail starts at bad position: %q", got)
	}
	// The last line of content must be present.
	if !strings.Contains(got, "line10") {
		t.Errorf("tail missing last line, got: %q", got)
	}
}

func TestTailContent_ExactlyWindowSize(t *testing.T) {
	t.Parallel()
	s := strings.Repeat("x", statusDetectionTailBytes)
	got := tailContent(s, statusDetectionTailBytes)
	if got != s {
		t.Errorf("expected unchanged string for exact-size input")
	}
}

func TestTailContent_NoNewlineInTail(t *testing.T) {
	t.Parallel()
	// Content that after slicing has no newline — entire tail is one line.
	prefix := strings.Repeat("a\n", 200) // lots of short lines
	suffix := strings.Repeat("b", 100)   // no newline, fits in window
	content := prefix + suffix
	got := tailContent(content, 200)
	// Should contain the suffix (no newline, so tail starts wherever the slice lands)
	if !strings.Contains(got, suffix) {
		t.Errorf("expected tail to include no-newline suffix, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Unit tests — hashString
// ---------------------------------------------------------------------------

func TestHashString_SameInputSameHash(t *testing.T) {
	t.Parallel()
	h1 := hashString("hello")
	h2 := hashString("hello")
	if h1 != h2 {
		t.Error("identical inputs must produce identical hashes")
	}
}

func TestHashString_DifferentInputDifferentHash(t *testing.T) {
	t.Parallel()
	if hashString("hello") == hashString("world") {
		t.Error("different inputs must (almost certainly) produce different hashes")
	}
}

func TestHashString_EmptyString(t *testing.T) {
	t.Parallel()
	// Should not panic and should return a consistent value.
	h1 := hashString("")
	h2 := hashString("")
	if h1 != h2 {
		t.Error("empty string hash must be deterministic")
	}
}

func Benchmark_tailContent_Large(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tailContent(tailBenchOutput, statusDetectionTailBytes)
	}
}

func Benchmark_hashString_4KB(b *testing.B) {
	s := tailContent(tailBenchOutput, statusDetectionTailBytes)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hashString(s)
	}
}
