package secretscan

// Redaction sentinel constants used when suppressing secrets before storage or
// prompt injection. Use these instead of inline string literals so grep-based
// audits can find every redaction site.
const (
	// RedactedSecret is written to analytics storage when a secret is detected
	// in a command and must not be persisted.
	RedactedSecret = "[REDACTED: secret detected]" // #nosec G101 -- redaction sentinel string, not a credential
	// RedactedPrompt is inserted into AI prompts in place of a command that
	// triggered the secret scanner (defense-in-depth, per FLAG-1).
	RedactedPrompt = "[REDACTED]"
)
