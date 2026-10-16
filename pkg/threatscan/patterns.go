package threatscan

import "regexp"

// Scope selects scan breadth.
type Scope int

const (
	// ScopeStrict is high-confidence secrets; used by session.RunPreGateSecurityCheck.
	ScopeStrict Scope = iota
	// ScopeContext is prompt-injection, HTML-injection, role-play and exfiltration signals.
	ScopeContext
	// ScopeAll is every pattern.
	ScopeAll
)

func (s Scope) String() string {
	switch s {
	case ScopeStrict:
		return "strict"
	case ScopeContext:
		return "context"
	case ScopeAll:
		return "all"
	default:
		return "unknown"
	}
}

type pattern struct {
	id     string
	scopes []Scope
	re     *regexp.Regexp
}

// DefaultPatterns returns the ordered pattern registry. It is a function, not a var,
// to satisfy gochecknoglobals; Scan is not a hot path so recompiling is fine.
func DefaultPatterns() []pattern {
	strict := []Scope{ScopeStrict, ScopeAll}
	ctx := []Scope{ScopeContext, ScopeAll}
	return []pattern{
		// Secrets, migrated verbatim from session/backlog_review.go.
		{id: "aws_access_key_id", scopes: strict, re: regexp.MustCompile(`(?i)aws_access_key_id`)},
		{id: "AKIA_key", scopes: strict, re: regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
		{id: "private_key_pem", scopes: strict, re: regexp.MustCompile(`-----BEGIN .{0,30}PRIVATE KEY-----`)},
		{id: "github_pat", scopes: strict, re: regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`)},
		{id: "openai_key", scopes: strict, re: regexp.MustCompile(`sk-[a-zA-Z0-9]{48}`)},
		{id: "stripe_secret_key", scopes: strict, re: regexp.MustCompile(`sk_live_[a-zA-Z0-9]{24,}`)},
		{id: "slack_token", scopes: strict, re: regexp.MustCompile(`xox[baprs]-[a-zA-Z0-9-]+`)},
		{id: "npm_token", scopes: strict, re: regexp.MustCompile(`npm_[a-zA-Z0-9]{36}`)},
		{id: "sendgrid_key", scopes: strict, re: regexp.MustCompile(`SG\.[a-zA-Z0-9_-]{22}\.[a-zA-Z0-9_-]{43}`)},
		{id: "twilio_sid", scopes: strict, re: regexp.MustCompile(`AC[a-f0-9]{32}`)},
		{id: "generic_bearer", scopes: strict, re: regexp.MustCompile(`(?i)Authorization:\s*Bearer\s+[a-zA-Z0-9_.+/=-]{20,}`)},
		{id: "database_url", scopes: strict, re: regexp.MustCompile(`(?i)(postgres|mysql|mongodb|redis)://[^@\s]+:[^@\s]+@`)},

		// Dormant ScopeContext patterns (not wired into any live caller; see doc.go).
		// Filler gaps are bounded ({0,N} words) so filler-word bypasses match without `.*`.
		// Accepted trade-off: the bound that catches "ignore all the previous silly
		// instructions" also matches structurally identical benign prose such as
		// "ignore the previous draft's instructions".
		{id: "prompt_injection_ignore_instructions", scopes: ctx, re: regexp.MustCompile(`(?i)ignore\s+(?:\S+\s+){0,3}previous\s+(?:\S+\s+){0,3}instructions?`)},
		{id: "prompt_injection_system_override", scopes: ctx, re: regexp.MustCompile(`(?i)(override|disregard|bypass)\s+(?:\S+\s+){0,3}system\s+prompt`)},
		{id: "html_hidden_display_none", scopes: ctx, re: regexp.MustCompile(`(?i)display:\s*none`)},
		// (?s) and lazy match: hidden-instruction comments often span lines.
		{id: "html_comment_injection", scopes: ctx, re: regexp.MustCompile(`(?s)<!--.*?-->`)},
		{id: "roleplay_identity_hijack", scopes: ctx, re: regexp.MustCompile(`(?i)pretend\s+(?:\S+\s+){0,4}(you\s+are|to\s+be)\s+(?:\S+\s+){0,4}unrestricted`)},
		{id: "exfiltration_send_to_http", scopes: ctx, re: regexp.MustCompile(`(?i)send\s+(?:\S+\s+){0,5}to\s+(?:\S+\s+){0,3}https?://`)},
		{id: "exfiltration_curl_webhook", scopes: ctx, re: regexp.MustCompile(`(?i)curl\s+(?:\S+\s+){0,6}webhook`)},
	}
}
