package detection

import "strings"

// PTYNormalizer handles ANSI stripping and carriage-return collapsing for PTY output.
// It is a stateless struct; all methods are pure transformations.
type PTYNormalizer struct{}

// Normalize strips ANSI escape sequences and collapses CR-overwritten segments.
// Equivalent to stripANSI(collapseCarriageReturns(content)).
func (n PTYNormalizer) Normalize(content string) string {
	return stripANSI(collapseCarriageReturns(content))
}

// SplitLines splits normalized content into non-blank lines.
func (n PTYNormalizer) SplitLines(content string) []string {
	lines := strings.Split(content, "\n")
	out := lines[:0]
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
