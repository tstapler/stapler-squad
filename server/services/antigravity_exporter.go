package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// antigravityManagedStateFile records the permissions.allow entries this exporter added,
// so a later export removes exactly those and never an entry added by hand.
const antigravityManagedStateFile = ".stapler-squad-managed-permissions.json"

// permissionsFileMu serializes read-merge-write cycles on Antigravity's settings files;
// exports run from concurrent rule RPCs and from the ssq-hooks CLI.
var permissionsFileMu sync.Mutex

// RulesToAntigravityPermissions converts auto-allow rules to Antigravity command(...)
// entries. A rule that can't be expressed without matching more than it should is
// skipped and logged, never approximated.
func RulesToAntigravityPermissions(rules []session.ApprovalRuleData) []string {
	var result []string
	seen := make(map[string]bool)

	for _, r := range rules {
		if !r.Enabled || r.Decision != 0 { // 0 = AutoAllow
			continue
		}
		entries, skipReason := ruleEntries(r)
		if skipReason != "" {
			log.Info("[AntigravityExport] rule not exported", "id", r.ID, "reason", skipReason)
			continue
		}
		for _, e := range entries {
			if !seen[e] {
				seen[e] = true
				result = append(result, e)
			}
		}
	}
	return result
}

// ruleEntries returns the entries for one auto-allow rule, or a skip reason when the
// rule's constraints can't be preserved by a flat command(...) entry.
func ruleEntries(r session.ApprovalRuleData) ([]string, string) {
	if reason := narrowingReason(r); reason != "" {
		return nil, reason
	}
	if len(r.Programs) > 0 {
		if r.CommandPattern != "" {
			return nil, "programs and command_pattern combine with AND semantics"
		}
		if r.ToolName != "" && !strings.EqualFold(r.ToolName, "Bash") {
			return nil, "programs only apply to Bash commands"
		}
		return programEntries(r.Programs, r.Subcommands), ""
	}
	if r.CommandPattern != "" {
		literal, ok := prefixLiteral(r.CommandPattern)
		if !ok {
			return nil, "command_pattern is not an anchored literal prefix"
		}
		return []string{fmt.Sprintf("command(%s)", literal)}, ""
	}
	if r.ToolName != "" {
		if strings.EqualFold(r.ToolName, "Bash") {
			return nil, "unscoped Bash rule would allow every shell command"
		}
		return []string{fmt.Sprintf("command(%s)", r.ToolName)}, ""
	}
	if r.ToolPattern != "" {
		literal, ok := prefixLiteral(r.ToolPattern)
		if !ok {
			return nil, "tool_pattern is not an anchored literal prefix"
		}
		return []string{fmt.Sprintf("command(%s)", literal)}, ""
	}
	return nil, "rule has no program, command, or tool scope"
}

// narrowingReason names a constraint that a flat command(...) entry cannot express.
// Exporting such a rule without it would allow more than the rule does.
func narrowingReason(r session.ApprovalRuleData) string {
	switch {
	case r.RequireCIPassing:
		return "require_ci_passing has no static equivalent"
	case r.FilePattern != "":
		return "file_pattern has no equivalent"
	case r.ToolCategory != "":
		return "tool_category is structural, not a literal tool name"
	case r.MinSessionIdleMinutes > 0:
		return "session-idle gating has no equivalent"
	case len(r.PythonModes) > 0 || r.SafePythonImportsOnly:
		return "python mode/import restrictions have no equivalent"
	case len(r.BlockedSubcommands) > 0 || len(r.RequiredFlags) > 0 ||
		len(r.RequiredFlagPrefixes) > 0 || len(r.ForbiddenFlags) > 0:
		return "flag and subcommand exclusions would widen the entry"
	}
	return ""
}

// prefixLiteral returns the literal text of a regex that matches exactly the commands
// starting with that text. Only ^-anchored patterns without regex syntax qualify: an
// unanchored or $-anchored pattern matches a different set than a prefix entry does.
func prefixLiteral(pattern string) (string, bool) {
	if !strings.HasPrefix(pattern, "^") || strings.HasSuffix(pattern, "$") {
		return "", false
	}
	literal := pattern[1:]
	if literal == "" || strings.ContainsAny(literal, `\.+*?()|[]{}^$`) {
		return "", false
	}
	return literal, true
}

func programEntries(programs, subcommands []string) []string {
	var entries []string
	for _, prog := range programs {
		prog = strings.TrimSpace(prog)
		if prog == "" {
			continue
		}
		if len(subcommands) == 0 {
			entries = append(entries, fmt.Sprintf("command(%s)", prog))
			continue
		}
		for _, sub := range subcommands {
			sub = strings.TrimSpace(sub)
			if sub != "" {
				entries = append(entries, fmt.Sprintf("command(%s %s)", prog, sub))
			}
		}
	}
	return entries
}

// ExportRulesToAntigravitySettings merges the rules' permissions into the settings file at
// settingsPath. It adds only entries that are absent, and removes only entries it added in
// an earlier export that no current rule produces.
func ExportRulesToAntigravitySettings(settingsPath string, rules []session.ApprovalRuleData) error {
	permissionsFileMu.Lock()
	defer permissionsFileMu.Unlock()
	return exportRulesLocked(settingsPath, rules)
}

