// Package claudehooks installs and detects Stapler Squad's standalone Claude Code
// hooks in a global ~/.claude/settings.json file.
//
// Two independent hooks are supported:
//
//   - Rules: a PreToolUse hook running "<ssq-hooks> check", which classifies each
//     tool request against the local rules database (allow/deny).
//   - Notifications: Notification + Stop hooks running "<ssq-hook-handler> <event>",
//     which produce audio chimes / attention alerts.
//
// Unlike the per-session HTTP hooks in server/services/hook_injector.go (which are
// scoped to a stapler-squad-managed session via X-CS-Session-ID), these target the
// user's GLOBAL settings so they fire for any Claude Code session, including ones
// started from a plain terminal.
//
// All mutations are idempotent and write atomically (temp file + rename).
package claudehooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// mu serializes read-modify-write cycles on a settings file. All writers live in
// this process (the server's InstallHooks handler and the ssq-hooks CLI), so a
// package mutex is sufficient to prevent a lost update / clobbered file when two
// installs race (e.g. a double-click in onboarding).
var mu sync.Mutex //nolint:gochecknoglobals // serializes settings.json read-modify-write across all in-process callers

// Markers identify our hook commands regardless of the absolute path they were
// installed with, so detection and idempotency survive a binary relocation.
const (
	rulesMarker        = "ssq-hooks check"
	notificationMarker = "ssq-hook-handler"
)

// Status reports which of our global hooks are currently installed.
type Status struct {
	RulesInstalled         bool `json:"rulesInstalled"`
	NotificationsInstalled bool `json:"notificationsInstalled"`
}

// DefaultGlobalSettingsPath returns ~/.claude/settings.json.
func DefaultGlobalSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// RulesHookCommand is the PreToolUse command for a given ssq-hooks binary path.
func RulesHookCommand(binPath string) string { return binPath + " check" }

// DetectStatus reads settingsPath and reports which hooks are present. A missing
// settings file is not an error — it reports both hooks as not installed.
func DetectStatus(settingsPath string) (Status, error) {
	settings, err := readSettings(settingsPath)
	if err != nil {
		return Status{}, err
	}
	hooks, _ := settings["hooks"].(map[string]interface{})
	return Status{
		RulesInstalled: eventHasCommandContaining(hooks, "PreToolUse", rulesMarker),
		NotificationsInstalled: eventHasCommandContaining(hooks, "Notification", notificationMarker) ||
			eventHasCommandContaining(hooks, "Stop", notificationMarker),
	}, nil
}

// InstallRules registers "<binPath> check" as a PreToolUse hook. Idempotent:
// it is a no-op if a PreToolUse hook with the rules marker is already present.
func InstallRules(settingsPath, binPath string) error {
	return mutate(settingsPath, func(hooks map[string]interface{}) {
		if eventHasCommandContaining(hooks, "PreToolUse", rulesMarker) {
			return
		}
		prependHook(hooks, "PreToolUse", RulesHookCommand(binPath))
	})
}

// InstallNotifications registers "<handlerPath> notification" and
// "<handlerPath> stop" as Notification and Stop hooks. Idempotent per event.
func InstallNotifications(settingsPath, handlerPath string) error {
	return mutate(settingsPath, func(hooks map[string]interface{}) {
		if !eventHasCommandContaining(hooks, "Notification", notificationMarker) {
			prependHook(hooks, "Notification", handlerPath+" notification")
		}
		if !eventHasCommandContaining(hooks, "Stop", notificationMarker) {
			prependHook(hooks, "Stop", handlerPath+" stop")
		}
	})
}

// mutate reads settingsPath, hands the hooks map to fn (creating it if absent),
// and writes the result atomically.
func mutate(settingsPath string, fn func(hooks map[string]interface{})) error {
	mu.Lock()
	defer mu.Unlock()

	settings, err := readSettings(settingsPath)
	if err != nil {
		return err
	}
	if existing, ok := settings["hooks"]; ok {
		if _, ok := existing.(map[string]interface{}); !ok {
			return fmt.Errorf("parsing %s: \"hooks\" field is not an object", settingsPath)
		}
	}
	hooks, _ := settings["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = map[string]interface{}{}
		settings["hooks"] = hooks
	}

	fn(hooks)

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(settingsPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// Unique temp file in the same dir so the rename is atomic and a crash mid-write
	// can't leave a stale fixed-name .tmp or be clobbered by a concurrent writer.
	tmp, err := os.CreateTemp(dir, "settings-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) //nolint:errcheck // best-effort cleanup if rename fails
	if _, err := tmp.Write(append(out, '\n')); err != nil {
		tmp.Close() //nolint:errcheck
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, settingsPath)
}

// readSettings parses settingsPath into a map. A missing file yields an empty map.
func readSettings(settingsPath string) (map[string]interface{}, error) {
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]interface{}{}, nil
		}
		return nil, err
	}
	if len(raw) == 0 {
		return map[string]interface{}{}, nil
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", settingsPath, err)
	}
	return settings, nil
}

// eventHasCommandContaining reports whether any hook command under hooks[event]
// contains marker. Tolerates malformed entries by skipping them.
func eventHasCommandContaining(hooks map[string]interface{}, event, marker string) bool {
	if hooks == nil {
		return false
	}
	groups, _ := hooks[event].([]interface{})
	for _, g := range groups {
		m, ok := g.(map[string]interface{})
		if !ok {
			continue
		}
		list, _ := m["hooks"].([]interface{})
		for _, h := range list {
			hm, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			if cmd, _ := hm["command"].(string); strings.Contains(cmd, marker) {
				return true
			}
		}
	}
	return false
}

// prependHook adds a `.*`-matcher command group to the front of hooks[event] so
// our hook runs before any existing ones.
func prependHook(hooks map[string]interface{}, event, command string) {
	existing, _ := hooks[event].([]interface{})
	entry := map[string]interface{}{
		"matcher": ".*",
		"hooks": []interface{}{
			map[string]interface{}{"type": "command", "command": command},
		},
	}
	hooks[event] = append([]interface{}{entry}, existing...)
}
