package fsops

import (
	"fmt"
	"os"
	"strings"
)

// ExpandTilde replaces a leading "~" with the user's home directory.
// A lone "~" is treated as "~/" so that path completion lists home-directory contents.
func ExpandTilde(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not expand ~: %w", err)
	}
	if path == "~" {
		// Treat lone "~" as "~/" so the caller sees home dir entries.
		return home + "/", nil
	}
	if strings.HasPrefix(path, "~/") {
		// Preserve any trailing slash in the user's input.
		return home + path[1:], nil
	}
	// ~username form not supported server-side; return as-is.
	return path, nil
}
