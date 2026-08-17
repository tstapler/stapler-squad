package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/classifier"
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

var (
	// ErrSettingsNotFound is returned when the Claude settings file is not found.
	ErrSettingsNotFound = fmt.Errorf("settings file not found")
)

// ParseClaudeSettings reads a Claude settings.json file and extracts permissions.
// Returns nil permissions (no error) if the file does not exist or has no permissions key.
func ParseClaudeSettings(path string) (*ClaudePermissions, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrSettingsNotFound
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var settings claudeSettingsFile
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return settings.Permissions, nil
}

// settingsPath is one candidate Claude settings file: where to find it, what priority
// its derived rules get, and a human label used for reload-origin tagging.
type settingsPath struct {
	path     string
	priority int
	label    string
}

// settingsPaths returns the candidate Claude settings file paths for projectDir, resolved
// through symlinks and deduplicated. Search order:
//  1. ~/.claude/settings.json (global)
//  2. ~/.claude/settings.local.json (global local overrides)
//  3. <projectDir>/.claude/settings.json (project)
//  4. <projectDir>/.claude/settings.local.json (project local)
//
// Symlink resolution (resolveSettingsPathOrOriginal) matters for a symlinked project-level
// settings file (monorepo pattern) — without it, fsnotify would watch the symlink's parent
// directory instead of the real file's, and silently stop firing after the first edit.
//
// Deduplication by resolved path matters because the real deployed systemd unit runs with
// WorkingDirectory=$HOME (see scripts/install-service.sh), so projectDir == home and the
// "global" and "project" entries above resolve to the identical file. Without dedup, that
// file's rules would be loaded twice and reload-origin tagging would misreport a benign
// single-file edit as origin=mixed. Global entries are listed first, so when a project path
// collides with a global one, the surviving entry keeps the global label/priority.
func settingsPaths(projectDir string) []settingsPath {
	home, _ := os.UserHomeDir()

	paths := []settingsPath{
		{resolveSettingsPathOrOriginal(filepath.Join(home, ".claude", "settings.json")), 150, "global"},
		{resolveSettingsPathOrOriginal(filepath.Join(home, ".claude", "settings.local.json")), 160, "global-local"},
	}
	if projectDir != "" {
		paths = append(paths,
			settingsPath{resolveSettingsPathOrOriginal(filepath.Join(projectDir, ".claude", "settings.json")), 170, "project"},
			settingsPath{resolveSettingsPathOrOriginal(filepath.Join(projectDir, ".claude", "settings.local.json")), 180, "project-local"},
		)
	}

	seen := make(map[string]bool, len(paths))
	deduped := make([]settingsPath, 0, len(paths))
	for _, p := range paths {
		if seen[p.path] {
			continue
		}
		seen[p.path] = true
		deduped = append(deduped, p)
	}
	return deduped
}

// resolveSettingsPathOrOriginal resolves path through any symlinks so a watcher targets the
// real file's parent directory. On a resolution error (e.g. the file doesn't exist yet, or a
// broken symlink), it logs and falls back to the original path unchanged — mirrors
// config/defaults.go's evalSymlinksOrOriginal, which is unexported there and not importable
// from this package.
func resolveSettingsPathOrOriginal(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return resolved
}

// ClaudeSettingsPathResult is one settings file's parse outcome: its resolved path, the
// priority/label its rules were assigned, the rules themselves, and any parse error. Letting
// a caller (the watcher) see per-path errors means one malformed file doesn't wipe rules
// successfully loaded from another.
type ClaudeSettingsPathResult struct {
	Path     string
	Priority int
	Label    string
	Rules    []classifier.Rule
	Err      error
}

// LoadClaudeSettingsRulesDetailed parses each candidate Claude settings file independently
// and returns one ClaudeSettingsPathResult per path, so a parse failure in one file doesn't
// prevent rules from being returned for the others. See settingsPaths for search order.
func LoadClaudeSettingsRulesDetailed(projectDir string) []ClaudeSettingsPathResult {
	results := make([]ClaudeSettingsPathResult, 0, 4)
	for _, p := range settingsPaths(projectDir) {
		perms, err := ParseClaudeSettings(p.path)
		if err != nil {
			if err != ErrSettingsNotFound {
				log.Warn("[ClaudeSettings] failed to parse settings file", "path", p.path, "err", err)
			}
			results = append(results, ClaudeSettingsPathResult{Path: p.path, Priority: p.priority, Label: p.label, Err: err})
			continue
		}
		var rules []classifier.Rule
		if perms != nil && len(perms.Allow) > 0 {
			rules = claudeAllowsToRules(perms.Allow, p.priority, p.label)
			log.Info("[ClaudeSettings] loaded allow rules", "count", len(rules), "path", p.path)
		}
		results = append(results, ClaudeSettingsPathResult{Path: p.path, Priority: p.priority, Label: p.label, Rules: rules})
	}
	return results
}

// LoadClaudeSettingsRules parses both the global and project-level Claude settings and
// returns AutoAllow rules derived from their permissions.allow lists, flattened into one
// slice. Project settings take precedence: if both define the same tool pattern, the project
// rule will be checked first due to higher priority. Per-path parse errors are logged (via
// LoadClaudeSettingsRulesDetailed) and otherwise ignored — a caller that needs to distinguish
// which path failed should call LoadClaudeSettingsRulesDetailed directly.
func LoadClaudeSettingsRules(projectDir string) []classifier.Rule {
	var allRules []classifier.Rule
	for _, result := range LoadClaudeSettingsRulesDetailed(projectDir) {
		allRules = append(allRules, result.Rules...)
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
func claudeAllowsToRules(allows []string, basePriority int, label string) []classifier.Rule {
	var rules []classifier.Rule
	for i, pattern := range allows {
		rule := classifier.Rule{
			ID:        fmt.Sprintf("claude-settings-%s-%d", label, i),
			Name:      fmt.Sprintf("Claude settings allow: %s", pattern),
			Decision:  classifier.AutoAllow,
			RiskLevel: classifier.RiskLow,
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
				log.Warn("[ClaudeSettings] skipping invalid pattern", "pattern", pattern, "err", err)
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
