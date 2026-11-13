package domain

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
	// ScanVersion is the extractor version that last scanned the whole transcript;
	// absent (0) means version 1. Read it through Version().
	ScanVersion int `json:"scan_version,omitempty"`
	// Records are the typed findings (v2). The v1 lists above are still written
	// so an older binary reads this blob unchanged.
	Records RecordList `json:"records,omitempty"`
}

// ScanVersion is the current extractor version. Raising it rescans every
// transcript from the top, but only once the artifact_scan_v2 flag is on.
const ScanVersion = 2

// LegacyScanVersion is what a blob or position written before versions existed means.
const LegacyScanVersion = 1

// Version returns the extractor version recorded in the blob.
func (b *SessionArtifactsBlob) Version() int { return NormalizeScanVersion(b.ScanVersion) }

func NormalizeScanVersion(v int) int { return max(v, LegacyScanVersion) }

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
	return len(b.PRURLs)+len(b.CommitSHAs)+len(b.ExternalURLs)+len(b.Commands)+len(b.Records) > 0
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
