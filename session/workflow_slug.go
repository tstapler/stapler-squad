package session

import (
	"fmt"
	"regexp"
	"strings"
)

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)

// ValidateWorkflowSlug validates that slug conforms to the workflow slug format:
// - 2–64 characters
// - Lowercase alphanumeric with hyphens
// - No leading/trailing hyphens
// - No consecutive hyphens
func ValidateWorkflowSlug(slug string) error {
	if len(slug) < 2 || len(slug) > 64 {
		return fmt.Errorf("slug must be 2–64 characters")
	}
	if !slugRe.MatchString(slug) {
		return fmt.Errorf("slug must be lowercase alphanumeric with hyphens (no leading/trailing/consecutive hyphens)")
	}
	if strings.Contains(slug, "--") {
		return fmt.Errorf("slug must be lowercase alphanumeric with hyphens (no leading/trailing/consecutive hyphens)")
	}
	return nil
}
