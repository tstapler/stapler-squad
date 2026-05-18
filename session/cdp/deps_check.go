package cdp

import (
	"fmt"
	"os/exec"
)

// DepsResult holds the outcome of a CDP dependency check.
type DepsResult struct {
	// Available is true when a Chrome/Chromium binary was found.
	// Only when Available is true will New() return a live manager.
	Available bool
	// ChromePath is the absolute path to the first Chrome binary found.
	// Empty when Available is false.
	ChromePath string
	// Reason is a human-readable explanation when Available is false.
	Reason string
}

// chromeBinaries is the ordered list of Chrome/Chromium binary names to search.
var chromeBinaries = []string{
	"google-chrome",
	"google-chrome-stable",
	"chromium",
	"chromium-browser",
}

// CheckDependencies checks whether a Chrome or Chromium binary is present on
// the current host. It is safe to call multiple times; each call re-runs the
// LookPath checks (results are not cached).
func CheckDependencies() DepsResult {
	for _, bin := range chromeBinaries {
		path, err := exec.LookPath(bin)
		if err == nil && path != "" {
			return DepsResult{
				Available:  true,
				ChromePath: path,
			}
		}
	}

	return DepsResult{
		Available: false,
		Reason:    fmt.Sprintf("no Chrome or Chromium binary found; searched: %v", chromeBinaries),
	}
}
