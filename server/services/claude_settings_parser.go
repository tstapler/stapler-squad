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

// ClaudePermissions mirrors the "permissions" key in ~/.claude/settings.json. Both Allow and
// Deny are converted into classifier.Rules (see claudeAllowsToRules/claudeDeniesToRules) and
// enforced by the live classifier — Deny is not merely parsed and displayed.
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
	// #nosec G304 -- path always comes from settingsPaths(), built from os.UserHomeDir()
	// or the server's own projectDir plus literal ".claude/settings*.json" suffixes; never
	// network/RPC input.
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
// real file's parent directory. Falls back to the original path unchanged on any resolution
// error (missing file, broken symlink) — mirrors config/defaults.go's evalSymlinksOrOriginal,
// unexported there and not importable from this package.
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
			allowRules := claudeAllowsToRules(perms.Allow, p.priority, p.label)
			rules = append(rules, allowRules...)
			log.Info("[ClaudeSettings] loaded allow rules", "count", len(allowRules), "path", p.path)
		}
		if perms != nil && len(perms.Deny) > 0 {
			denyRules := claudeDeniesToRules(perms.Deny, p.priority, p.label)
			rules = append(rules, denyRules...)
			log.Info("[ClaudeSettings] loaded deny rules", "count", len(denyRules), "path", p.path)
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

// claudeSettingsDenyPriorityOffset lifts claude-settings Deny rules close to the seed rule
// engine's hard-critical AutoDeny tier (see pkg/classifier/classifier.go's SeedRules doc
// comment: "1000 — AutoDeny (critical, must fire before any allow)") without colliding with
// it. A Deny entry must outrank every claude-settings Allow rule (Priority 150-180, see
// settingsPaths) and every non-critical seed tier below it — Escalate-before-allow (500) and
// AutoAllow-before-escalate (510-525) — so a user's explicit deny actually blocks a command
// their own (or another) settings file separately allows. It must NOT outrank the seed's own
// hard-critical AutoDeny rules (rm -rf /, .env writes, force-push, etc. — all pinned at
// exactly 1000): those exist regardless of user configuration, and a settings.json deny
// entry should never be able to weaken them. 750 puts every label's deny rule (900-930)
// safely inside the (525, 1000) gap.
const claudeSettingsDenyPriorityOffset = 750

// claudeAllowsToRules converts Claude's allow patterns to AutoAllow rules.
//
// Claude allow/deny patterns have the form:
//   - "Bash"           -- match any Bash invocation
//   - "Bash(git log*)" -- match Bash where command starts with "git log"
//   - "Read"           -- match any Read invocation
//
// Glob wildcards (*) are converted to regex (.*).
func claudeAllowsToRules(allows []string, basePriority int, label string) []classifier.Rule {
	return claudePatternsToRules(allows, basePriority, label, "claude-settings",
		classifier.AutoAllow, classifier.RiskLow, "allow", "Allowed")
}

// claudeDeniesToRules converts Claude's deny patterns (permissions.deny in settings.json)
// to AutoDeny rules, so a deny entry actually blocks matching tool calls instead of being
// parsed and silently ignored (see ClaudePermissions.Deny's doc comment). Same pattern
// syntax as claudeAllowsToRules; see claudeSettingsDenyPriorityOffset for why these rules
// get a different, higher priority than the allow rules from the same file.
func claudeDeniesToRules(denies []string, basePriority int, label string) []classifier.Rule {
	return claudePatternsToRules(denies, basePriority+claudeSettingsDenyPriorityOffset, label, "claude-settings-deny",
		classifier.AutoDeny, classifier.RiskHigh, "deny", "Denied")
}

// claudePatternsToRules is the shared pattern-parsing implementation behind
// claudeAllowsToRules and claudeDeniesToRules.
func claudePatternsToRules(patterns []string, priority int, label, idPrefix string, decision classifier.ClassificationDecision, riskLevel classifier.RiskLevel, verb, verbPast string) []classifier.Rule {
	var rules []classifier.Rule
	for i, pattern := range patterns {
		rule := classifier.Rule{
			ID:        fmt.Sprintf("%s-%s-%d", idPrefix, label, i),
			Name:      fmt.Sprintf("Claude settings %s: %s", verb, pattern),
			Decision:  decision,
			RiskLevel: riskLevel,
			Reason:    fmt.Sprintf("%s by Claude settings (%s): %s", verbPast, label, pattern),
			Priority:  priority,
			Enabled:   true,
			Source:    string(classifier.SourceClaudeSettings),
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

// globToRegex converts a simple glob pattern to a regex string, anchored at both ends unless
// the pattern ends in *. Only * is supported (matches any sequence of characters).
//
// The end anchor matters for security: classifier.Rule.CommandPattern is matched via
// MatchString, which succeeds on any substring match, not just a full match. Without a `$`,
// "git status" (no trailing *) would compile to "^git status" and auto-allow any command with
// that literal prefix — including "git status && rm -rf ~" — turning an intended exact-match
// allow rule into an unbounded prefix match. Found in review: this was pre-existing dead code
// (LoadClaudeSettingsRules had zero call sites) that this project activates for the first
// time, so the bug had no live security impact until now.
func globToRegex(glob string) string {
	escaped := regexp.QuoteMeta(glob)
	pattern := strings.ReplaceAll(escaped, `\*`, `.*`)
	if !strings.HasSuffix(glob, "*") {
		pattern += "$"
	}
	return "^" + pattern
}
