package artifacts

import "time"

// SessionArtifactsBlob is the JSON-serialized payload stored in the session_artifacts TEXT column.
type SessionArtifactsBlob struct {
	PRURLs          []string          `json:"pr_urls"`
	CommitSHAs      []string          `json:"commit_shas"`
	ExternalURLs    []string          `json:"external_urls"`
	Commands        []CommandArtifact `json:"commands,omitempty"` // from tool_use bash invocations
	ScanOffsetBytes int64             `json:"scan_offset_bytes"`
	LastScannedAt   time.Time         `json:"last_scanned_at"`
	// ScanState distinguishes "scanned, found nothing" from "never scanned".
	// Empty in blobs written before it existed; read it through State().
	ScanState ScanState `json:"scan_state,omitempty"`
}

// ScanState is the outcome of the last transcript scan for a session. The
// wire values match the proto SessionArtifacts.scan_state strings.
type ScanState string

const (
	ScanStateNotLinked ScanState = "not_linked" // no transcript to scan
	ScanStateScanning  ScanState = "scanning"   // linked, no scan finished yet
	ScanStateNoneFound ScanState = "none_found" // scan finished, nothing extracted
	ScanStateHasData   ScanState = "has_data"   // scan finished, artifacts present
	ScanStateFailed    ScanState = "failed"     // last scan errored; data is the last good set
)

// HasData reports whether any artifact has been extracted.
func (b *SessionArtifactsBlob) HasData() bool {
	return len(b.PRURLs)+len(b.CommitSHAs)+len(b.ExternalURLs)+len(b.Commands) > 0
}

// State returns the recorded scan state, deriving it for blobs written before
// ScanState existed. A published blob is immutable, so this never writes.
func (b *SessionArtifactsBlob) State() ScanState {
	if b.ScanState != "" {
		return b.ScanState
	}
	if b.HasData() {
		return ScanStateHasData
	}
	return ScanStateNoneFound
}

// CommandArtifact records a structured signal extracted from a tool_use bash command.
type CommandArtifact struct {
	Type    string `json:"type"`    // "gh_pr_create", "gh_pr_merge", "git_commit", "package_install"
	Command string `json:"command"` // first 200 chars of the raw command for display
	Detail  string `json:"detail"`  // extracted value: PR title, commit message, pkg name, PR number
}

const maxExternalURLs = 50
const maxCommands = 30
