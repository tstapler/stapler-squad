package omnibar

import (
	"claude-squad/ui/overlay"
	"os"
	"path/filepath"
	"strings"
)

// LocalPathDetector detects local filesystem paths
// This is the catch-all detector with lowest priority
type LocalPathDetector struct{}

func (d *LocalPathDetector) Name() string {
	return "LocalPath"
}

func (d *LocalPathDetector) Priority() int {
	return 100 // Lowest priority - catch-all
}

func (d *LocalPathDetector) Detect(input string) *DetectionResult {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	// Don't detect if it looks like a GitHub URL or shorthand
	if strings.Contains(input, "github.com") {
		return nil
	}

	// Check if it contains exactly one "/" with no path separators - likely GitHub shorthand
	if strings.Count(input, "/") == 1 && !strings.HasPrefix(input, "/") &&
		!strings.HasPrefix(input, "~") && !strings.HasPrefix(input, ".") {
		// Could be GitHub shorthand (owner/repo)
		return nil
	}

	// Check for path indicators
	isPath := false

	// Absolute paths
	if strings.HasPrefix(input, "/") {
		isPath = true
	}

	// Home directory paths
	if strings.HasPrefix(input, "~") {
		isPath = true
	}

	// Relative paths
	if strings.HasPrefix(input, "./") || strings.HasPrefix(input, "../") {
		isPath = true
	}

	// Current directory
	if input == "." || input == ".." {
		isPath = true
	}

	// Path with multiple separators likely indicates a path
	if strings.Count(input, "/") > 1 || strings.Count(input, string(os.PathSeparator)) > 1 {
		isPath = true
	}

	if !isPath {
		return nil
	}

	// Generate suggested session name from path
	suggestedName := generateSessionNameFromPath(input)

	return &DetectionResult{
		Type:          InputTypeLocalPath,
		Confidence:    0.8,
		ParsedValue:   input,
		SuggestedName: suggestedName,
		LocalPath:     input,
		Metadata:      make(map[string]interface{}),
	}
}

func (d *LocalPathDetector) Validate(result *DetectionResult) *ValidationResult {
	if result == nil || result.LocalPath == "" {
		return &ValidationResult{
			Valid:        false,
			ErrorMessage: "No path provided",
		}
	}

	// Use the existing enhanced path validation
	validation := overlay.ValidatePathEnhanced(result.LocalPath)

	return &ValidationResult{
		Valid:        validation.Valid,
		Error:        validation.Error,
		ErrorMessage: validation.ErrorMessage,
		Warnings:     validation.Warnings,
		IsGitRepo:    validation.IsGitRepo,
		ExpandedPath: validation.ExpandedPath,
	}
}

// generateSessionNameFromPath generates a suggested session name from a path
func generateSessionNameFromPath(path string) string {
	// Expand the path first
	expandedPath, err := overlay.ExpandPath(path)
	if err != nil {
		expandedPath = path
	}

	// Get the last directory name
	baseName := filepath.Base(expandedPath)

	// Clean up the name
	if baseName == "" || baseName == "." || baseName == "/" {
		// Try to get parent directory name
		parentDir := filepath.Dir(expandedPath)
		baseName = filepath.Base(parentDir)
	}

	// Sanitize for session name
	baseName = strings.ReplaceAll(baseName, " ", "-")

	return baseName
}
