package tmux

import (
	"regexp"
	"strings"
)

// Compiled once at init; shared across all BannerFilter instances.
var (
	// statusLinePatterns match tmux status banners based on text content.
	statusLinePatterns = []*regexp.Regexp{
		// [session-name] window-index:name[*-#] "hostname" HH:MM DD-Mon-YY
		regexp.MustCompile(`^\[.+\]\s+(?:\d+:\S+[\*\-\#]?\s+)+".+"\s+\d{2}:\d{2}\s+\d{1,2}-\w{3}-\d{2}$`),
		// 14:23 5-Jan-24
		regexp.MustCompile(`^\d{2}:\d{2}\s+\d{1,2}-\w{3}-\d{2}$`),
		// [0] 1:vim- 2:bash* 3:top#
		regexp.MustCompile(`^\[\d+\]\s+(?:\d+:\S+[\*\-\#]?\s*)+$`),
		// [0] 0:zsh* 1:vim- 2:htop# 14:30 17-Oct-25
		regexp.MustCompile(`^\[\d+\]\s+(?:\d+:\S+[\*\-\#]?\s*)+\d{2}:\d{2}\s+\d{1,2}-\w{3}-\d{2}$`),
		// [session] | main | 16:45
		regexp.MustCompile(`^\[.+\]\s*[\|│]\s*.+[\|│]\s*\d{2}:\d{2}`),
	}

	// ansiStatusPatterns match ANSI escape sequences used by tmux status bars.
	ansiStatusPatterns = []*regexp.Regexp{
		// ESC[7m ... ESC[27m (reverse video)
		regexp.MustCompile(`\x1b\[7m.*?\x1b\[27m`),
		// ESC[48;5;Nm (256-colour background)
		regexp.MustCompile(`\x1b\[48;5;\d+m`),
		// reverse video + 256-colour pair
		regexp.MustCompile(`\x1b\[7m\x1b\[38;5;\d+m\x1b\[48;5;\d+m`),
		// cursor-to-row + reverse video
		regexp.MustCompile(`\x1b\[(\d+);1H.*?\x1b\[7m`),
		// bold + reverse video
		regexp.MustCompile(`\x1b\[1m\x1b\[7m`),
	}

	// stripANSIRe matches ANSI escape sequences for stripping.
	stripANSIRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
)

// BannerFilter detects and filters tmux status line banners from terminal output
type BannerFilter struct {
	statusLinePatterns []*regexp.Regexp
	ansiStatusPatterns []*regexp.Regexp
}

// NewBannerFilter creates a new banner filter with default tmux status patterns
func NewBannerFilter() *BannerFilter {
	return &BannerFilter{
		statusLinePatterns: statusLinePatterns,
		ansiStatusPatterns: ansiStatusPatterns,
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
	return stripANSIRe.ReplaceAllString(s, "")
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
// The last line may be a tmux status bar, but only exclude it if it matches status bar patterns
func (bf *BannerFilter) HasMeaningfulContent(text string) bool {
	lines := strings.Split(text, "\n")

	// Determine how many lines to check
	// Only exclude the last line if it actually matches a banner pattern
	numLinesToCheck := len(lines)
	if numLinesToCheck > 1 {
		// Check if the last line is a banner (likely tmux status bar)
		lastLine := strings.TrimSpace(lines[len(lines)-1])
		if lastLine != "" && bf.IsBanner(lastLine) {
			numLinesToCheck-- // Exclude last line only if it's a banner
		}
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
