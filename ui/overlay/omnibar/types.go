// Package omnibar provides the intelligent input detection system for session creation.
// It supports multiple input types including local paths, GitHub URLs, and shorthand references.
package omnibar

import (
	"claude-squad/github"
)

// InputType represents the type of input detected from user input
type InputType int

const (
	InputTypeUnknown InputType = iota
	InputTypeLocalPath
	InputTypePathWithBranch
	InputTypeGitHubPR
	InputTypeGitHubBranch
	InputTypeGitHubRepo
	InputTypeGitHubShorthand
)

// String returns a human-readable name for the input type
func (t InputType) String() string {
	switch t {
	case InputTypeLocalPath:
		return "Local Path"
	case InputTypePathWithBranch:
		return "Path + Branch"
	case InputTypeGitHubPR:
		return "GitHub PR"
	case InputTypeGitHubBranch:
		return "GitHub Branch"
	case InputTypeGitHubRepo:
		return "GitHub Repository"
	case InputTypeGitHubShorthand:
		return "GitHub Shorthand"
	default:
		return "Unknown"
	}
}

// Icon returns an emoji icon for the input type
func (t InputType) Icon() string {
	switch t {
	case InputTypeLocalPath:
		return "📁"
	case InputTypePathWithBranch:
		return "📁🌿"
	case InputTypeGitHubPR:
		return "🔀"
	case InputTypeGitHubBranch:
		return "🌿"
	case InputTypeGitHubRepo:
		return "📦"
	case InputTypeGitHubShorthand:
		return "📦"
	default:
		return "❓"
	}
}

// DetectionResult contains the result of input type detection
type DetectionResult struct {
	// Type is the detected input type
	Type InputType

	// Confidence is a value between 0 and 1 indicating detection confidence
	Confidence float64

	// ParsedValue contains the parsed/normalized value of the input
	ParsedValue string

	// SuggestedName is a suggested session name based on the input
	SuggestedName string

	// LocalPath is set for local path types
	LocalPath string

	// Branch is set for path+branch or GitHub branch types
	Branch string

	// GitHubRef is set for GitHub URL types
	GitHubRef *github.ParsedGitHubRef

	// Metadata contains additional information from detection
	Metadata map[string]interface{}
}

// ValidationResult contains the result of input validation
type ValidationResult struct {
	// Valid indicates whether the input is valid
	Valid bool

	// Error contains the validation error if not valid
	Error error

	// ErrorMessage is a user-friendly error message
	ErrorMessage string

	// Warnings contains non-blocking warnings
	Warnings []string

	// IsGitRepo indicates if the path is a Git repository (for local paths)
	IsGitRepo bool

	// ExpandedPath is the fully expanded path (for local paths)
	ExpandedPath string

	// RequiresClone indicates if the repository needs to be cloned (for GitHub URLs)
	RequiresClone bool

	// PRInfo contains PR metadata (for GitHub PR URLs)
	PRInfo *PRMetadata
}

// PRMetadata contains information about a GitHub pull request
type PRMetadata struct {
	Number       int
	Title        string
	Author       string
	Description  string
	Status       string // "open", "closed", "merged"
	BaseBranch   string
	HeadBranch   string
	FilesChanged int
	Additions    int
	Deletions    int
	Labels       []string
	IsDraft      bool
}

// SessionSource represents the source configuration for creating a session
type SessionSource struct {
	// Type indicates the source type
	Type InputType

	// LocalPath is the local filesystem path (for local or cloned repos)
	LocalPath string

	// Branch is the branch to use or create
	Branch string

	// GitHubRef contains GitHub reference information
	GitHubRef *github.ParsedGitHubRef

	// PRInfo contains PR metadata if this is a PR-based session
	PRInfo *PRMetadata

	// SuggestedName is the suggested session name
	SuggestedName string

	// RequiresClone indicates if the repository needs to be cloned first
	RequiresClone bool

	// GeneratePRPrompt indicates whether to generate a PR context prompt
	GeneratePRPrompt bool
}
