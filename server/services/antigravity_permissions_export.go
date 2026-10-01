package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/classifier"
)

// antigravityCommandEntries translates one AutoAllow rule into zero or more of
// Antigravity's permissions.allow entries ("command(<text>)" strings — see
// ~/.gemini/antigravity-cli/settings.json, grown one entry at a time by
// clicking "always allow" in the Antigravity UI). It returns the entries plus
// a non-empty skip reason when the rule can't be represented safely.
//
// A rule is skipped rather than approximated whenever a flat allow-string
// would necessarily be WIDER than the rule itself (RequireCIPassing and the
// Blocked/Required/Forbidden-flag fields all narrow an otherwise-matching
// command; Antigravity's format has no equivalent narrowing, so exporting the
// bare command would silently grant more than the rule intended) or has no
// literal representation at all (FilePattern/ToolCategory match structure
// Antigravity's command(...) wrapper can't express). This is a hard format
// limitation, not a conservativeness choice.
func antigravityCommandEntries(rule classifier.Rule) ([]string, string) {
	if rule.RequireCIPassing {
		return nil, "RequireCIPassing has no static equivalent in permissions.allow"
	}
	if rule.FilePattern != nil {
		return nil, "FilePattern has no equivalent in Antigravity's command(...) format"
	}
	if len(rule.ToolCategory) > 0 && rule.ToolName == "" && rule.ToolPattern == nil {
		return nil, "ToolCategory is a structural bucket, not a literal name Antigravity can match"
	}
	if rule.Criteria != nil {
		if len(rule.Criteria.BlockedSubcommands) > 0 || len(rule.Criteria.RequiredFlags) > 0 ||
			len(rule.Criteria.RequiredFlagPrefixes) > 0 || len(rule.Criteria.ForbiddenFlags) > 0 {
			return nil, "flag/subcommand exclusions would be dropped, widening the exported entry beyond the rule"
		}
		if len(rule.Criteria.Programs) > 0 {
			return commandCriteriaEntries(rule.Criteria), ""
		}
	}
	if rule.CommandPattern != nil {
		pattern := rule.CommandPattern.String()
		if overbreadCommandPatterns[pattern] {
			return nil, "command_pattern matches every command — too broad to export"
		}
		return []string{fmt.Sprintf("command(%s)", pattern)}, ""
	}
	// ToolName matching is case-insensitive per classifier.Rule.ToolName's own doc
	// comment; a bare "Bash" rule with no Criteria/CommandPattern means "allow every
	// shell command" and has no safe command(...) equivalent (handled by the final
	// return below).
	if rule.ToolName != "" && !strings.EqualFold(rule.ToolName, "Bash") {
		return []string{fmt.Sprintf("command(%s)", rule.ToolName)}, ""
	}
	if rule.ToolPattern != nil {
		return []string{fmt.Sprintf("command(%s)", rule.ToolPattern.String())}, ""
	}
	return nil, "rule has no program/command/tool scoping — would allow everything"
}

// commandCriteriaEntries expands Programs x Subcommands into one entry per pair —
// e.g. Programs: ["git"], Subcommands: ["status", "log"] -> "command(git status)",
// "command(git log)" — mirroring the shape already hand-grown in the real settings
// file (command(git status), command(git log), command(git branch), ...). A program
// with no subcommands exports as the bare program name.
func commandCriteriaEntries(c *classifier.CommandCriteria) []string {
	if len(c.Subcommands) == 0 {
		entries := make([]string, len(c.Programs))
		for i, program := range c.Programs {
			entries[i] = fmt.Sprintf("command(%s)", program)
		}
		return entries
	}
	entries := make([]string, 0, len(c.Programs)*len(c.Subcommands))
	for _, program := range c.Programs {
		for _, sub := range c.Subcommands {
			entries = append(entries, fmt.Sprintf("command(%s %s)", program, sub))
		}
	}
	return entries
}

// exportableAntigravityPermissions returns the deduped, sorted set of
// permissions.allow entries for every enabled AutoAllow rule in rules.
func exportableAntigravityPermissions(rules []classifier.Rule) []string {
	seen := map[string]bool{}
	for _, rule := range rules {
		if rule.Decision != classifier.AutoAllow || !rule.Enabled {
			continue
		}
		entries, skipReason := antigravityCommandEntries(rule)
		if skipReason != "" {
			log.Info("[AntigravityPermissionsExport] rule not exportable", "id", rule.ID, "reason", skipReason)
			continue
		}
		for _, entry := range entries {
			seen[entry] = true
		}
	}
	out := make([]string, 0, len(seen))
	for entry := range seen {
		out = append(out, entry)
	}
	sort.Strings(out)
	return out
}

func antigravitySettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".gemini", "antigravity-cli", "settings.json"), nil
}

func antigravityManagedPermissionsStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".gemini", "antigravity-cli", ".stapler-squad-managed-permissions.json"), nil
}

