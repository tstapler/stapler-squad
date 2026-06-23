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
}

// CommandArtifact records a structured signal extracted from a tool_use bash command.
type CommandArtifact struct {
	Type    string `json:"type"`    // "gh_pr_create", "gh_pr_merge", "git_commit", "package_install"
	Command string `json:"command"` // first 200 chars of the raw command for display
	Detail  string `json:"detail"`  // extracted value: PR title, commit message, pkg name, PR number
}

const maxExternalURLs = 50
const maxCommands = 30
