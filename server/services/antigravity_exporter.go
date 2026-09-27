package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// RulesToAntigravityPermissions converts auto-allow ApprovalRuleData entries to Antigravity command(...) strings.
func RulesToAntigravityPermissions(rules []session.ApprovalRuleData) []string {
	var result []string
	seen := make(map[string]bool)

	for _, r := range rules {
		if !r.Enabled || r.Decision != 0 { // 0 = AutoAllow
			continue
		}

		entries := ruleToPermissionEntries(r)
		for _, e := range entries {
			if !seen[e] {
				seen[e] = true
				result = append(result, e)
			}
		}
	}
	return result
}

func ruleToPermissionEntries(r session.ApprovalRuleData) []string {
	if len(r.Programs) == 0 {
		if r.CommandPattern != "" {
			clean := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(r.CommandPattern, "^"), "$"), ".*"))
			if clean != "" && !strings.ContainsAny(clean, "[]{}()\\+?|") {
				return []string{fmt.Sprintf("command(%s)", clean)}
			}
		} else if r.ToolName != "" && r.ToolName != "Bash" {
			return []string{fmt.Sprintf("command(%s)", r.ToolName)}
		}
		return nil
	}

	var entries []string
	for _, prog := range r.Programs {
		prog = strings.TrimSpace(prog)
		if prog == "" {
			continue
		}
		if len(r.Subcommands) == 0 {
			entries = append(entries, fmt.Sprintf("command(%s)", prog))
			continue
		}
		for _, sub := range r.Subcommands {
			sub = strings.TrimSpace(sub)
			if sub != "" {
				entries = append(entries, fmt.Sprintf("command(%s %s)", prog, sub))
			}
		}
	}
	return entries
}

// ExportRulesToAntigravitySettings updates the specified settingsPath with the exported permissions.
func ExportRulesToAntigravitySettings(settingsPath string, rules []session.ApprovalRuleData) error {
	newPerms := RulesToAntigravityPermissions(rules)
	if len(newPerms) == 0 {
		return nil
	}

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read settings %s: %w", settingsPath, err)
		}
		raw = []byte("{}")
	}

	var root map[string]interface{}
	if err := json.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("parse settings %s: %w", settingsPath, err)
	}
	if root == nil {
		root = make(map[string]interface{})
	}

	permsMap, _ := root["permissions"].(map[string]interface{})
	if permsMap == nil {
		permsMap = make(map[string]interface{})
		root["permissions"] = permsMap
	}

	var existingAllows []string
	if rawAllow, ok := permsMap["allow"].([]interface{}); ok {
		for _, item := range rawAllow {
			if s, ok := item.(string); ok {
				existingAllows = append(existingAllows, s)
			}
		}
	}

	allowSet := make(map[string]bool, len(existingAllows)+len(newPerms))
	mergedAllows := make([]string, 0, len(existingAllows)+len(newPerms))

	for _, item := range existingAllows {
		if !allowSet[item] {
			allowSet[item] = true
			mergedAllows = append(mergedAllows, item)
		}
	}
	for _, item := range newPerms {
		if !allowSet[item] {
			allowSet[item] = true
			mergedAllows = append(mergedAllows, item)
		}
	}

	permsMap["allow"] = mergedAllows

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0700); err != nil {
		return fmt.Errorf("create settings dir %s: %w", filepath.Dir(settingsPath), err)
	}

	tmpPath := settingsPath + ".tmp"
	if err := os.WriteFile(tmpPath, append(out, '\n'), 0600); err != nil {
		return fmt.Errorf("write temp settings %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, settingsPath); err != nil {
		return fmt.Errorf("atomic rename settings %s: %w", settingsPath, err)
	}
	return nil
}

// ExportAntigravityRulesFromDB exports rules from storage to all candidate Antigravity settings paths.
func ExportAntigravityRulesFromDB(ctx context.Context, storage *session.Storage) error {
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
		if err := ExportRulesToAntigravitySettings(c, rules); err != nil {
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
