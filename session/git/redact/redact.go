// Package redact strips credentials from text bound for logs, spans and metric labels.
// It imports the standard library only so every layer of the git seam may use it.
package redact

import "regexp"

var (
	// userinfo in any URL: scheme://user:pass@host -> scheme://***@host. It runs through the
	// LAST '@' of the token so a password containing '@' or '/' is hidden entirely.
	urlUserinfo = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^\s'"]*@`)
	// scp-style user:token@host:path.
	scpUserinfo = regexp.MustCompile(`\b[A-Za-z0-9._~-]+:[^\s'"/]+@`)
	// Authorization / Proxy-Authorization header values, whatever the scheme.
	authHeader = regexp.MustCompile(`(?i)((?:proxy-)?authorization:\s*)[^\r\n'"]+`)
	// credential-helper protocol lines (git credential fill/get output): password=..., etc.
	helperSecret = regexp.MustCompile(`(?im)^(\s*(?:password|oauth_refresh_token|authtoken|bearer)\s*=)[^\r\n]*`)
	// bare GitHub tokens (classic, fine-grained) outside a URL.
	githubToken = regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9_]{6,}|github_pat_[A-Za-z0-9_]{6,})\b`)
)

// Git returns s with URL userinfo, Authorization header values, credential-helper secret lines and GitHub
// tokens replaced by "***".
func Git(s string) string {
	s = urlUserinfo.ReplaceAllString(s, "${1}***@")
	s = scpUserinfo.ReplaceAllString(s, "***@")
	s = authHeader.ReplaceAllString(s, "${1}***")
	s = helperSecret.ReplaceAllString(s, "${1}***")
	return githubToken.ReplaceAllString(s, "***")
}
