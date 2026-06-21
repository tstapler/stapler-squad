package detection

import (
	"testing"
)

// TestClaude_AsterismActive_SingleLine verifies that spinner-frame + Verb... lines are
// classified as StatusExecuting across all known frame characters.
func TestClaude_AsterismActive_SingleLine(t *testing.T) {
	sd := NewStatusDetector()
	cases := []struct {
		name  string
		input string
	}{
		// Modern ✻ (U+273B ASTERISM) — primary current frame
		{"asterism basic", "✻ Perambulating..."},
		{"asterism with timing", "✻ Perambulating... (1h 5m 37s · ↑ 5.4k tokens)"},
		{"asterism unicode ellipsis", "✻ Cogitating…"},
		// Other macOS bounce-cycle frames: · ✢ ✳ ✶ ✻ ✽
		{"middle dot frame", "· Herding…"},
		{"four teardrop frame ✢", "✢ Spelunking…"},
		{"eight spoked asterisk ✳", "✳ Ruminating…"},
		{"six pointed star ✶", "✶ Wandering…"},
		{"eight pointed pinwheel ✽", "✽ Tinkering…"},
		// Reduced-motion static frame ● (U+25CF BLACK CIRCLE)
		{"reduced motion frame", "● Working…"},
		// Legacy * (U+002A ASTERISK)
		{"asterisk legacy", "* Moonwalking..."},
		// Verbs with accented chars (Go RE2 \w misses é, è, etc.)
		{"accented flambe", "✻ Flambéing…"},
		{"accented saute", "✻ Sautéing…"},
		// Hyphenated verbs
		{"hyphenated dilly", "✻ Dilly-dallying…"},
		{"hyphenated razzle", "✻ Razzle-dazzling…"},
		// Apostrophe-truncated
		{"apostrophe beboppin", "✻ Beboppin'…"},
		// Extended thinking suffix — ellipsis is on the verb, suffix is parenthetical
		{"thinking some more suffix", "✻ Moonwalking… (43s · thinking some more)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sd.Detect([]byte(tc.input))
			if got != StatusExecuting && got != StatusProcessing {
				t.Errorf("Detect(%q) = %s, want StatusExecuting or StatusProcessing", tc.input, got)
			}
		})
	}
}

// TestClaude_AsteriskActive_NoRegression verifies the existing * prefix still works.
func TestClaude_AsteriskActive_NoRegression(t *testing.T) {
	sd := NewStatusDetector()
	cases := []string{
		"* Moonwalking... (4m 18s · ↓ 2.0k tokens · thinking)",
		"* Ebbing...",
	}
	for _, input := range cases {
		got := sd.Detect([]byte(input))
		if got != StatusExecuting && got != StatusProcessing {
			t.Errorf("Detect(%q) = %s, want StatusExecuting (regression)", input, got)
		}
	}
}

// TestClaude_SpinnerFrame_NoFalsePositive verifies that spinner-like chars in
// non-active contexts don't trigger the pattern.
func TestClaude_SpinnerFrame_NoFalsePositive(t *testing.T) {
	sd := NewStatusDetector()
	cases := []struct {
		name  string
		input string
	}{
		// · as separator mid-line in timing string — must NOT match
		{"middle dot separator in timing", "(8m 39s · ↓ 834 tokens)"},
		// lowercase after frame — real spinner verbs are always capitalized
		{"lowercase after frame", "✻ perambulating..."},
		// ◉ (FISHEYE) is completion-only — must NOT match Active
		{"fisheye active line", "◉ Working…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sd.Detect([]byte(tc.input))
			if got == StatusExecuting {
				t.Errorf("Detect(%q) = StatusExecuting, expected no match (false positive)", tc.input)
			}
		})
	}
}

// TestClaude_AsterismCompletion_IsSuccess verifies that ✻ PastTenseVerb for N → StatusSuccess (AC-2).
func TestClaude_AsterismCompletion_IsSuccess(t *testing.T) {
	sd := NewStatusDetector()
	cases := []string{
		"✻ Perambulated for 1h 5m",
		"✻ Synthesized for 30s",
		"◉ Baked for 10s",
	}
	for _, input := range cases {
		got := sd.Detect([]byte(input))
		if got != StatusSuccess {
			t.Errorf("Detect(%q) = %s, want StatusSuccess", input, got)
		}
	}
}

// TestClaude_AsterismCompletion_NotActive verifies completion lines are never Active (AC-3 variant).
func TestClaude_AsterismCompletion_NotActive(t *testing.T) {
	sd := NewStatusDetector()
	cases := []string{
		"✻ Perambulated for 1h 5m",
		"✻ Synthesized for 30s",
	}
	for _, input := range cases {
		got := sd.Detect([]byte(input))
		if got == StatusExecuting {
			t.Errorf("Detect(%q) = StatusExecuting, completion line must not be Active", input)
		}
	}
}

// TestClaude_ScrollbackFalsePositive_ActiveThenIdle verifies that old spinner lines
// in scrollback don't override a current idle prompt (AC-3).
func TestClaude_ScrollbackFalsePositive_ActiveThenIdle(t *testing.T) {
	sd := NewStatusDetector()
	lines := []string{
		"✻ Perambulating... (5m 12s · ↑ 3.2k tokens)",
		" └ some tool output",
		"",
		"✻ Perambulated for 5m 12s",
		"",
		"> ▌",
		"? for shortcuts",
	}
	got, _ := sd.DetectWithContextFromLines(lines)
	if got != StatusIdle && got != StatusReady {
		t.Errorf("DetectWithContextFromLines with stale scrollback: got %s, want StatusIdle or StatusReady", got)
	}
}

// TestClaude_AsterismPattern_NoAiderFalsePositive confirms the ✻ pattern doesn't
// produce unexpected results for Aider-style bullet output.
// The real regression guard is snapshot_test.go/aider_active.txt.
func TestClaude_AsterismPattern_NoAiderFalsePositive(t *testing.T) {
	sd := NewStatusDetector()
	aiderOutputs := []string{
		" Updating /path/to/file...",
		" Applying patch to foo.go...",
		" Installing dependencies...",
	}
	for _, input := range aiderOutputs {
		// These may or may not match Active - the snapshot test is the real guard.
		// This test just ensures the detector doesn't panic on such inputs.
		_ = sd.Detect([]byte(input))
	}
}
