package vnc

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// DepsResult holds the outcome of a dependency check.
type DepsResult struct {
	// Available is true when all required binaries are present and the platform
	// is supported. Only when Available is true will New() return a live manager.
	Available bool
	// Missing lists the names of binaries that could not be found via LookPath.
	Missing []string
	// Reason is a human-readable explanation when Available is false.
	Reason string
}

// requiredBinaries are the external programs needed for VNC support.
var requiredBinaries = []string{"Xvfb", "x11vnc", "xdotool"}

// CheckDependencies checks whether all required VNC binaries are present and
// the current OS is Linux. It is safe to call multiple times; each call
// re-runs the LookPath checks (results are cached by New() callers).
func CheckDependencies() DepsResult {
	if runtime.GOOS != "linux" {
		return DepsResult{
			Available: false,
			Reason:    fmt.Sprintf("platform not supported: VNC requires Linux, got %s", runtime.GOOS),
		}
	}

	var missing []string
	for _, bin := range requiredBinaries {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}

	if len(missing) > 0 {
		return DepsResult{
			Available: false,
			Missing:   missing,
			Reason:    fmt.Sprintf("missing required binaries: %s", strings.Join(missing, ", ")),
		}
	}

	return DepsResult{Available: true}
}
