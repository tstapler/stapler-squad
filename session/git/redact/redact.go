// Package redact strips credentials from text bound for logs, spans and metric labels.
// It imports the standard library only so every layer of the git seam may use it.
package redact

import "regexp"

var (
	// userinfo in any URL: scheme://user:pass@host -> scheme://***@host
	urlUserinfo = regexp.MustCompile(`(://)[^/@\s'"]*@`)
	// Authorization / Proxy-Authorization header values, whatever the scheme.
	authHeader = regexp.MustCompile(`(?i)((?:proxy-)?authorization:\s*)[^\r\n'"]+`)
	// bare GitHub tokens (classic, fine-grained) outside a URL.
	githubToken = regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9_]{6,}|github_pat_[A-Za-z0-9_]{6,})\b`)
)

// Git returns s with URL userinfo, Authorization header values and GitHub tokens replaced by
// "***". It is a minimal first cut of plan Story 1.3.1; the credential-helper output rules
// land with that story.
func Git(s string) string {
	s = urlUserinfo.ReplaceAllString(s, "${1}***@")
	s = authHeader.ReplaceAllString(s, "${1}***")
	return githubToken.ReplaceAllString(s, "***")
}
