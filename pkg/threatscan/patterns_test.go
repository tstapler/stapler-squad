package threatscan

import (
	"slices"
	"testing"
)

func TestScope_String_should_ReturnHumanReadableName_When_Called(t *testing.T) {
	for scope, want := range map[Scope]string{ScopeStrict: "strict", ScopeContext: "context", ScopeAll: "all", Scope(99): "unknown"} {
		if got := scope.String(); got != want {
			t.Errorf("Scope(%d).String() = %q, want %q", int(scope), got, want)
		}
	}
}

// goldenSecretSources returns the original secretPatterns sources, hardcoded from
// session/backlog_review.go (git show 35218a2a2:session/backlog_review.go), independent of patterns.go.
func goldenSecretSources() map[string]string {
	return map[string]string{
		"aws_access_key_id": `(?i)aws_access_key_id`,
		"AKIA_key":          `AKIA[0-9A-Z]{16}`,
		"private_key_pem":   `-----BEGIN .{0,30}PRIVATE KEY-----`,
		"github_pat":        `ghp_[a-zA-Z0-9]{36}`,
		"openai_key":        `sk-[a-zA-Z0-9]{48}`,
		"stripe_secret_key": `sk_live_[a-zA-Z0-9]{24,}`,
		"slack_token":       `xox[baprs]-[a-zA-Z0-9-]+`,
		"npm_token":         `npm_[a-zA-Z0-9]{36}`,
		"sendgrid_key":      `SG\.[a-zA-Z0-9_-]{22}\.[a-zA-Z0-9_-]{43}`,
		"twilio_sid":        `AC[a-f0-9]{32}`,
		"generic_bearer":    `(?i)Authorization:\s*Bearer\s+[a-zA-Z0-9_.+/=-]{20,}`,
		"database_url":      `(?i)(postgres|mysql|mongodb|redis)://[^@\s]+:[^@\s]+@`,
	}
}

func TestDefaultPatterns_should_MatchGoldenSecretPatternSourceStrings_When_Compared(t *testing.T) {
	got := map[string]string{}
	for _, p := range DefaultPatterns() {
		got[p.id] = p.re.String()
	}
	for id, src := range goldenSecretSources() {
		if got[id] != src {
			t.Errorf("pattern %s source = %q, want golden %q", id, got[id], src)
		}
	}
	if len(goldenSecretSources()) != 12 {
		t.Fatalf("golden list has %d entries, want 12", len(goldenSecretSources()))
	}
}

func TestDefaultPatterns_should_HaveUniqueIDsAndScopeTags_When_Called(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range DefaultPatterns() {
		if seen[p.id] {
			t.Errorf("duplicate pattern id %s", p.id)
		}
		seen[p.id] = true
		if !slices.Contains(p.scopes, ScopeAll) {
			t.Errorf("pattern %s is not part of ScopeAll", p.id)
		}
	}
}
