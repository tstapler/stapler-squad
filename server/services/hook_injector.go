package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tstapler/stapler-squad/log"
)

// HookName is a typed constant for the built-in hooks that can be injected.
type HookName string

const (
	HookPermissionApproval HookName = "permission_approval" // maps to PermissionRequest event
	HookStopNotification   HookName = "stop_notification"   // maps to Stop event
	HookPreToolLogging     HookName = "pre_tool_logging"    // maps to PreToolUse event
	HookPostToolLogging    HookName = "post_tool_logging"   // maps to PostToolUse event
	HookPromptSubmit       HookName = "prompt_submit"       // maps to UserPromptSubmit event

	// HookGitDriftCheck also maps to PostToolUse, but on its own dedicated endpoint
	// (distinct from HookPostToolLogging's) so it can be injected independently of
	// the generic hooks a manually-created session opts into. Scoped strictly to
	// autonomous backlog work sessions — see spawnSessionAfterGates in
	// server/services/backlog_service_triage.go, the only call site that injects
	// it, and hook_receiver_drift.go for the receiver. Fires the same branch-drift
	// check that gates review (BUG-044) right after every git commit/push, so an
	// autonomous session notices and can self-correct immediately instead of only
	// learning about drift from a review verdict hours or days later.
	HookGitDriftCheck HookName = "git_drift_check" // maps to PostToolUse event
)

// hookEventName maps a HookName to the Claude Code hooks.* key.
var hookEventName = map[HookName]string{
	HookPermissionApproval: "PermissionRequest",
	HookStopNotification:   "Stop",
	HookPreToolLogging:     "PreToolUse",
	HookPostToolLogging:    "PostToolUse",
	HookPromptSubmit:       "UserPromptSubmit",
	HookGitDriftCheck:      "PostToolUse",
}

// hookBaseURLFn resolves the base URL (scheme + host + port) used to build hook callback
// endpoints. It defaults to the historical localhost:8543 address for backward compatibility
// with callers that never wire a real server (e.g. existing unit tests), and is overridden via
// SetHookBaseURLFn during real server wiring (server.go's wireDepsIntoServer) with a closure
// that reads the server's real listen address lazily — e.g. via srv.GetAddr() — so hook URLs
// are never snapshotted before the server has bound its real port (PORT=0 support).
var hookBaseURLFn = func() string { return "http://localhost:8543" }

// SetHookBaseURLFn overrides the base URL function used when building hook endpoint URLs via
// hookEndpoints. Call once during server wiring; passing nil is a no-op.
func SetHookBaseURLFn(fn func() string) {
	if fn != nil {
		hookBaseURLFn = fn
	}
}

// hookEndpoints builds the HookName -> URL map fresh from baseURLFn() on every call (never
// cached into a package-level map), so hook URLs written into a session's settings always
// reflect the base URL current at hook-injection time rather than one baked in at server- or
// package-construction time.
func hookEndpoints(baseURLFn func() string) map[HookName]string {
	base := baseURLFn()
	return map[HookName]string{
		HookPermissionApproval: base + "/api/hooks/permission-request",
		HookStopNotification:   base + "/api/hooks/stop",
		HookPreToolLogging:     base + "/api/hooks/pre-tool-use",
		HookPostToolLogging:    base + "/api/hooks/post-tool-use",
		HookPromptSubmit:       base + "/api/hooks/prompt-submit",
		HookGitDriftCheck:      base + "/api/hooks/post-tool-use-drift-check",
	}
}

// InjectHooksConfig writes (or merges) hook entries into
// <rootDir>/.claude/settings.local.json.
//
//   - HookPermissionApproval is always injected regardless of the hooks slice.
//   - Each hook entry is a curl command POSTing to the server endpoint with
//     X-CS-Session-ID set to sessionTitle.
//   - The write is atomic (temp file + rename).
//   - Idempotent: existing entries pointing to our URL are preserved.
func InjectHooksConfig(rootDir, sessionTitle string, hooks []HookName) error {
	claudeDir := filepath.Join(rootDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.local.json")

	// Build the set of hooks to inject (permission_approval always included).
	wanted := map[HookName]struct{}{HookPermissionApproval: {}}
	for _, h := range hooks {
		if _, ok := hookEventName[h]; ok {
			wanted[h] = struct{}{}
		} else {
			log.Warn("[InjectHooksConfig] unknown hook name, skipping", "name", h)
		}
	}

	// Read existing settings.
	raw := map[string]json.RawMessage{}
	data, err := os.ReadFile(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", settingsPath, err)
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &raw); err != nil {
			log.Warn("[InjectHooksConfig] settings file has invalid JSON, attempting repair", "path", settingsPath, "err", err)
			repaired, repairErr := repairSettingsJSON(data)
			if repairErr == nil {
				_ = json.Unmarshal(repaired, &raw)
			} else {
				raw = map[string]json.RawMessage{}
			}
		}
	}

	// Parse existing hooks map.
	hooksMap := map[string]json.RawMessage{}
	if hooksRaw, ok := raw["hooks"]; ok {
		_ = json.Unmarshal(hooksRaw, &hooksMap)
	}

	// Resolved once per InjectHooksConfig call (i.e. per-session, at hook-injection time), not
	// cached at package-construction time, so it reflects the server's current base URL.
	endpoints := hookEndpoints(hookBaseURLFn)

	for hookName := range wanted {
		eventKey := hookEventName[hookName]
		url := endpoints[hookName]
		curlCmd := fmt.Sprintf(
			"curl -s --max-time %d -X POST '%s' -H 'Content-Type: application/json' -H 'X-CS-Session-ID: %s' -d @-",
			hookTimeout, url, sessionTitle,
		)

		// Check if this hook command is already present.
		if existing, ok := hooksMap[eventKey]; ok {
			var groups []hookMatcherGroup
			if err := json.Unmarshal(existing, &groups); err == nil {
				alreadyPresent := false
				for _, g := range groups {
					for _, h := range g.Hooks {
						if h.Type == "command" && strings.Contains(h.Command, url) {
							alreadyPresent = true
							break
						}
					}
					if alreadyPresent {
						break
					}
				}
				if alreadyPresent {
					continue
				}
			}
		}

		// Prepend our entry.
		entry := hookEntry{Type: "command", Command: curlCmd, Timeout: hookTimeout}
		group := hookMatcherGroup{Hooks: []hookEntry{entry}}

		var existing []hookMatcherGroup
		if raw, ok := hooksMap[eventKey]; ok {
			_ = json.Unmarshal(raw, &existing)
		}
		merged := append([]hookMatcherGroup{group}, existing...)
		mergedJSON, err := json.Marshal(merged)
		if err != nil {
			return fmt.Errorf("marshal hooks for %s: %w", eventKey, err)
		}
		hooksMap[eventKey] = json.RawMessage(mergedJSON)
	}

	hooksJSON, err := json.Marshal(hooksMap)
	if err != nil {
		return fmt.Errorf("marshal hooks map: %w", err)
	}
	raw["hooks"] = json.RawMessage(hooksJSON)

	return writeSettingsAtomic(settingsPath, claudeDir, raw)
}

