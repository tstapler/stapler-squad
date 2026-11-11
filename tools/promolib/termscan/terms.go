// Package termscan finds banned trademark terms in tracked source and published
// content. Terms are assembled from fragments so this source never contains the
// literals it bans.
package termscan

import (
	"regexp"
	"strings"
)

// Term is one banned term.
type Term struct {
	// Text is the matched term as reported to the user.
	Text string
	// Rationale says why the term is banned.
	Rationale string
	// PublishedOnly restricts the term to published paths (see IsPublishedPath).
	PublishedOnly bool
	re            *regexp.Regexp
}

// Matches reports whether line contains the term.
func (t Term) Matches(line string) bool { return t.re.MatchString(line) }

func join(parts ...string) string { return strings.Join(parts, "") }

func substring(rationale string, parts ...string) Term {
	text := join(parts...)
	return Term{Text: text, Rationale: rationale, re: regexp.MustCompile("(?i)" + regexp.QuoteMeta(text))}
}

// BannedTermSet returns the compiled banned terms. It is a function, not a
// package variable, because gochecknoglobals covers this package.
func BannedTermSet() []Term {
	const brand = "trademarked franchise name"
	const owner = "trademark owner name"
	const unit = "franchise faction or unit name"
	return []Term{
		substring(brand+" (compact form; also catches glued identifiers)", "wh", "40", "k"),
		substring(brand, "war", "hammer"),
		substring(brand+" (hyphenated)", "wh", "-", "40", "k"),
		substring(brand+" (snake case)", "wh", "_", "40", "k"),
		substring(brand+" (spaced)", "wh", " ", "40", "k"),
		substring(owner, "games", " ", "work", "shop"),
		substring(unit, "space", " ", "mar", "ine"),
		substring(unit, "ad", "eptus"),
		substring(unit, "asta", "rtes"),
		substring(owner+" (publisher imprint)", "black", " ", "lib", "rary"),
		{
			Text:          join("40", "k"),
			Rationale:     "short franchise shorthand; word-bounded and applied only under published paths to avoid noise like \"a 40k budget\" in internal code",
			PublishedOnly: true,
			re:            regexp.MustCompile(`(?i)\b` + join("40", "k") + `\b`),
		},
	}
}

// IsPublishedPath reports whether path is user-visible content, where the
// short word-bounded term applies.
func IsPublishedPath(path string) bool {
	if path == "README.md" {
		return true
	}
	for _, prefix := range []string{"website/", "web-app/src/", "assets/", "docs/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
