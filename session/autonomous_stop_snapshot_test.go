package session

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBoundedStopTail_RuneSafeAtByteCap(t *testing.T) {
	t.Parallel()

	// One long line so the byte cap (not the line cap) is what bites.
	// 10,005 bytes total (4999*2 + len("tail-") + 2), cut lands inside a rune.
	body := strings.Repeat("é", 4999) + "tail-é"
	got := BoundedStopTail(body)

	if !utf8.ValidString(got) {
		t.Fatalf("result is not valid UTF-8: %q", got[:min(len(got), 32)])
	}
	if len(got) > stopTailMaxBytes {
		t.Fatalf("len = %d, want <= %d", len(got), stopTailMaxBytes)
	}
	if !strings.HasSuffix(got, "tail-é") {
		t.Fatalf("result lost the last line's text: %q", got[len(got)-16:])
	}
	if !strings.HasPrefix(got, "é") {
		t.Fatalf("cut did not land on a rune boundary: first rune is %q", []rune(got)[0])
	}
}

func TestBoundedStopTail_RuneSafeForEveryCutOffset(t *testing.T) {
	t.Parallel()

	// Shifting an ASCII prefix moves the cut through every byte of a 3-byte rune.
	for pad := 0; pad < 4; pad++ {
		in := strings.Repeat("a", pad) + strings.Repeat("€", 3000)
		got := BoundedStopTail(in)
		if !utf8.ValidString(got) || len(got) > stopTailMaxBytes {
			t.Fatalf("pad=%d: valid=%v len=%d", pad, utf8.ValidString(got), len(got))
		}
	}
}

func TestBoundedStopTail_LineCapAndANSI(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&b, "\x1b[31mline-%03d\x1b[0m\n", i)
	}
	b.WriteString("\x1b]0;window title\x07\n\n   \n")

	got := BoundedStopTail(b.String())
	lines := strings.Split(got, "\n")

	if strings.Contains(got, "\x1b") {
		t.Errorf("escape sequences survived: %q", got)
	}
	if strings.Contains(got, "window title") {
		t.Errorf("OSC sequence body survived")
	}
	// The OSC line becomes an empty line and the blank lines are trimmed, so the
	// last kept text line is line-199.
	if last := lines[len(lines)-1]; last != "line-199" {
		t.Errorf("last line = %q, want line-199 (trailing blanks must be trimmed)", last)
	}
	if len(lines) > stopTailMaxLines {
		t.Errorf("kept %d lines, want <= %d", len(lines), stopTailMaxLines)
	}
	if strings.Contains(got, "line-000") || !strings.Contains(got, "line-170") {
		t.Errorf("did not keep the last lines: first=%q", lines[0])
	}
}

func TestBoundedStopTail_EdgeInputs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want func(t *testing.T, got string)
	}{
		{"empty", "", func(t *testing.T, got string) {
			if got != "" {
				t.Errorf("got %q, want empty", got)
			}
		}},
		{"ansi only", "\x1b[2J\x1b[H\x1b[0m", func(t *testing.T, got string) {
			if got != "" {
				t.Errorf("got %q, want empty", got)
			}
		}},
		{"invalid utf8 dropped, valid runes kept", "ok-é\xff\xfe-end", func(t *testing.T, got string) {
			if got != "ok-é-end" {
				t.Errorf("got %q, want %q", got, "ok-é-end")
			}
		}},
		{"exactly at cap is untouched", strings.Repeat("x", stopTailMaxBytes), func(t *testing.T, got string) {
			if len(got) != stopTailMaxBytes {
				t.Errorf("len = %d, want %d", len(got), stopTailMaxBytes)
			}
		}},
		{"cap plus one drops one byte from the front", strings.Repeat("x", stopTailMaxBytes+1), func(t *testing.T, got string) {
			if len(got) != stopTailMaxBytes {
				t.Errorf("len = %d, want %d", len(got), stopTailMaxBytes)
			}
		}},
		{"short input is untouched", "hello\nworld", func(t *testing.T, got string) {
			if got != "hello\nworld" {
				t.Errorf("got %q", got)
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := BoundedStopTail(tc.in)
			if !utf8.ValidString(got) {
				t.Fatalf("invalid UTF-8 result %q", got)
			}
			tc.want(t, got)
		})
	}
}

func TestTailBytes(t *testing.T) {
	t.Parallel()

	if got := TailBytes("abc", 5); got != "abc" {
		t.Errorf("under cap: got %q", got)
	}
	if got := TailBytes("abcdef", 3); got != "def" {
		t.Errorf("ascii: got %q", got)
	}
	// "€" is 3 bytes; a 4-byte tail of "a€€" would start mid-rune and must advance.
	got := TailBytes("a€€", 4)
	if !utf8.ValidString(got) || got != "€" {
		t.Errorf("multibyte: got %q", got)
	}
}
