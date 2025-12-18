package omnibar

import (
	"claude-squad/ui/overlay"
	"path/filepath"
	"regexp"
	"strings"
)

// PathWithBranchDetector detects local paths with branch specifier using @ notation
// Example: /path/to/repo@feature-branch, ~/projects/myapp@main
type PathWithBranchDetector struct{}

// pathBranchPattern matches path@branch format
// The @ is used as separator since it's not valid in git branch names
var pathBranchPattern = regexp.MustCompile(`^(.+)@([^@/\\]+)$`)

func (d *PathWithBranchDetector) Name() string {
	return "PathWithBranch"
}

func (d *PathWithBranchDetector) Priority() int {
	return 50 // Higher priority than LocalPath, lower than GitHub
}

func (d *PathWithBranchDetector) Detect(input string) *DetectionResult {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	// Don't detect if it looks like a GitHub URL
	if strings.Contains(input, "github.com") {
		return nil
	}

	// Must contain @ to be path+branch
	if !strings.Contains(input, "@") {
		return nil
	}

	// Try to match path@branch pattern
	matches := pathBranchPattern.FindStringSubmatch(input)
	if matches == nil || len(matches) != 3 {
		return nil
	}

	path := matches[1]
	branch := matches[2]

	// Validate branch name looks valid
	if !isValidBranchName(branch) {
		return nil
	}

	// Validate path looks like a path (not GitHub shorthand)
	if !looksLikePath(path) {
		return nil
	}

	// Generate suggested session name
	suggestedName := generateSessionNameFromPathAndBranch(path, branch)

	return &DetectionResult{
		Type:          InputTypePathWithBranch,
		Confidence:    0.9,
		ParsedValue:   input,
		SuggestedName: suggestedName,
		LocalPath:     path,
		Branch:        branch,
		Metadata:      make(map[string]interface{}),
	}
}

func (d *PathWithBranchDetector) Validate(result *DetectionResult) *ValidationResult {
	if result == nil || result.LocalPath == "" {
		return &ValidationResult{
			Valid:        false,
			ErrorMessage: "No path provided",
		}
	}

	// Use the existing enhanced path validation for the path component
	validation := overlay.ValidatePathEnhanced(result.LocalPath)

	validationResult := &ValidationResult{
		Valid:        validation.Valid,
		Error:        validation.Error,
		ErrorMessage: validation.ErrorMessage,
		Warnings:     validation.Warnings,
		IsGitRepo:    validation.IsGitRepo,
		ExpandedPath: validation.ExpandedPath,
	}

	// Add warning if the path is not a git repository
	if validation.Valid && !validation.IsGitRepo {
		validationResult.Warnings = append(validationResult.Warnings,
			"Path is not a Git repository - branch specification will be ignored")
	}

	// Validate branch name
	if result.Branch != "" && !isValidBranchName(result.Branch) {
		validationResult.Valid = false
		validationResult.ErrorMessage = "Invalid branch name: " + result.Branch
	}

	return validationResult
}

// looksLikePath checks if a string looks like a filesystem path
func looksLikePath(s string) bool {
	// Absolute paths
	if strings.HasPrefix(s, "/") {
		return true
	}
	// Home directory
	if strings.HasPrefix(s, "~") {
		return true
	}
	// Relative paths
	if strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		return true
	}
	// Current/parent directory
	if s == "." || s == ".." {
		return true
	}
	// Multiple path separators indicates a path
	if strings.Count(s, "/") > 1 {
		return true
	}
	return false
}

// isValidBranchName checks if a string is a valid git branch name
func isValidBranchName(name string) bool {
	if name == "" || len(name) > 255 {
		return false
	}
	// Git branch names can't start with a dot, end with .lock, contain certain characters
	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".lock") {
		return false
	}
	// Check for invalid characters
	invalidChars := []string{" ", "~", "^", ":", "?", "*", "[", "\\", ".."}
	for _, invalid := range invalidChars {
		if strings.Contains(name, invalid) {
			return false
		}
	}
	return true
}

// generateSessionNameFromPathAndBranch generates a suggested session name
func generateSessionNameFromPathAndBranch(path, branch string) string {
	// Expand the path first
	expandedPath, err := overlay.ExpandPath(path)
	if err != nil {
		expandedPath = path
	}

	// Get the last directory name
	baseName := filepath.Base(expandedPath)
	if baseName == "" || baseName == "." || baseName == "/" {
		parentDir := filepath.Dir(expandedPath)
		baseName = filepath.Base(parentDir)
	}

	// Sanitize branch name for session name (replace / with -)
	sanitizedBranch := strings.ReplaceAll(branch, "/", "-")

	// Combine: repo-branch
	sessionName := baseName + "-" + sanitizedBranch
	sessionName = strings.ReplaceAll(sessionName, " ", "-")

	return sessionName
}
