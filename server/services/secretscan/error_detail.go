package secretscan

import "regexp"

// errorDetailSecretPattern matches common credential shapes that could ride along in a
// wrapped exec/network error: vendor token prefixes, JWTs, bearer/basic auth, and
// key/value or --flag secrets with =, :, quoted-JSON or (for --flags) space separators.
var errorDetailSecretPattern = regexp.MustCompile(`(?i)(` +
	`\bsk-[a-z0-9_-]{8,}` +
	`|\b(?:gh[pousr]|github_pat)_[a-z0-9_]{20,}` +
	`|\bxox[abpr]-[a-z0-9-]{8,}` +
	`|\b(?:akia|asia)[a-z0-9]{16}\b` +
	`|\beyj[a-z0-9_-]+\.[a-z0-9_-]+\.[a-z0-9_-]*` +
	`|\b(?:bearer|basic)\s+[a-z0-9._~+/=-]{8,}` +
	`|(?:api[_-]?key|token|secret|passw(?:or)?d|authorization)\b["']?\s*[:=]\s*["']?(?:(?:bearer|basic)\s+)?[^\s"',;&]+` +
	`|--(?:api[_-]?key|token|secret|passw(?:or)?d)\s+[^\s"',;&]+` +
	`)`)

// errorDetailURLUserinfoPattern captures a URL's scheme so only its user:pass@ part is masked.
var errorDetailURLUserinfoPattern = regexp.MustCompile(`(?i)(\b[a-z][a-z0-9+.-]*://)[^\s/@]+@`)

// RedactErrorDetail masks credential-shaped substrings. Best-effort: error_detail outlives
// log rotation and is shown in the UI, so it must not become a durable secret store.
func RedactErrorDetail(s string) string {
	s = errorDetailURLUserinfoPattern.ReplaceAllString(s, "${1}[REDACTED]@")
	return errorDetailSecretPattern.ReplaceAllString(s, "[REDACTED]")
}
