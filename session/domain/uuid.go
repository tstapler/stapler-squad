package domain

import (
	"regexp"
	"strings"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// IsValidUUID reports whether s is a UUID in 8-4-4-4-12 hex form (case-insensitive).
// Claude CLI requires valid UUIDs for session IDs, so callers validate before building a command.
func IsValidUUID(s string) bool {
	return uuidRegex.MatchString(strings.ToLower(s))
}