// RemoveHooksConfig strips any previously-injected entries for the given hooks from
// <rootDir>/.claude/settings.local.json, leaving every other hook (ours or the
// user's own) untouched. It is the inverse of InjectHooksConfig, needed because a
// backlog work session's worktree/branch is reused across reopen cycles (see
// spawnSessionAfterGates in backlog_service_triage.go — same "backlog/<item>" branch
// every revision): without an explicit removal step, a hook injected while an item
// was spawned autonomously would otherwise persist in that worktree's settings file
// forever, even after a later manual ("Reopen for Revision") respawn on the same
// worktree — silently violating the "never inject into a human-driven session"
// scoping requirement HookGitDriftCheck depends on. Call this whenever spawning a
// session in a mode that must NOT have a given hook, symmetrically with the
// InjectHooksConfig call used for the mode that must.
//
// No-op (not an error) if the settings file doesn't exist or doesn't reference the
// hook — safe to call unconditionally on every spawn.
func RemoveHooksConfig(rootDir string, hooks []HookName) error {
	if len(hooks) == 0 {
		return nil
	}
	claudeDir := filepath.Join(rootDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.local.json")

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", settingsPath, err)
	}

	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		log.Warn("[RemoveHooksConfig] settings file has invalid JSON, attempting repair", "path", settingsPath, "err", err)
		repaired, repairErr := repairSettingsJSON(data)
		if repairErr != nil {
			return fmt.Errorf("settings file is corrupt and could not be repaired: %w", repairErr)
		}
		if err := json.Unmarshal(repaired, &raw); err != nil {
			return fmt.Errorf("unmarshal repaired settings: %w", err)
		}
	}

	hooksRaw, ok := raw["hooks"]
	if !ok {
		return nil
	}
	hooksMap := map[string]json.RawMessage{}
	if err := json.Unmarshal(hooksRaw, &hooksMap); err != nil {
		return fmt.Errorf("unmarshal hooks map: %w", err)
	}

	endpoints := hookEndpoints(hookBaseURLFn)
	changed := false

	for _, hookName := range hooks {
		eventKey, ok := hookEventName[hookName]
		if !ok {
			continue
		}
		url := endpoints[hookName]
		existingRaw, ok := hooksMap[eventKey]
		if !ok {
			continue
		}
		var groups []hookMatcherGroup
		if err := json.Unmarshal(existingRaw, &groups); err != nil {
			continue
		}

		filtered := make([]hookMatcherGroup, 0, len(groups))
		for _, g := range groups {
			keptHooks := make([]hookEntry, 0, len(g.Hooks))
			for _, h := range g.Hooks {
				if h.Type == "command" && strings.Contains(h.Command, url) {
					changed = true
					continue
				}
				keptHooks = append(keptHooks, h)
			}
			if len(keptHooks) > 0 {
				g.Hooks = keptHooks
				filtered = append(filtered, g)
			} else if len(g.Hooks) > 0 {
				// The group had hooks but all were removed — drop the (now-empty) group.
				changed = true
			} else {
				filtered = append(filtered, g)
			}
		}

		if len(filtered) == 0 {
			delete(hooksMap, eventKey)
		} else {
			filteredJSON, err := json.Marshal(filtered)
			if err != nil {
				return fmt.Errorf("marshal hooks for %s: %w", eventKey, err)
			}
			hooksMap[eventKey] = json.RawMessage(filteredJSON)
		}
	}

	if !changed {
		return nil
	}

	hooksJSON, err := json.Marshal(hooksMap)
	if err != nil {
		return fmt.Errorf("marshal hooks map: %w", err)
	}
	raw["hooks"] = json.RawMessage(hooksJSON)

	return writeSettingsAtomic(settingsPath, claudeDir, raw)
}
