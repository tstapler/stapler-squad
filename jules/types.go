// Package jules is a self-contained gateway over the Jules alpha API
// (https://jules.googleapis.com/v1alpha). It imports nothing from session/
// or server/ (enforced by TestJulesPackage_should_NotImportSessionOrServer_When_DepsListed
// in client_test.go) so wire-format churn from an alpha API stays confined
// to this one directory.
package jules

import (
	"encoding/json"
	"fmt"
	"strings"
)

// JulesSourceName is a Jules API resource name identifying a registered
// GitHub source, wire format "sources/github-{owner}-{repo}".
type JulesSourceName string

// JulesSessionName is a Jules API resource name identifying a session, wire
// format "sessions/{id}".
type JulesSessionName string

// GitHubBranchRef is a branch name that must already exist on the GitHub
// remote backing a JulesSourceName — Jules cannot target a local worktree
// (research/stack.md §Sources).
type GitHubBranchRef string

// JulesAPIKey is a Jules API key, sent via the x-goog-api-key header. It
// never prints in full: String() always returns a redacted placeholder, and
// the underlying value is reachable only through the unexported reveal(),
// called solely by newRequest in client.go.
type JulesAPIKey string

// ParseJulesSourceName validates that s carries the "sources/" resource-name
// prefix Jules requires.
func ParseJulesSourceName(s string) (JulesSourceName, error) {
	if s == "" {
		return "", fmt.Errorf("jules: source name must not be empty")
	}
	if !strings.HasPrefix(s, "sources/") {
		return "", fmt.Errorf("jules: source name %q must have a sources/ prefix", s)
	}
	return JulesSourceName(s), nil
}

// ParseJulesSessionName validates that s carries the "sessions/"
// resource-name prefix Jules requires.
func ParseJulesSessionName(s string) (JulesSessionName, error) {
	if s == "" {
		return "", fmt.Errorf("jules: session name must not be empty")
	}
	if !strings.HasPrefix(s, "sessions/") {
		return "", fmt.Errorf("jules: session name %q must have a sessions/ prefix", s)
	}
	return JulesSessionName(s), nil
}

// ParseGitHubBranchRef validates that s is a non-empty branch name.
func ParseGitHubBranchRef(s string) (GitHubBranchRef, error) {
	if s == "" {
		return "", fmt.Errorf("jules: branch ref must not be empty")
	}
	return GitHubBranchRef(s), nil
}

// ParseJulesAPIKey validates that s is a non-empty API key.
func ParseJulesAPIKey(s string) (JulesAPIKey, error) {
	if s == "" {
		return "", fmt.Errorf("jules: API key must not be empty")
	}
	return JulesAPIKey(s), nil
}

// String never reveals the key value — it always returns a redacted
// placeholder, so both %v and %s formatting (and any accidental logging)
// are safe.
func (k JulesAPIKey) String() string {
	return "jules-api-key(redacted)"
}

// reveal returns the underlying key text. Only newRequest (client.go) may
// call this, to populate the x-goog-api-key header.
func (k JulesAPIKey) reveal() string {
	return string(k)
}

// JulesSessionState is a closed sum type over the Jules session lifecycle,
// plus an Unknown variant so a wire value this package has never seen (an
// alpha API adding a new state) fails safely — distinguishable via
// IsKnown() — instead of silently aliasing a known state. The underlying
// string IS the raw wire value (rather than a private field on a struct),
// so the known values can be plain `const`s — the repo's gochecknoglobals
// lint rule (.golangci.yml) forbids new packages from adding package-level
// `var`s, which a struct-typed sum type would have required here.
type JulesSessionState string

// Known JulesSessionState values, matching the wire states documented in
// research/stack.md §Sessions. JulesStateUnknown is the zero value: parsing
// an unrecognized wire value does NOT collapse to this exact constant — it
// returns a JulesSessionState carrying the original raw text (see
// ParseJulesSessionState), so IsKnown()/Raw() can still report what was
// actually on the wire. Use IsKnown()==false, not equality against
// JulesStateUnknown, to test for "an unrecognized state".
const (
	JulesStateQueued               JulesSessionState = "QUEUED"
	JulesStatePlanning             JulesSessionState = "PLANNING"
	JulesStateAwaitingPlanApproval JulesSessionState = "AWAITING_PLAN_APPROVAL"
	JulesStateInProgress           JulesSessionState = "IN_PROGRESS"
	JulesStateCompleted            JulesSessionState = "COMPLETED"
	JulesStateFailed               JulesSessionState = "FAILED"
	JulesStateUnknown              JulesSessionState = ""
)

// ParseJulesSessionState never errors: it returns raw as a JulesSessionState
// unconditionally, so Raw() below always yields back exactly what was seen
// on the wire, known or not.
func ParseJulesSessionState(raw string) JulesSessionState {
	return JulesSessionState(raw)
}

// Raw returns the original wire value this state was parsed from (empty for
// the zero value JulesStateUnknown).
func (s JulesSessionState) Raw() string {
	return string(s)
}

// IsKnown reports whether s matched one of the states this package
// recognizes at compile time.
func (s JulesSessionState) IsKnown() bool {
	switch s {
	case JulesStateQueued, JulesStatePlanning, JulesStateAwaitingPlanApproval,
		JulesStateInProgress, JulesStateCompleted, JulesStateFailed:
		return true
	case JulesStateUnknown:
		return false
	default:
		// Any raw wire value outside the known set above, including
		// JulesStateUnknown's own "" — reachable when the wire genuinely
		// sends an unrecognized (non-empty) state string.
		return false
	}
}

// IsTerminal reports whether this state indicates the Jules session has
// finished executing and will not transition further. True only for
// COMPLETED and FAILED.
func (s JulesSessionState) IsTerminal() bool {
	return s == JulesStateCompleted || s == JulesStateFailed
}

// String renders the raw wire value, or "UNKNOWN" when s is the zero value
// (JulesStateUnknown).
func (s JulesSessionState) String() string {
	if s == "" {
		return "UNKNOWN"
	}
	return string(s)
}

// UnmarshalJSON decodes a JSON string into a JulesSessionState via
// ParseJulesSessionState, which never errors on an unrecognized value.
func (s *JulesSessionState) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("jules: decoding session state: %w", err)
	}
	*s = ParseJulesSessionState(raw)
	return nil
}
