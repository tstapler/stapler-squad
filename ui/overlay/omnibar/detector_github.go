package omnibar

import (
	"claude-squad/github"
	"strings"
)

// GitHubPRDetector detects GitHub pull request URLs
// Example: https://github.com/owner/repo/pull/123
type GitHubPRDetector struct{}

func (d *GitHubPRDetector) Name() string {
	return "GitHubPR"
}

func (d *GitHubPRDetector) Priority() int {
	return 10 // Highest priority for GitHub URLs
}

func (d *GitHubPRDetector) Detect(input string) *DetectionResult {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	// Quick check - must contain github.com and /pull/
	if !strings.Contains(input, "github.com") || !strings.Contains(input, "/pull/") {
		return nil
	}

	// Try to parse as GitHub reference
	parsed, err := github.ParseGitHubRef(input)
	if err != nil || parsed.Type != github.RefTypePR {
		return nil
	}

	return &DetectionResult{
		Type:          InputTypeGitHubPR,
		Confidence:    1.0,
		ParsedValue:   input,
		SuggestedName: parsed.SuggestedSessionName(),
		GitHubRef:     parsed,
		Metadata:      make(map[string]interface{}),
	}
}

func (d *GitHubPRDetector) Validate(result *DetectionResult) *ValidationResult {
	if result == nil || result.GitHubRef == nil {
		return &ValidationResult{
			Valid:        false,
			ErrorMessage: "No GitHub PR reference provided",
		}
	}

	// For now, we assume the URL is valid if it parsed correctly
	// Full validation (checking if PR exists) would require API call
	// which should be done asynchronously
	return &ValidationResult{
		Valid:         true,
		RequiresClone: true,
		Warnings:      []string{"PR metadata will be fetched during session creation"},
	}
}

// GitHubBranchDetector detects GitHub branch URLs
// Example: https://github.com/owner/repo/tree/branch-name
type GitHubBranchDetector struct{}

func (d *GitHubBranchDetector) Name() string {
	return "GitHubBranch"
}

func (d *GitHubBranchDetector) Priority() int {
	return 20
}

func (d *GitHubBranchDetector) Detect(input string) *DetectionResult {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	// Quick check - must contain github.com and /tree/
	if !strings.Contains(input, "github.com") || !strings.Contains(input, "/tree/") {
		return nil
	}

	// Try to parse as GitHub reference
	parsed, err := github.ParseGitHubRef(input)
	if err != nil || parsed.Type != github.RefTypeBranch {
		return nil
	}

	return &DetectionResult{
		Type:          InputTypeGitHubBranch,
		Confidence:    1.0,
		ParsedValue:   input,
		SuggestedName: parsed.SuggestedSessionName(),
		Branch:        parsed.Branch,
		GitHubRef:     parsed,
		Metadata:      make(map[string]interface{}),
	}
}

func (d *GitHubBranchDetector) Validate(result *DetectionResult) *ValidationResult {
	if result == nil || result.GitHubRef == nil {
		return &ValidationResult{
			Valid:        false,
			ErrorMessage: "No GitHub branch reference provided",
		}
	}

	return &ValidationResult{
		Valid:         true,
		RequiresClone: true,
	}
}

// GitHubRepoDetector detects GitHub repository URLs
// Example: https://github.com/owner/repo
type GitHubRepoDetector struct{}

func (d *GitHubRepoDetector) Name() string {
	return "GitHubRepo"
}

func (d *GitHubRepoDetector) Priority() int {
	return 30
}

func (d *GitHubRepoDetector) Detect(input string) *DetectionResult {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	// Quick check - must contain github.com
	if !strings.Contains(input, "github.com") {
		return nil
	}

	// Exclude PR and branch URLs (handled by other detectors)
	if strings.Contains(input, "/pull/") || strings.Contains(input, "/tree/") {
		return nil
	}

	// Try to parse as GitHub reference
	parsed, err := github.ParseGitHubRef(input)
	if err != nil || parsed.Type != github.RefTypeRepo {
		return nil
	}

	return &DetectionResult{
		Type:          InputTypeGitHubRepo,
		Confidence:    1.0,
		ParsedValue:   input,
		SuggestedName: parsed.SuggestedSessionName(),
		GitHubRef:     parsed,
		Metadata:      make(map[string]interface{}),
	}
}

func (d *GitHubRepoDetector) Validate(result *DetectionResult) *ValidationResult {
	if result == nil || result.GitHubRef == nil {
		return &ValidationResult{
			Valid:        false,
			ErrorMessage: "No GitHub repository reference provided",
		}
	}

	return &ValidationResult{
		Valid:         true,
		RequiresClone: true,
	}
}

// GitHubShorthandDetector detects GitHub shorthand references
// Examples: owner/repo, owner/repo:branch
type GitHubShorthandDetector struct{}

func (d *GitHubShorthandDetector) Name() string {
	return "GitHubShorthand"
}

func (d *GitHubShorthandDetector) Priority() int {
	return 40
}

func (d *GitHubShorthandDetector) Detect(input string) *DetectionResult {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	// Quick check - must look like GitHub shorthand (owner/repo or owner/repo:branch)
	// Should NOT contain github.com (those are handled by URL detectors)
	if strings.Contains(input, "github.com") {
		return nil
	}

	// Should NOT look like a path
	if strings.HasPrefix(input, "/") || strings.HasPrefix(input, "~") ||
		strings.HasPrefix(input, "./") || strings.HasPrefix(input, "../") {
		return nil
	}

	// Must have exactly one "/" (for owner/repo format)
	if strings.Count(input, "/") != 1 {
		return nil
	}

	// Use GitHub's IsGitHubRef for validation
	if !github.IsGitHubRef(input) {
		return nil
	}

	// Try to parse as GitHub reference
	parsed, err := github.ParseGitHubRef(input)
	if err != nil {
		return nil
	}

	inputType := InputTypeGitHubShorthand
	if parsed.Type == github.RefTypeBranch {
		inputType = InputTypeGitHubBranch
	}

	return &DetectionResult{
		Type:          inputType,
		Confidence:    0.9, // Slightly lower confidence since it's shorthand
		ParsedValue:   input,
		SuggestedName: parsed.SuggestedSessionName(),
		Branch:        parsed.Branch,
		GitHubRef:     parsed,
		Metadata:      make(map[string]interface{}),
	}
}

func (d *GitHubShorthandDetector) Validate(result *DetectionResult) *ValidationResult {
	if result == nil || result.GitHubRef == nil {
		return &ValidationResult{
			Valid:        false,
			ErrorMessage: "No GitHub shorthand reference provided",
		}
	}

	return &ValidationResult{
		Valid:         true,
		RequiresClone: true,
		Warnings:      []string{"Repository will be cloned from GitHub"},
	}
}
