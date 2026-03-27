package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tstapler/stapler-squad/log"
)

// ClaudePermissions mirrors the "permissions" key in ~/.claude/settings.json.
type ClaudePermissions struct {
	Allow []string `json:"allow"` // tool patterns, e.g. "Bash(git log*)"
	Deny  []string `json:"deny,omitempty"`
}

// claudeSettingsFile is the partial structure we parse from settings.json.
type claudeSettingsFile struct {
	Permissions *ClaudePermissions `json:"permissions,omitempty"`
}

// ParseClaudeSettings reads a Claude settings.json file and extracts permissions.
// Returns nil permissions (no error) if the file does not exist or has no permissions key.
func ParseClaudeSettings(path string) (*ClaudePermissions, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var settings claudeSettingsFile
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return settings.Permissions, nil
}

// LoadClaudeSettingsRules parses both the global and project-level Claude settings
// and returns AutoAllow rules derived from their permissions.allow lists.
//
// Search order:
//  1. ~/.claude/settings.json (global)
//  2. ~/.claude/settings.local.json (global local overrides)
//  3. <projectDir>/.claude/settings.json (project)
//  4. <projectDir>/.claude/settings.local.json (project local)
//
// Project settings take precedence: if both define the same tool pattern, the project
// rule will be checked first due to higher priority.
func LoadClaudeSettingsRules(projectDir string) []Rule {
	var allRules []Rule

	home, _ := os.UserHomeDir()

	type settingsPath struct {
		path     string
		priority int
		label    string
	}

	paths := []settingsPath{
		{filepath.Join(home, ".claude", "settings.json"), 150, "global"},
		{filepath.Join(home, ".claude", "settings.local.json"), 160, "global-local"},
	}
	if projectDir != "" {
		paths = append(paths,
			settingsPath{filepath.Join(projectDir, ".claude", "settings.json"), 170, "project"},
			settingsPath{filepath.Join(projectDir, ".claude", "settings.local.json"), 180, "project-local"},
		)
	}

	for _, p := range paths {
		perms, err := ParseClaudeSettings(p.path)
		if err != nil {
			log.WarningLog.Printf("[ClaudeSettings] Skipping %s: %v", p.path, err)
			continue
		}
		if perms == nil || len(perms.Allow) == 0 {
			continue
		}
		rules := claudeAllowsToRules(perms.Allow, p.priority, p.label)
		allRules = append(allRules, rules...)
		log.InfoLog.Printf("[ClaudeSettings] Loaded %d allow rules from %s", len(rules), p.path)
	}
	return allRules
}

// claudeAllowsToRules converts Claude's allow patterns to AutoAllow rules.
//
// Claude allow patterns have the form:
//   - "Bash"           -- allow any Bash invocation
//   - "Bash(git log*)" -- allow Bash where command starts with "git log"
//   - "Read"           -- allow any Read invocation
//
// Glob wildcards (*) are converted to regex (.*).
func claudeAllowsToRules(allows []string, basePriority int, label string) []Rule {
	var rules []Rule
	for i, pattern := range allows {
		rule := Rule{
			ID:        fmt.Sprintf("claude-settings-%s-%d", label, i),
			Name:      fmt.Sprintf("Claude settings allow: %s", pattern),
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    fmt.Sprintf("Allowed by Claude settings (%s): %s", label, pattern),
			Priority:  basePriority,
			Enabled:   true,
			Source:    "claude-settings",
		}

		// Parse "ToolName(commandGlob)" or just "ToolName".
		if idx := strings.Index(pattern, "("); idx != -1 {
			toolName := pattern[:idx]
			glob := strings.TrimSuffix(pattern[idx+1:], ")")
			rule.ToolName = toolName
			// Convert glob to regex: escape special chars, then replace * -> .*
			reStr := globToRegex(glob)
			re, err := regexp.Compile(reStr)
			if err != nil {
				log.WarningLog.Printf("[ClaudeSettings] Skipping invalid pattern %q: %v", pattern, err)
				continue
			}
			rule.CommandPattern = re
		} else {
			rule.ToolName = pattern
		}
		rules = append(rules, rule)
	}
	return rules
}

// globToRegex converts a simple glob pattern to a regex string.
// Only * is supported (matches any sequence of characters).
func globToRegex(glob string) string {
	// Escape regex metacharacters, then replace escaped \* with .*
	escaped := regexp.QuoteMeta(glob)
	return "^" + strings.ReplaceAll(escaped, `\*`, `.*`)
}
