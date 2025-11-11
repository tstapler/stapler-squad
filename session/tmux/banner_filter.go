package tmux

import (
	"regexp"
	"strings"
)

// BannerFilter detects and filters tmux status line banners from terminal output
type BannerFilter struct {
	// Patterns that match tmux status banners based on text content
	statusLinePatterns []*regexp.Regexp
	// ANSI escape sequence patterns that indicate status bar formatting
	ansiStatusPatterns []*regexp.Regexp
}

// NewBannerFilter creates a new banner filter with default tmux status patterns
func NewBannerFilter() *BannerFilter {
	return &BannerFilter{
		statusLinePatterns: []*regexp.Regexp{
			// Tmux status line pattern with session, windows, hostname, and timestamp
			// Format: [session-name] window-index:window-name[*-#] "hostname" HH:MM DD-Mon-YY
			// Example: [claudesquad_test-session] 0:claude* "MacBook-Pro" 09:45 17-Oct-25
			// Example: [work] 1:vim- 2:bash* "dev-server" 14:22 17-Oct-25
			regexp.MustCompile(`^\[.+\]\s+(?:\d+:\S+[\*\-\#]?\s+)+".+"\s+\d{2}:\d{2}\s+\d{1,2}-\w{3}-\d{2}$`),

			// Simpler pattern for just the timestamp portion
			// Example: 14:23 5-Jan-24
			regexp.MustCompile(`^\d{2}:\d{2}\s+\d{1,2}-\w{3}-\d{2}$`),

			// Pattern for status line with window indicators (no timestamp or hostname)
			// Example: [0] 1:vim- 2:bash* 3:top#
			regexp.MustCompile(`^\[\d+\]\s+(?:\d+:\S+[\*\-\#]?\s*)+$`),

			// Pattern for status line with multiple windows and timestamp
			// Example: [0] 0:zsh* 1:vim- 2:htop# 14:30 17-Oct-25
			regexp.MustCompile(`^\[\d+\]\s+(?:\d+:\S+[\*\-\#]?\s*)+\d{2}:\d{2}\s+\d{1,2}-\w{3}-\d{2}$`),

			// Pattern for status line components with pipes (session, window, pane info)
			// Example: [session] | main | 16:45
			regexp.MustCompile(`^\[.+\]\s*[\|│]\s*.+[\|│]\s*\d{2}:\d{2}`),
		},
		ansiStatusPatterns: []*regexp.Regexp{
			// Tmux status bar uses reverse video mode (ESC[7m) for highlighting
			// Pattern: ESC[7m ... ESC[27m (reverse video on/off)
			regexp.MustCompile(`\x1b\[7m.*?\x1b\[27m`),

			// Tmux status bar background colors (typically green/blue for status)
			// Pattern: ESC[48;5;Nm for 256-color background
			regexp.MustCompile(`\x1b\[48;5;\d+m`),

			// Combined pattern: reverse video + specific color codes typical of status bars
			// Status bars often use: ESC[7m ESC[38;5;Xm ESC[48;5;Ym
			regexp.MustCompile(`\x1b\[7m\x1b\[38;5;\d+m\x1b\[48;5;\d+m`),

			// Pattern for status bar positioning (move cursor to last line)
			// ESC[H moves to home (used with row number for status bar)
			regexp.MustCompile(`\x1b\[(\d+);1H.*?\x1b\[7m`),

			// Bold + reverse video combination (common in tmux status)
			regexp.MustCompile(`\x1b\[1m\x1b\[7m`),
		},
	}
}

// IsBanner returns true if the given line appears to be a tmux status banner
func (bf *BannerFilter) IsBanner(line string) bool {
	// Empty lines are not banners
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}

	// PRIORITY 1: Check for ANSI escape sequences (most reliable indicator)
	// If the line contains tmux status bar ANSI codes, it's definitely a banner
	for _, pattern := range bf.ansiStatusPatterns {
		if pattern.MatchString(line) {
			return true
		}
	}

	// PRIORITY 2: Check text-based patterns as fallback
	// Strip ANSI codes for text pattern matching
	stripped := stripANSICodes(line)
	trimmedStripped := strings.TrimSpace(stripped)

	for _, pattern := range bf.statusLinePatterns {
		if pattern.MatchString(trimmedStripped) {
			return true
		}
	}

	return false
}

// stripANSICodes removes ANSI escape sequences from a string
func stripANSICodes(s string) string {
	// Pattern to match ANSI escape sequences: ESC[ followed by parameters and command
	ansiPattern := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	return ansiPattern.ReplaceAllString(s, "")
}

// FilterBanners removes tmux status banners from a slice of lines
// Returns the filtered lines and a count of how many banners were removed
func (bf *BannerFilter) FilterBanners(lines []string) ([]string, int) {
	filtered := make([]string, 0, len(lines))
	bannersRemoved := 0

	for _, line := range lines {
		if !bf.IsBanner(line) {
			filtered = append(filtered, line)
		} else {
			bannersRemoved++
		}
	}

	return filtered, bannersRemoved
}

// FilterBannersFromText takes a multi-line string and removes banner lines
// Returns the filtered text and a count of banners removed
func (bf *BannerFilter) FilterBannersFromText(text string) (string, int) {
	lines := strings.Split(text, "\n")
	filtered, count := bf.FilterBanners(lines)
	return strings.Join(filtered, "\n"), count
}

// HasMeaningfulContent returns true if the text has content beyond just banners
// Excludes the last line (tmux status bar) from meaningful content detection
func (bf *BannerFilter) HasMeaningfulContent(text string) bool {
	lines := strings.Split(text, "\n")

	// Exclude the last line (tmux status bar with timestamp)
	// If we have at least one line, check all but the last
	numLinesToCheck := len(lines)
	if numLinesToCheck > 0 {
		numLinesToCheck-- // Exclude last line
	}

	for i := 0; i < numLinesToCheck; i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		// Skip empty lines
		if trimmed == "" {
			continue
		}
		// If we find a non-banner line with content, we have meaningful output
		if !bf.IsBanner(trimmed) {
			return true
		}
	}

	return false
}