func exportRulesLocked(settingsPath string, rules []session.ApprovalRuleData) error {
	desired := RulesToAntigravityPermissions(rules)
	statePath := filepath.Join(filepath.Dir(settingsPath), antigravityManagedStateFile)
	previouslyManaged := readManagedState(statePath)

	root, err := readSettingsObject(settingsPath)
	if err != nil {
		return err
	}
	permsMap, _ := root["permissions"].(map[string]interface{})
	if permsMap == nil {
		permsMap = map[string]interface{}{}
	}

	existing := stringList(permsMap["allow"])
	merged := mergeAllowList(existing, desired, previouslyManaged)
	managed := ownedEntries(existing, desired, previouslyManaged)

	if !sameStrings(existing, merged) {
		permsMap["allow"] = merged
		root["permissions"] = permsMap
		if err := writeJSONAtomic(settingsPath, root); err != nil {
			return fmt.Errorf("write settings %s: %w", settingsPath, err)
		}
	}
	if !sameSet(managed, previouslyManaged) {
		if err := writeManagedState(statePath, managed); err != nil {
			return fmt.Errorf("write managed state %s: %w", statePath, err)
		}
	}
	return nil
}

// mergeAllowList drops entries this exporter previously owned that no rule produces now,
// then appends desired entries not already present. Hand-added entries are always kept.
func mergeAllowList(current, desired []string, previouslyManaged map[string]bool) []string {
	desiredSet := make(map[string]bool, len(desired))
	for _, e := range desired {
		desiredSet[e] = true
	}

	seen := make(map[string]bool, len(current)+len(desired))
	merged := make([]string, 0, len(current)+len(desired))
	for _, e := range current {
		if previouslyManaged[e] && !desiredSet[e] {
			continue
		}
		if !seen[e] {
			seen[e] = true
			merged = append(merged, e)
		}
	}
	for _, e := range desired {
		if !seen[e] {
			seen[e] = true
			merged = append(merged, e)
		}
	}
	return merged
}

// ownedEntries is the set of desired entries this exporter is responsible for: ones it
// already owned, plus ones it is adding now. An entry that was already present by hand is
// not claimed, so a later rule change never deletes it.
func ownedEntries(current, desired []string, previouslyManaged map[string]bool) map[string]bool {
	present := make(map[string]bool, len(current))
	for _, e := range current {
		present[e] = true
	}
	owned := make(map[string]bool)
	for _, e := range desired {
		if previouslyManaged[e] || !present[e] {
			owned[e] = true
		}
	}
	return owned
}

// ExportAntigravityRulesFromDB exports rules from storage to all candidate Antigravity settings paths.
func ExportAntigravityRulesFromDB(ctx context.Context, storage *session.Storage) error {
	// Read under the lock: a snapshot taken before waiting for it can be written after a
	// newer export and bring a deleted rule back into permissions.allow.
	permissionsFileMu.Lock()
	defer permissionsFileMu.Unlock()

	rules, err := storage.AllRules(ctx)
	if err != nil {
		return fmt.Errorf("fetch rules for export: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home dir for export: %w", err)
	}

	candidates := []string{
		filepath.Join(home, ".gemini", "antigravity-cli", "settings.json"),
		filepath.Join(home, ".gemini", "settings.json"),
	}

	var exportErrs []string
	for _, c := range candidates {
		if err := exportRulesLocked(c, rules); err != nil {
			exportErrs = append(exportErrs, fmt.Sprintf("%s: %v", c, err))
		} else {
			log.Info("[AntigravityExport] exported rules to settings", "path", c, "count", len(rules))
		}
	}

	if len(exportErrs) > 0 {
		return fmt.Errorf("antigravity rule export errors: %s", strings.Join(exportErrs, "; "))
	}
	return nil
}

// readSettingsObject returns the settings file as a generic object, or an empty object
// when the file doesn't exist yet. Unparseable JSON is an error, so the file is never
// overwritten from a partial read.
func readSettingsObject(settingsPath string) (map[string]interface{}, error) {
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]interface{}{}, nil
		}
		return nil, fmt.Errorf("read settings %s: %w", settingsPath, err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse settings %s: %w", settingsPath, err)
	}
	if root == nil {
		root = map[string]interface{}{}
	}
	return root, nil
}

// readManagedState returns the entries recorded as owned by this exporter. A missing or
// unreadable state file yields an empty set, which removes nothing: the safe direction.
func readManagedState(statePath string) map[string]bool {
	raw, err := os.ReadFile(statePath)
	if err != nil {
		return map[string]bool{}
	}
	var entries []string
	if err := json.Unmarshal(raw, &entries); err != nil {
		log.Warn("[AntigravityExport] ignoring unreadable managed-state file", "path", statePath, "err", err)
		return map[string]bool{}
	}
	return toSet(entries)
}

func writeManagedState(statePath string, managed map[string]bool) error {
	entries := make([]string, 0, len(managed))
	for e := range managed {
		entries = append(entries, e)
	}
	sort.Strings(entries)
	return writeJSONAtomic(statePath, entries)
}

// writeJSONAtomic writes v to path via a uniquely named temp file and rename, so a
// concurrent or interrupted write never leaves a truncated file. The file is owner-only.
func writeJSONAtomic(path string, v interface{}) error {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op once renamed
	if _, err := tmp.Write(append(out, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp %s: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		return fmt.Errorf("chmod temp %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}

func stringList(v interface{}) []string {
	items, _ := v.([]interface{})
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func toSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, item := range items {
		set[item] = true
	}
	return set
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}
