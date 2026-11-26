package goalrunner

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type hookCmd struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type hookGroup struct {
	Hooks []hookCmd `json:"hooks"`
}

// BuildSettings renders the --settings JSON for a goal run. /goal installs its
// own session-scoped Stop hook at runtime, so the file holds only Stapler
// Squad's hooks; Claude Code runs every Stop hook it has, from all sources.
func BuildSettings(stopHook, permissionHook string) ([]byte, error) {
	hooks := map[string][]hookGroup{}
	if stopHook != "" {
		hooks["Stop"] = []hookGroup{{Hooks: []hookCmd{{Type: "command", Command: stopHook}}}}
	}
	if permissionHook != "" {
		hooks["PermissionRequest"] = []hookGroup{{Hooks: []hookCmd{{Type: "command", Command: permissionHook}}}}
	}
	return json.MarshalIndent(map[string]any{"hooks": hooks}, "", "  ")
}

// WriteSettings writes BuildSettings output to dir/goal-settings.json (0600).
func WriteSettings(dir, stopHook, permissionHook string) (string, error) {
	raw, err := BuildSettings(stopHook, permissionHook)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "goal-settings.json")
	return path, os.WriteFile(path, raw, 0o600)
}
