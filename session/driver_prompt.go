package session

import (
	"errors"
	"fmt"
	"strings"
)

// Bracketed-paste markers (DECSET 2004). Inside them a TUI treats newlines and
// control bytes as literal text instead of keystrokes.
const (
	bracketedPasteStart = "\x1b[200~"
	bracketedPasteEnd   = "\x1b[201~"
)

// ErrPromptUndeliverable marks a prompt that cannot be typed into a live pane
// without changing its meaning. It is deterministic, so callers must not retry.
var ErrPromptUndeliverable = errors.New("prompt cannot be delivered to the pane intact")

// DriverPrompt is a prompt proven safe to type into a live pane. Size is not a
// constraint: Keys is written in chunks (sendKeysChunked), never truncated.
type DriverPrompt struct{ keys string }

// cleanDriverPromptText drops NUL bytes (tmux drops them unpredictably) and
// surrounding whitespace. Empty means "fall back to driverInitialPrompt".
func cleanDriverPromptText(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\x00", ""))
}

// ParseDriverPrompt turns prompt text into keystrokes. Text with a control byte
// (newline, tab, ESC, ...) goes in a bracketed paste so it arrives as text, not
// as Enter or a key chord; plain printable text is typed as is. Text containing
// the paste terminator is refused, since it would end the paste early and let
// the rest act as keystrokes.
func ParseDriverPrompt(s string) (DriverPrompt, error) {
	text := cleanDriverPromptText(s)
	if strings.Contains(text, bracketedPasteEnd) {
		return DriverPrompt{}, fmt.Errorf("%w: contains the bracketed-paste terminator ESC[201~", ErrPromptUndeliverable)
	}
	if !hasControlByte(text) {
		return DriverPrompt{keys: text}, nil
	}
	return DriverPrompt{keys: bracketedPasteStart + text + bracketedPasteEnd}, nil
}

// Keys is the exact byte sequence to write to the pane.
func (p DriverPrompt) Keys() string { return p.keys }

func hasControlByte(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 || c == 0x7f {
			return true
		}
	}
	return false
}
