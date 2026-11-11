// Package codeowners parses a CODEOWNERS file far enough to answer one question:
// which individual users own a path. Teams are reported separately because
// GITHUB_TOKEN cannot resolve team membership.
package codeowners

import (
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Rule is one CODEOWNERS line.
type Rule struct {
	Pattern string
	Owners  []string
}

// File is a parsed CODEOWNERS file; the last matching rule wins.
type File struct{ rules []Rule }

// Parse reads CODEOWNERS text. Blank lines and comments are skipped.
func Parse(data []byte) File {
	var f File
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if i := indexComment(fields); i >= 0 {
			fields = fields[:i]
		}
		if len(fields) == 0 {
			continue
		}
		f.rules = append(f.rules, Rule{Pattern: fields[0], Owners: fields[1:]})
	}
	return f
}

func indexComment(fields []string) int {
	for i, f := range fields {
		if strings.HasPrefix(f, "#") {
			return i
		}
	}
	return -1
}

// Owners returns the owners of path from the last matching rule (nil if none).
func (f File) Owners(path string) []string {
	var owners []string
	for _, r := range f.rules {
		if matches(r.Pattern, path) {
			owners = r.Owners
		}
	}
	return owners
}

func matches(pattern, path string) bool {
	anchored := strings.HasPrefix(pattern, "/") || strings.Contains(strings.TrimSuffix(pattern, "/"), "/")
	pattern = strings.TrimPrefix(pattern, "/")
	if strings.HasSuffix(pattern, "/") {
		pattern += "**"
	}
	if !anchored {
		pattern = "**/" + pattern
	}
	ok, err := doublestar.Match(pattern, path)
	if err != nil {
		return false
	}
	if !ok && !strings.HasSuffix(pattern, "**") {
		// a pattern naming a directory also owns everything beneath it
		ok, _ = doublestar.Match(pattern+"/**", path)
	}
	return ok
}

// Split separates owners into individual user logins (lowercased, without the
// leading "@") and skipped entries (teams "@org/team" and email addresses).
func Split(owners []string) (users, skipped []string) {
	for _, o := range owners {
		switch {
		case strings.HasPrefix(o, "@") && !strings.Contains(o, "/"):
			users = append(users, strings.ToLower(strings.TrimPrefix(o, "@")))
		default:
			skipped = append(skipped, o)
		}
	}
	return users, skipped
}
