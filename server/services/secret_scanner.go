package services

import (
	"fmt"
	"regexp"
)

// secretPattern describes a known secret format.
type secretPattern struct {
	Name    string
	Pattern *regexp.Regexp
}

// secretPatterns is the list of patterns checked by ScanForSecrets.
// These are intentionally conservative to minimize false positives.
var secretPatterns = []secretPattern{
	// GitHub personal access tokens
	{Name: "GitHub personal access token", Pattern: regexp.MustCompile(`\bghp_[A-Za-z0-9]{36}\b`)},
	{Name: "GitHub OAuth token", Pattern: regexp.MustCompile(`\bgho_[A-Za-z0-9]{36}\b`)},
	{Name: "GitHub Actions token", Pattern: regexp.MustCompile(`\bghs_[A-Za-z0-9]{36}\b`)},
	// AWS credentials
	{Name: "AWS access key ID", Pattern: regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	// OpenAI / Anthropic
	{Name: "OpenAI API key", Pattern: regexp.MustCompile(`\bsk-[A-Za-z0-9]{32,48}\b`)},
	{Name: "Anthropic API key", Pattern: regexp.MustCompile(`\bsk-ant-[A-Za-z0-9\-]{20,}\b`)},
	// Slack tokens
	{Name: "Slack token", Pattern: regexp.MustCompile(`\bxox[boas]-[0-9A-Za-z\-]+\b`)},
	// JWT tokens (three base64url-separated segments, starts with eyJ which is base64 for `{"`)
	{Name: "JWT token", Pattern: regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}`)},
	// PEM private key headers
	{Name: "PEM private key", Pattern: regexp.MustCompile(`-----BEGIN[A-Z ]+PRIVATE KEY-----`)},
	// Generic inline secret assignments: SECRET=value, PASSWORD=value, PGPASSWORD=value, etc.
	// The \b word boundary is intentionally omitted so that env vars like PGPASSWORD or
	// MYSQL_ROOT_PASSWORD (where the keyword is a suffix) are also caught.
	{Name: "Inline secret env var", Pattern: regexp.MustCompile(`(?i)(?:password|passwd|secret|api_?key|auth_?token|access_?token|private_?key)\s*=\s*\S{8,}`)},
	// Well-known database / service credential env vars with keyword as a suffix (PGPASSWORD, MYSQL_PWD, …)
	{Name: "Database credential env var", Pattern: regexp.MustCompile(`(?i)(?:pg|mysql|mariadb|postgres|redis|mongo(?:db)?|db|database|rabbitmq|smtp|mail)(?:_root)?(?:_?password|_?passwd|_?pwd|_pass)\s*=\s*\S{4,}`)},
	// CLI flags: --password=value --token=value etc.
	{Name: "Inline secret CLI flag", Pattern: regexp.MustCompile(`(?i)--(?:password|passwd|secret|api-?key|auth-?token|access-?token|private-?key)\s*(?:=|\s+)\S{8,}`)},
}

// SecretScanResult holds the result of scanning for secrets.
type SecretScanResult struct {
	Found       bool
	PatternName string // name of the first matching pattern
}

// ScanForSecrets checks text for known secret patterns.
// Returns the first match found, or an empty result if none.
// Only the first 4096 bytes are scanned to bound performance on very long commands.
func ScanForSecrets(text string) SecretScanResult {
	if len(text) > 4096 {
		text = text[:4096]
	}
	for _, p := range secretPatterns {
		if p.Pattern.MatchString(text) {
			return SecretScanResult{Found: true, PatternName: p.Name}
		}
	}
	return SecretScanResult{}
}

// FormatSecretDenyMessage returns a user-facing denial message for a secret scan hit.
func FormatSecretDenyMessage(patternName string) string {
	return fmt.Sprintf(
		"Blocked: command appears to contain a plaintext secret (%s). "+
			"Use environment variables or a secrets manager instead of passing secrets directly in command arguments.",
		patternName,
	)
}
