package tmux

import (
	"testing"
	"time"
)

// tmuxEncode mirrors tmux's own control-mode encoding rule (decodeControlModeOutput's
// doc comment): every byte < ASCII 32, plus a literal backslash, is replaced by a
// 3-digit octal escape "\ooo". Used to build realistic %output payloads for these
// tests instead of hand-typing octal digits.
func tmuxEncode(raw []byte) []byte {
	out := make([]byte, 0, len(raw)*4)
	for _, b := range raw {
		if b < 32 || b == '\\' {
			out = append(out, '\\', '0'+(b>>6)&7, '0'+(b>>3)&7, '0'+b&7)
			continue
		}
		out = append(out, b)
	}
	return out
}

// piRedrawBytes is the exact single-line redraw sequence emitted by
// @earendil-works/pi-tui@0.84.4 (extracted from dist/tui-main-screen.js via
// `npm pack`, not guessed): begin synchronized-output, CR + erase-line, the
// new text, end synchronized-output.
func piRedrawBytes(text string) []byte {
	return []byte("\x1b[?2026h\r\x1b[2K" + text + "\x1b[?2026l")
}

func TestDecodeControlModeOutput_RoundTripsPiTuiRedrawSequence(t *testing.T) {
	words := []string{"w", "wh", "whe", "when", "wheni", "wheni'", "wheni'm"}
	sess := &TmuxSession{}

	for _, w := range words {
		raw := piRedrawBytes(w)
		encoded := tmuxEncode(raw)
		decoded := sess.decodeControlModeOutput(encoded)

		if string(decoded) != string(raw) {
			t.Fatalf("decodeControlModeOutput mismatch for %q:\n  encoded: %q\n  want:    %q\n  got:     %q",
				w, encoded, raw, decoded)
		}
	}
}

func TestDecodeControlModeOutput_RoundTripsLiteralBackslashAndControlBytes(t *testing.T) {
	// tmux escapes a literal backslash the same way as any other <32 byte --
	// exercised separately since it's the one printable-range character that
	// still needs escaping (decodeControlModeOutput's \ooo branch is keyed off
	// seeing '\\', so a raw backslash in pane content is the case most likely
	// to be mishandled by an off-by-one in that branch).
	raw := []byte("a\\b\rc\nd\x1bcolor")
	encoded := tmuxEncode(raw)
	sess := &TmuxSession{}

	decoded := sess.decodeControlModeOutput(encoded)
	if string(decoded) != string(raw) {
		t.Fatalf("mismatch:\n  encoded: %q\n  want:    %q\n  got:     %q", encoded, raw, decoded)
	}
}

func TestHandleOutputBytes_BroadcastsDecodedPiTuiRedrawToSubscriber(t *testing.T) {
	// End-to-end through the real production entry point (handleOutputBytes),
	// not just the inner decode helper -- covers the "%output %PANE_ID " prefix
	// stripping and the subscriber broadcast, matching what readControlModeOutput
	// actually calls per scanned line.
	sess := &TmuxSession{sanitizedName: "test-session"}
	id, ch := sess.SubscribeToControlModeUpdates()
	defer sess.UnsubscribeFromControlModeUpdates(id)

	raw := piRedrawBytes("wheni'm")
	line := append([]byte("%output %0 "), tmuxEncode(raw)...)

	sess.handleOutputBytes(line)

	select {
	case got := <-ch:
		if string(got) != string(raw) {
			t.Fatalf("broadcast mismatch:\n  want: %q\n  got:  %q", raw, got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broadcast")
	}
}