// SyncAntigravityPermissions translates rules' AutoAllow entries into
// Antigravity's permissions.allow format and merges them into
// ~/.gemini/antigravity-cli/settings.json. It tracks which entries it added in
// a side-state file (antigravityManagedPermissionsStatePath) so a later call
// can safely add/remove only its own entries — manually-added entries (via
// Antigravity's own "always allow" UI) are never touched. Best-effort: call
// sites log and continue rather than fail the triggering RPC on error.
func SyncAntigravityPermissions(rules []classifier.Rule) error {
	settingsPath, err := antigravitySettingsPath()
	if err != nil {
		return fmt.Errorf("sync antigravity permissions: %w", err)
	}
	statePath, err := antigravityManagedPermissionsStatePath()
	if err != nil {
		return fmt.Errorf("sync antigravity permissions: %w", err)
	}
	return syncAntigravityPermissionsAt(settingsPath, statePath, rules)
}

// readManagedPermissionsState reads statePath, which writeSettingsAtomic wrote as
// {"managed": [...]} (that helper always takes a top-level object, so the entry
// list is nested under its own key rather than being the bare array at the file's
// root).
func readManagedPermissionsState(statePath string) (map[string]bool, error) {
	data, err := os.ReadFile(statePath) //#nosec G304 -- fixed path under the user's own ~/.gemini
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", statePath, err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", statePath, err)
	}
	var entries []string
	if managedRaw, ok := raw["managed"]; ok {
		if err := json.Unmarshal(managedRaw, &entries); err != nil {
			return nil, fmt.Errorf("parse %s managed: %w", statePath, err)
		}
	}
	managed := make(map[string]bool, len(entries))
	for _, e := range entries {
		managed[e] = true
	}
	return managed, nil
}

// readSettingsPermissionsAllow reads settingsPath's top-level raw object plus its
// permissions.allow list, treating a missing file as empty (first-ever sync).
func readSettingsPermissionsAllow(settingsPath string) (map[string]json.RawMessage, map[string]json.RawMessage, []string, error) {
	raw := map[string]json.RawMessage{}
	data, err := os.ReadFile(settingsPath) //#nosec G304 -- fixed path under the user's own ~/.gemini
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, nil, fmt.Errorf("read %s: %w", settingsPath, err)
	}
	if err == nil {
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, nil, nil, fmt.Errorf("parse %s: %w", settingsPath, err)
		}
	}

	permissions := map[string]json.RawMessage{}
	if permRaw, ok := raw["permissions"]; ok {
		if err := json.Unmarshal(permRaw, &permissions); err != nil {
			return nil, nil, nil, fmt.Errorf("parse %s permissions: %w", settingsPath, err)
		}
	}

	var currentAllow []string
	if allowRaw, ok := permissions["allow"]; ok {
		if err := json.Unmarshal(allowRaw, &currentAllow); err != nil {
			return nil, nil, nil, fmt.Errorf("parse %s permissions.allow: %w", settingsPath, err)
		}
	}
	return raw, permissions, currentAllow, nil
}

// mergeManagedAllowList keeps every entry that isn't ours to remove (not previously
// managed, or still desired), then appends any newly-desired entry not already
// present. Manually-added entries (never in previouslyManaged) are never dropped.
func mergeManagedAllowList(currentAllow, desired []string, previouslyManaged map[string]bool) []string {
	desiredSet := make(map[string]bool, len(desired))
	for _, e := range desired {
		desiredSet[e] = true
	}

	present := make(map[string]bool, len(currentAllow))
	merged := make([]string, 0, len(currentAllow)+len(desired))
	for _, entry := range currentAllow {
		if previouslyManaged[entry] && !desiredSet[entry] {
			continue // a rule behind this entry was deleted/disabled/changed
		}
		if !present[entry] {
			present[entry] = true
			merged = append(merged, entry)
		}
	}
	for _, entry := range desired {
		if !present[entry] {
			present[entry] = true
			merged = append(merged, entry)
		}
	}
	return merged
}

func syncAntigravityPermissionsAt(settingsPath, statePath string, rules []classifier.Rule) error {
	desired := exportableAntigravityPermissions(rules)

	previouslyManaged, err := readManagedPermissionsState(statePath)
	if err != nil {
		return fmt.Errorf("sync antigravity permissions: %w", err)
	}

	raw, permissions, currentAllow, err := readSettingsPermissionsAllow(settingsPath)
	if err != nil {
		return fmt.Errorf("sync antigravity permissions: %w", err)
	}
	merged := mergeManagedAllowList(currentAllow, desired, previouslyManaged)

	allowJSON, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("marshal permissions.allow: %w", err)
	}
	permissions["allow"] = json.RawMessage(allowJSON)
	permJSON, err := json.Marshal(permissions)
	if err != nil {
		return fmt.Errorf("marshal permissions: %w", err)
	}
	raw["permissions"] = json.RawMessage(permJSON)

	if err := writeSettingsAtomic(settingsPath, filepath.Dir(settingsPath), raw); err != nil {
		return fmt.Errorf("sync antigravity permissions: %w", err)
	}

	stateJSON, err := json.Marshal(desired)
	if err != nil {
		return fmt.Errorf("marshal managed state: %w", err)
	}
	if err := writeSettingsAtomic(statePath, filepath.Dir(statePath), map[string]json.RawMessage{
		"managed": stateJSON,
	}); err != nil {
		return fmt.Errorf("sync antigravity permissions: %w", err)
	}
	return nil
}
