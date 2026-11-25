package secretscan

import "testing"

// The sentinels are persisted (analytics) and shown to models (prompts), so
// their literal values are part of the contract.
func TestRedactionSentinels_LiteralValues(t *testing.T) {
	t.Parallel()
	if RedactedSecret != "[REDACTED: secret detected]" {
		t.Errorf("RedactedSecret = %q", RedactedSecret)
	}
	if RedactedPrompt != "[REDACTED]" {
		t.Errorf("RedactedPrompt = %q", RedactedPrompt)
	}
}
