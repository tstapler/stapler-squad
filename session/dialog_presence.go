package session

import (
	"regexp"
	"strings"
	"unicode"
)

// DialogPresence is what a pane capture says about "is any interactive dialog up".
// The zero value is DialogIndeterminate so an unset or unparsed result fails
// closed: callers treat only DialogAbsent as "no dialog" and never write on anything else.
type DialogPresence uint8

const (
	// DialogIndeterminate: an unrecognised layout, an empty capture or a capture error.
	DialogIndeterminate DialogPresence = iota
	// DialogShowing: a footer, a selected numbered row or a tab bar sits in the live tail.
	DialogShowing
	// DialogAbsent: the capture positively shows an idle composer at the bottom.
	DialogAbsent
)

func (p DialogPresence) String() string {
	switch p {
	case DialogShowing:
		return "showing"
	case DialogAbsent:
		return "absent"
	}
	return "indeterminate"
}

// Absent reports whether the capture positively shows no dialog.
func (p DialogPresence) Absent() bool { return p == DialogAbsent }

const (
	// dialogPresenceTail bounds how far from the bottom a marker still counts as live.
	dialogPresenceTail = 40
	// dialogComposerLookback is how many non-blank lines from the bottom the composer prompt may be:
	// the prompt, its bottom rule and up to two status lines.
	dialogComposerLookback = 6
	dialogFooterEsc        = "Esc to cancel"
	dialogComposerGlyph    = "❯"
	dialogRuleGlyph        = '─'
	dialogMinRuleRunes     = 10
)

var (
	selectedNumberedRow = regexp.MustCompile(`^❯\s+[0-9]+\.`)
	dialogTabBar        = regexp.MustCompile(`^←\s.*[☐☒✔].*→$`)
)

// PaneShowsDialog classifies a pane capture. It is bottom-anchored like
// DialogMatch: the live dialog is drawn last, so a composer at the bottom
// proves any footer above it is scrollback, while a footer, a selected
// numbered row (`❯ 1.`) or the multi-question tab bar (`← ☐ A ☐ B ✔ Submit →`)
// in the tail with no composer below it is a dialog. Text after a footer does
// not hide it: that is an unrecognised layout, so it stays Showing and a
// forged row cannot talk a caller into an Absent.
func PaneShowsDialog(capture string) DialogPresence {
	lines := presenceLines(capture)
	if len(lines) == 0 {
		return DialogIndeterminate
	}
	if composerAtBottom(lines) {
		return DialogAbsent
	}
	tail := lines
	if len(tail) > dialogPresenceTail {
		tail = tail[len(tail)-dialogPresenceTail:]
	}
	for _, l := range tail {
		if isDialogMarker(l) {
			return DialogShowing
		}
	}
	return DialogIndeterminate
}

// DialogPresenceFromCapture folds a capture error into DialogIndeterminate.
func DialogPresenceFromCapture(capture string, err error) DialogPresence {
	if err != nil {
		return DialogIndeterminate
	}
	return PaneShowsDialog(capture)
}

// presenceLines strips ANSI, the `│ ` gutter and blank lines; it keeps the
// `❯` glyph, which normalizeDialogLine removes.
func presenceLines(capture string) []string {
	var out []string
	for _, raw := range strings.Split(capture, "\n") {
		l := strings.TrimRightFunc(ansiEscape.ReplaceAllString(raw, ""), unicode.IsSpace)
		l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), dialogGutter))
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func isDialogMarker(l string) bool {
	return strings.HasPrefix(l, dialogFooterPrefix) ||
		strings.HasPrefix(l, dialogFooterEsc) ||
		selectedNumberedRow.MatchString(l) ||
		dialogTabBar.MatchString(l)
}

func isRuleLine(l string) bool {
	return len([]rune(l)) >= dialogMinRuleRunes && strings.Trim(l, string(dialogRuleGlyph)) == ""
}

// isComposerPrompt is a bare `❯` or `❯ typed text`, not a selected dialog row.
func isComposerPrompt(l string) bool {
	if l != dialogComposerGlyph && !strings.HasPrefix(l, dialogComposerGlyph+" ") {
		return false
	}
	return !selectedNumberedRow.MatchString(l) && !isDialogMarker(l)
}

// composerAtBottom reports the idle layout: rule, prompt, rule, then at most a
// few status lines, none of them a dialog marker.
func composerAtBottom(lines []string) bool {
	start := max(0, len(lines)-dialogComposerLookback)
	for p := len(lines) - 2; p > start && p >= 1; p-- {
		if !isComposerPrompt(lines[p]) || !isRuleLine(lines[p-1]) || !isRuleLine(lines[p+1]) {
			continue
		}
		for _, l := range lines[p+2:] {
			if isDialogMarker(l) {
				return false
			}
		}
		return true
	}
	return false
}
