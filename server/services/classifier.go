package services

import (
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// RiskLevel indicates the severity of a tool use request.
type RiskLevel int

const (
	RiskLow RiskLevel = iota
	RiskMedium
	RiskHigh
	RiskCritical
)

// ClassificationDecision is the action taken by the classifier.
type ClassificationDecision int

const (
	// AutoAllow bypasses the manual review queue and immediately allows the request.
	AutoAllow ClassificationDecision = iota
	// AutoDeny immediately denies the request, optionally suggesting an alternative.
	AutoDeny
	// Escalate sends the request to the manual review queue for human review.
	Escalate
)

// ClassificationResult holds the outcome of classifying a tool use request.
type ClassificationResult struct {
	Decision    ClassificationDecision
	RiskLevel   RiskLevel
	Reason      string
	Alternative string
	RuleID      string
	RuleName    string
}

// ClassificationContext provides local-environment context to the classifier.
type ClassificationContext struct {
	Cwd        string
	IsGitRepo  bool
	RepoRoot   string
	IsWorktree bool
}

// Classifier classifies a PermissionRequestPayload to determine the action to take.
type Classifier interface {
	Classify(payload PermissionRequestPayload, ctx ClassificationContext) ClassificationResult
	BuildContext(cwd string) ClassificationContext
}

// ToolCategory constants classify tool names into coarse groups for use in Rule.ToolCategory.
// This lets seed rules match whole classes of tools without fragile long regex patterns.
const (
	// ToolCategoryAny matches any tool (empty string — default behaviour).
	ToolCategoryAny = ""
	// ToolCategoryBuiltin matches any Claude Code built-in tool (no "__" in name).
	// Examples: Bash, Read, Write, Edit, Glob, Grep, Task, WebFetch, WebSearch, ToolSearch.
	ToolCategoryBuiltin = "builtin"
	// ToolCategoryBuiltinAgent matches planning / task-management built-ins that pose no risk.
	// Examples: ExitPlanMode, EnterPlanMode, AskUserQuestion, TodoWrite, Task*, Skill, NotebookEdit.
	ToolCategoryBuiltinAgent = "builtin-agent"
	// ToolCategoryMCP matches any MCP tool (name contains "__").
	ToolCategoryMCP = "mcp"
	// ToolCategoryMCPRead matches MCP tools whose operation names are read-only.
	// Determined by CategorizeToolName; covers context7, sequential-thinking, and
	// filesystem/repomix read operations.
	ToolCategoryMCPRead = "mcp-read"
	// ToolCategoryMCPWrite matches MCP tools whose operation names mutate state.
	ToolCategoryMCPWrite = "mcp-write"
)

// builtinAgentTools is the set of Claude Code tool names that are planning / task-management
// tools with no side effects requiring review.
var builtinAgentTools = map[string]bool{
	"exitplanmode": true, "enterplanmode": true, "askuserquestion": true,
	"todowrite": true, "taskcreate": true, "taskupdate": true, "taskget": true,
	"tasklist": true, "taskoutput": true, "taskstop": true,
	"notebookedit": true, "skill": true,
}

// mcpReadOperations is the set of operation suffixes (the part after the second "__") that are
// considered read-only for MCP tools. Used by CategorizeToolName.
var mcpReadOperations = map[string]bool{
	// filesystem
	"read_file": true, "read_text_file": true, "read_media_file": true,
	"read_multiple_files": true, "list_directory": true, "list_directory_with_sizes": true,
	"directory_tree": true, "get_file_info": true, "list_allowed_directories": true,
	"search_files": true,
	// repomix — pack_remote_repository only fetches a remote repo's contents, no mutations
	"read_repomix_output": true, "grep_repomix_output": true, "attach_packed_output": true,
	"pack_remote_repository": true,
	// context7 — all operations are read-only
	"resolve-library-id": true, "query-docs": true,
	// sequential-thinking — pure reasoning, no side effects
	"sequentialthinking": true,
	// playwright — observation-only operations (no clicks, inputs, or code execution)
	"browser_take_screenshot": true, "browser_snapshot": true,
	"browser_network_requests": true, "browser_console_messages": true,
	"browser_tabs": true,
}

// CategorizeToolName returns the ToolCategory constant for a given tool name.
// The classification uses Claude Code naming conventions:
//   - MCP tools follow the pattern "mcp__<server>__<operation>" (contains "__").
//   - Built-in tools never contain "__".
//   - Agent tools are a named subset of built-ins.
func CategorizeToolName(name string) string {
	lower := strings.ToLower(name)
	if !strings.Contains(lower, "__") {
		// Built-in tool.
		if builtinAgentTools[lower] {
			return ToolCategoryBuiltinAgent
		}
		return ToolCategoryBuiltin
	}
	// MCP tool: mcp__<server>__<operation>
	parts := strings.SplitN(lower, "__", 3)
	if len(parts) == 3 {
		op := parts[2]
		if mcpReadOperations[op] {
			return ToolCategoryMCPRead
		}
		return ToolCategoryMCPWrite
	}
	return ToolCategoryMCP
}

// CommandCriteria provides structured, composable matching criteria for Bash commands.
// It is evaluated against a ParsedCommand and allows precise rules without complex regex.
// When multiple fields are set, all must match (AND semantics).
type CommandCriteria struct {
	// Programs lists the allowed primary programs. Empty means any program matches.
	// Prefix matching handles versioned interpreters (e.g., "python3" matches "python3.11").
	Programs []string
	// Subcommands lists allowed subcommand values. Empty means any (or no) subcommand matches.
	// For deep-subcommand programs (gh, aws, etc.) multi-word entries are supported ("pr view").
	Subcommands []string
	// BlockedSubcommands lists subcommands that prevent this rule from matching.
	BlockedSubcommands []string
	// RequiredFlags: at least one of the listed flags must be present in the command args.
	// Uses exact token matching (e.g., RequiredFlags: ["--hard"] matches git reset --hard only).
	RequiredFlags []string
	// RequiredFlagPrefixes: like RequiredFlags but uses prefix matching.
	// Useful when a flag accepts an optional inline value (e.g., sed -i.bak satisfies prefix "-i").
	RequiredFlagPrefixes []string
	// ForbiddenFlags: if any of these flags appear in args, the rule does not match.
	ForbiddenFlags []string
	// PythonModes restricts matching to specific Python invocation modes.
	// Valid values: "inline" (-c), "module" (-m), "version" (-V/--version), "script" (*.py).
	// Empty means no Python-mode check is performed.
	PythonModes []string
}

// Matches returns true if pc satisfies all criteria fields.
func (cc *CommandCriteria) Matches(pc ParsedCommand) bool {
	// Programs check.
	if len(cc.Programs) > 0 && !matchesProgram(cc.Programs, pc.Program) {
		return false
	}

	// Extract subcommand, correctly skipping prefix flags (e.g., git -C <path>).
	sub := extractSubcommand(pc.Program, pc.Args)

	// Subcommands allow-list.
	// Prefix matching handles programs in deepSubcommandPrograms (e.g., docker) where
	// trailing positional args (container names, image names) may be captured as an
	// extra subcommand token. "logs my-container" matches rule entry "logs".
	if len(cc.Subcommands) > 0 {
		found := false
		for _, s := range cc.Subcommands {
			if sub == s || strings.HasPrefix(sub, s+" ") {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// BlockedSubcommands deny-list.
	for _, bs := range cc.BlockedSubcommands {
		if sub == bs {
			return false
		}
	}

	// RequiredFlags: at least one must be present in args (exact match).
	if len(cc.RequiredFlags) > 0 {
		found := false
	outer:
		for _, rf := range cc.RequiredFlags {
			for _, arg := range pc.Args {
				if arg == rf {
					found = true
					break outer
				}
			}
		}
		if !found {
			return false
		}
	}

	// RequiredFlagPrefixes: at least one arg must have one of the listed prefixes.
	if len(cc.RequiredFlagPrefixes) > 0 {
		found := false
	outerPrefix:
		for _, prefix := range cc.RequiredFlagPrefixes {
			for _, arg := range pc.Args {
				if strings.HasPrefix(arg, prefix) {
					found = true
					break outerPrefix
				}
			}
		}
		if !found {
			return false
		}
	}

	// ForbiddenFlags: none may be present in args.
	for _, ff := range cc.ForbiddenFlags {
		for _, arg := range pc.Args {
			if arg == ff {
				return false
			}
		}
	}

	// PythonModes check.
	if len(cc.PythonModes) > 0 {
		mode := detectPythonMode(pc.Program, pc.Args)
		found := false
		for _, pm := range cc.PythonModes {
			if mode == pm {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// Rule is a single classification rule evaluated against a tool use request.
type Rule struct {
	ID   string
	Name string
	// ToolName is an exact match on the tool name (case-insensitive). If non-empty, ToolPattern is ignored.
	ToolName string
	// ToolPattern matches against the tool name when ToolName is empty.
	ToolPattern *regexp.Regexp
	// ToolCategory matches against the structural category returned by CategorizeToolName.
	// Evaluated after ToolName/ToolPattern (those take precedence when non-empty).
	// Use one of the ToolCategory* constants. Empty string means any category matches.
	ToolCategory string
	// Criteria provides structured matching for Bash command programs, subcommands and flags.
	// When set alongside CommandPattern, both must match (AND semantics).
	Criteria *CommandCriteria
	// CommandPattern matches against tool_input["command"]. nil means any command matches.
	CommandPattern *regexp.Regexp
	// FilePattern matches against tool_input["file_path"]. nil means any file path matches.
	FilePattern *regexp.Regexp
	Decision    ClassificationDecision
	RiskLevel   RiskLevel
	Reason      string
	Alternative string
	// Priority determines rule evaluation order. Higher values are evaluated first.
	Priority int
	Enabled  bool
	// Source tracks how the rule was loaded: "seed", "user", or "claude-settings".
	Source string
}

// RuleBasedClassifier evaluates a priority-ordered list of Rules.
type RuleBasedClassifier struct {
	mu    sync.RWMutex
	rules []Rule // sorted by Priority descending
}

// NewRuleBasedClassifier creates a classifier pre-loaded with seed rules.
func NewRuleBasedClassifier() *RuleBasedClassifier {
	rules := SeedRules()
	sort.Slice(rules, func(i, j int) bool { return rules[i].Priority > rules[j].Priority })
	return &RuleBasedClassifier{rules: rules}
}

// ReplaceRules atomically replaces all rules with the provided list.
func (c *RuleBasedClassifier) ReplaceRules(rules []Rule) {
	sorted := make([]Rule, len(rules))
	copy(sorted, rules)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Priority > sorted[j].Priority })
	c.mu.Lock()
	c.rules = sorted
	c.mu.Unlock()
}

// AddRules appends additional rules and re-sorts by priority.
func (c *RuleBasedClassifier) AddRules(rules []Rule) {
	c.mu.Lock()
	c.rules = append(c.rules, rules...)
	sort.Slice(c.rules, func(i, j int) bool { return c.rules[i].Priority > c.rules[j].Priority })
	c.mu.Unlock()
}

// Rules returns a copy of the current rule set.
func (c *RuleBasedClassifier) Rules() []Rule {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Rule, len(c.rules))
	copy(out, c.rules)
	return out
}

// Classify evaluates rules in priority order and returns the first match.
// For Bash commands, compound commands (with &&, |, ;, $(), etc.) are evaluated
// using classifyCompound to ensure every sub-command is covered.
// If no rule matches, returns Escalate for human review.
func (c *RuleBasedClassifier) Classify(payload PermissionRequestPayload, ctx ClassificationContext) ClassificationResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if strings.EqualFold(payload.ToolName, "Bash") {
		cmd, _ := payload.ToolInput["command"].(string)
		if cmd != "" {
			cmds := ExtractAllCommands(cmd)
			if len(cmds) > 1 {
				return c.classifyCompound(payload, cmds)
			}
		}
	}

	return c.classifySingle(payload)
}

// classifySingle evaluates rules against a single (non-compound) payload.
func (c *RuleBasedClassifier) classifySingle(payload PermissionRequestPayload) ClassificationResult {
	for _, rule := range c.rules {
		if !rule.Enabled {
			continue
		}
		if c.matchesRule(rule, payload) {
			return ClassificationResult{
				Decision:    rule.Decision,
				RiskLevel:   rule.RiskLevel,
				Reason:      rule.Reason,
				Alternative: rule.Alternative,
				RuleID:      rule.ID,
				RuleName:    rule.Name,
			}
		}
	}
	return ClassificationResult{
		Decision:  Escalate,
		RiskLevel: RiskMedium,
		Reason:    "No matching rule; escalated for manual review.",
	}
}

// classifyCompound evaluates each sub-command extracted from a compound Bash command.
// Pass 1: any deny/escalate decision on any sub-command wins immediately.
// Pass 2: every sub-command must be matched by an allow rule; otherwise escalate.
func (c *RuleBasedClassifier) classifyCompound(payload PermissionRequestPayload, cmds []ParsedCommand) ClassificationResult {
	// Pass 1: deny/escalate takes priority.
	for _, sub := range cmds {
		subPayload := payloadWithCommand(payload, sub.Raw)
		for _, rule := range c.rules {
			if !rule.Enabled {
				continue
			}
			if c.matchesRule(rule, subPayload) {
				if rule.Decision == AutoDeny || rule.Decision == Escalate {
					return ClassificationResult{
						Decision:    rule.Decision,
						RiskLevel:   rule.RiskLevel,
						Reason:      fmt.Sprintf("Sub-command %q matched: %s", sub.Raw, rule.Reason),
						Alternative: rule.Alternative,
						RuleID:      rule.ID,
						RuleName:    rule.Name,
					}
				}
				// Allow found — stop checking rules for this sub-command.
				break
			}
		}
	}

	// Pass 2: every sub-command must be covered by an allow rule.
	var firstAllowRule *Rule
	for _, sub := range cmds {
		subPayload := payloadWithCommand(payload, sub.Raw)
		covered := false
		for i, rule := range c.rules {
			if !rule.Enabled {
				continue
			}
			if rule.Decision == AutoAllow && c.matchesRule(rule, subPayload) {
				covered = true
				if firstAllowRule == nil {
					firstAllowRule = &c.rules[i]
				}
				break
			}
		}
		if !covered {
			return ClassificationResult{
				Decision:  Escalate,
				RiskLevel: RiskMedium,
				Reason:    fmt.Sprintf("Sub-command %q has no matching allow rule; escalated for manual review.", sub.Raw),
			}
		}
	}

	if firstAllowRule != nil {
		return ClassificationResult{
			Decision:    AutoAllow,
			RiskLevel:   firstAllowRule.RiskLevel,
			Reason:      firstAllowRule.Reason,
			Alternative: firstAllowRule.Alternative,
			RuleID:      firstAllowRule.ID,
			RuleName:    firstAllowRule.Name,
		}
	}
	return ClassificationResult{
		Decision:  AutoAllow,
		RiskLevel: RiskLow,
		Reason:    "All sub-commands covered by allow rules.",
	}
}

// payloadWithCommand returns a shallow copy of payload with tool_input["command"] replaced.
func payloadWithCommand(payload PermissionRequestPayload, cmd string) PermissionRequestPayload {
	newInput := make(map[string]interface{}, len(payload.ToolInput))
	for k, v := range payload.ToolInput {
		newInput[k] = v
	}
	newInput["command"] = cmd
	return PermissionRequestPayload{
		ToolName:  payload.ToolName,
		ToolInput: newInput,
	}
}

// BuildContext detects git repository state for the given working directory.
func (c *RuleBasedClassifier) BuildContext(cwd string) ClassificationContext {
	ctx := ClassificationContext{Cwd: cwd}
	if cwd == "" {
		return ctx
	}
	if out, err := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel").Output(); err == nil {
		ctx.RepoRoot = strings.TrimSpace(string(out))
		ctx.IsGitRepo = true
	}
	if ctx.IsGitRepo {
		if out, err := exec.Command("git", "-C", cwd, "rev-parse", "--git-dir").Output(); err == nil {
			ctx.IsWorktree = strings.Contains(string(out), "worktrees")
		}
	}
	return ctx
}

// matchesRule returns true if all non-nil criteria in rule match the payload.
func (c *RuleBasedClassifier) matchesRule(rule Rule, payload PermissionRequestPayload) bool {
	// Tool name / pattern / category match.
	if rule.ToolName != "" {
		if !strings.EqualFold(payload.ToolName, rule.ToolName) {
			return false
		}
	} else if rule.ToolPattern != nil {
		if !rule.ToolPattern.MatchString(payload.ToolName) {
			return false
		}
	} else if rule.ToolCategory != "" {
		cat := CategorizeToolName(payload.ToolName)
		if cat != rule.ToolCategory {
			// ToolCategoryBuiltinAgent is a sub-category of ToolCategoryBuiltin.
			// A rule targeting "builtin" should also match agent tools.
			if !(rule.ToolCategory == ToolCategoryBuiltin && cat == ToolCategoryBuiltinAgent) {
				return false
			}
		}
	}

	cmd, _ := payload.ToolInput["command"].(string)
	if rule.CommandPattern != nil {
		if !rule.CommandPattern.MatchString(cmd) {
			return false
		}
	}

	// Structured criteria matching: parse the command and evaluate against Criteria.
	if rule.Criteria != nil {
		cmds := ExtractAllCommands(cmd)
		if len(cmds) == 0 {
			return false
		}
		if !rule.Criteria.Matches(cmds[0]) {
			return false
		}
	}

	filePath, _ := payload.ToolInput["file_path"].(string)
	if rule.FilePattern != nil {
		if !rule.FilePattern.MatchString(filePath) {
			return false
		}
	}

	return true
}

// SeedRules returns the built-in rule set, sorted by Priority descending.
// Priority tiers:
//
//	1000 — AutoDeny (critical, must fire before any allow)
//	 500 — Escalate-before-allow (targeted escalations that override allow rules at 100)
//	 100 — AutoAllow (standard development operations)
//	  50 — Escalate catch-all (operations with no allow rule; provides a helpful reason)
//
// Criteria-based rules provide precise matching without complex regex;
// CommandPattern is retained only where regex expressiveness is needed.
func SeedRules() []Rule {
	return []Rule{

		// ══════════════════════════════════════════════════════════════════════════
		// AutoDeny (Priority 1000) — checked before all allow rules
		// ══════════════════════════════════════════════════════════════════════════

		{
			ID:          "seed-deny-env-write",
			Name:        "Block writes to .env files",
			ToolPattern: regexp.MustCompile(`(?i)^(Write|Edit|MultiEdit)$`),
			FilePattern: regexp.MustCompile(`(^|/)\.env(\.|$)`),
			Decision:    AutoDeny,
			RiskLevel:   RiskCritical,
			Reason:      "Writing to .env files risks leaking or corrupting secrets.",
			Alternative: "Use environment variable management tools or a secrets manager instead.",
			Priority:    1000,
			Enabled:     true,
			Source:      "seed",
		},
		{
			ID:          "seed-deny-git-internals-write",
			Name:        "Block writes to .git internals",
			ToolPattern: regexp.MustCompile(`(?i)^(Write|Edit|MultiEdit)$`),
			FilePattern: regexp.MustCompile(`(^|/)\.git/`),
			Decision:    AutoDeny,
			RiskLevel:   RiskCritical,
			Reason:      "Directly modifying .git internals can corrupt the repository.",
			Alternative: "Use git commands (git commit, git branch, etc.) instead.",
			Priority:    1000,
			Enabled:     true,
			Source:      "seed",
		},
		{
			ID:             "seed-deny-rm-rf-root",
			Name:           "Block rm -rf on root or home paths",
			ToolName:       "Bash",
			CommandPattern: regexp.MustCompile(`rm\s+(-[a-zA-Z]*r[a-zA-Z]*f|-[a-zA-Z]*f[a-zA-Z]*r)\s+(/|~|\$HOME)`),
			Decision:       AutoDeny,
			RiskLevel:      RiskCritical,
			Reason:         "Deleting the root or home directory would cause irreversible data loss.",
			Alternative:    "Specify a precise subdirectory path instead.",
			Priority:       1000,
			Enabled:        true,
			Source:         "seed",
		},
		{
			ID:             "seed-deny-find-exec",
			Name:           "Block find with -exec/-delete/-ok",
			ToolName:       "Bash",
			CommandPattern: regexp.MustCompile(`find\s+.*(-(exec|delete|ok)\b|--delete\b)`),
			Decision:       AutoDeny,
			RiskLevel:      RiskHigh,
			Reason:         "find with -exec/-delete/-ok can execute arbitrary commands or delete files.",
			Alternative:    "Use the Glob tool for file pattern matching, or review the find command before running.",
			Priority:       1000,
			Enabled:        true,
			Source:         "seed",
		},
		{
			// Catches shell redirections that write to .env files, e.g.:
			//   echo "SECRET=x" >> .env
			//   cat config > .env.local
			//   printf "KEY=val" > /path/.env
			// The Write/Edit deny rule covers tool-based writes; this covers Bash redirects.
			ID:             "seed-deny-bash-redirect-env",
			Name:           "Block shell redirects to .env files",
			ToolName:       "Bash",
			CommandPattern: regexp.MustCompile(`>>?\s*\S*\.env(\s|$|[.'":])`),
			Decision:       AutoDeny,
			RiskLevel:      RiskCritical,
			Reason:         "Redirecting output to .env files risks corrupting or leaking secrets.",
			Alternative:    "Use environment variable management tools or a secrets manager instead.",
			Priority:       1000,
			Enabled:        true,
			Source:         "seed",
		},
		{
			// Deny git reset --hard: destructive and hard to undo.
			// git reset HEAD~1 (without --hard) remains allowed by seed-allow-git-write.
			ID:       "seed-deny-git-reset-hard",
			Name:     "Block git reset --hard",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:      []string{"git"},
				Subcommands:   []string{"reset"},
				RequiredFlags: []string{"--hard"},
			},
			Decision:    AutoDeny,
			RiskLevel:   RiskHigh,
			Reason:      "git reset --hard discards uncommitted changes and cannot be undone.",
			Alternative: "Use git stash to save changes, or git reset HEAD~1 to keep changes staged.",
			Priority:    1000,
			Enabled:     true,
			Source:      "seed",
		},
		{
			// Deny git push --force / -f: can overwrite remote history and destroy others' work.
			// --force-with-lease is NOT blocked here (safer); it escalates via seed-escalate-git-push.
			ID:       "seed-deny-git-push-force",
			Name:     "Block git push --force / -f",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:      []string{"git"},
				Subcommands:   []string{"push"},
				RequiredFlags: []string{"--force", "-f"},
			},
			Decision:    AutoDeny,
			RiskLevel:   RiskCritical,
			Reason:      "Force-pushing can overwrite remote history and destroy collaborators' work.",
			Alternative: "Use --force-with-lease for a safer force push, or coordinate with your team first.",
			Priority:    1000,
			Enabled:     true,
			Source:      "seed",
		},
		{
			// git branch -D force-deletes regardless of merge status, losing commits that
			// aren't reachable from another ref. Recoverable via reflog but risky.
			ID:       "seed-deny-git-branch-force-delete",
			Name:     "Block git branch -D (force delete)",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:      []string{"git"},
				Subcommands:   []string{"branch"},
				RequiredFlags: []string{"-D"},
			},
			Decision:    AutoDeny,
			RiskLevel:   RiskHigh,
			Reason:      "git branch -D force-deletes a branch even if it has unmerged commits.",
			Alternative: "Use git branch -d to safely delete only merged branches.",
			Priority:    1000,
			Enabled:     true,
			Source:      "seed",
		},

		// ══════════════════════════════════════════════════════════════════════════
		// Escalate-before-allow (Priority 500) — override the allow rules at 100
		// ══════════════════════════════════════════════════════════════════════════

		{
			// git branch -d / --delete only removes merged branches (safer than -D), but
			// branch deletion is still a write operation that should be reviewed.
			// The allow rule at 100 handles read-only branch operations (git branch, git branch -a).
			ID:       "seed-escalate-git-branch-safe-delete",
			Name:     "Escalate git branch -d (safe delete)",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:      []string{"git"},
				Subcommands:   []string{"branch"},
				RequiredFlags: []string{"-d", "--delete"},
			},
			Decision:    Escalate,
			RiskLevel:   RiskMedium,
			Reason:      "Branch deletion modifies repository structure and should be reviewed.",
			Alternative: "Confirm the branch is fully merged before deleting: git branch --merged",
			Priority:    500,
			Enabled:     true,
			Source:      "seed",
		},
		{
			// sed -i edits files in place; sed without -i is read-only (stdout only).
			// RequiredFlagPrefixes matches both `-i` (GNU) and `-i.bak` / `-i ''` (macOS/BSD)
			// since all in-place variants begin with the `-i` prefix.
			// The allow rule at 100 handles read-only sed invocations.
			ID:       "seed-escalate-sed-inplace",
			Name:     "Escalate sed -i (in-place editing)",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:             []string{"sed"},
				RequiredFlagPrefixes: []string{"-i"},
			},
			Decision:    Escalate,
			RiskLevel:   RiskMedium,
			Reason:      "sed -i modifies files in place; mistakes can corrupt source files.",
			Alternative: "Use the Edit tool for safe, reversible file modifications.",
			Priority:    500,
			Enabled:     true,
			Source:      "seed",
		},
		{
			// npm operations that publish to the registry or manage credentials.
			// Plain npm install/test/run remain AutoAllow via seed-allow-bash-npm at 100.
			ID:       "seed-escalate-npm-publish",
			Name:     "Escalate npm publish and credential operations",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:    []string{"npm"},
				Subcommands: []string{"publish", "adduser", "login", "logout", "unpublish", "deprecate"},
			},
			Decision:    Escalate,
			RiskLevel:   RiskHigh,
			Reason:      "npm publish/credential operations affect the public registry and should be reviewed.",
			Alternative: "Confirm the package version, changelog, and access settings before publishing.",
			Priority:    500,
			Enabled:     true,
			Source:      "seed",
		},
		{
			// cargo publish pushes crates to crates.io. cargo login stores credentials.
			// Standard cargo build/test/run remain AutoAllow via seed-allow-bash-cargo at 100.
			ID:       "seed-escalate-cargo-publish",
			Name:     "Escalate cargo publish and credential operations",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:    []string{"cargo"},
				Subcommands: []string{"publish", "login", "logout", "owner", "yank"},
			},
			Decision:    Escalate,
			RiskLevel:   RiskHigh,
			Reason:      "cargo publish/credential operations affect crates.io and should be reviewed.",
			Alternative: "Confirm the crate version and access settings before publishing.",
			Priority:    500,
			Enabled:     true,
			Source:      "seed",
		},
		{
			// gh api covers both REST (gh api repos/...) and GraphQL (gh api graphql).
			// Read operations like gh pr view are auto-allowed at 100 via seed-allow-bash-gh-read.
			// Write GH CLI operations (pr create, issue create, etc.) are caught by seed-escalate-gh-write below.
			// This rule catches the lower-level API calls that can do arbitrary reads or writes.
			ID:       "seed-escalate-gh-api",
			Name:     "Escalate gh api calls",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:    []string{"gh"},
				Subcommands: []string{"api"},
			},
			Decision:    Escalate,
			RiskLevel:   RiskMedium,
			Reason:      "gh api calls can modify GitHub resources and should be reviewed.",
			Alternative: "Use Python subprocess([\"gh\", \"api\", ...]) for unattended gh api calls; it bypasses the Bash tool approval handler.",
			Priority:    500,
			Enabled:     true,
			Source:      "seed",
		},
		{
			// Covers high-level gh CLI write commands. Read operations (pr view, pr list, etc.)
			// are auto-allowed by seed-allow-bash-gh-read at priority 100.
			ID:       "seed-escalate-gh-write",
			Name:     "Escalate gh write operations",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"gh"},
				Subcommands: []string{
					"pr create", "pr comment", "pr merge", "pr close", "pr edit", "pr reopen", "pr review",
					"issue create", "issue close", "issue edit", "issue comment",
					"repo create", "repo delete", "repo fork",
					"release create", "release delete", "release upload",
				},
			},
			Decision:  Escalate,
			RiskLevel: RiskMedium,
			Reason:    "gh write operations modify GitHub resources and should be reviewed.",
			Priority:  500,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// curl with file output flags (-o/-O/--output) writes response bodies to disk.
			// Must fire at 500 to override seed-allow-curl-read at 100.
			ID:             "seed-escalate-curl-output",
			Name:           "Escalate curl with file output flags",
			ToolName:       "Bash",
			CommandPattern: regexp.MustCompile(`\bcurl\b.*\s(-[a-zA-Z]*[oO]|--(output|remote-name))\b`),
			Decision:       Escalate,
			RiskLevel:      RiskMedium,
			Reason:         "curl -o/-O downloads a file to disk and should be reviewed.",
			Alternative:    "Review the URL and destination path before downloading.",
			Priority:       500,
			Enabled:        true,
			Source:         "seed",
		},
		{
			// curl with write HTTP methods can modify remote state.
			// Must fire at 500 to override seed-allow-curl-read at 100.
			// Note: -X alone (e.g. -X GET) is harmless but rare; we conservatively escalate any -X.
			ID:             "seed-escalate-curl-write-method",
			Name:           "Escalate curl write HTTP methods (POST/PUT/DELETE/PATCH)",
			ToolName:       "Bash",
			CommandPattern: regexp.MustCompile(`\bcurl\b.*(\s-X\s|\s--request\s|\s--data\b|\s-d\s|\s--data-raw\b|\s--data-binary\b|\s--upload-file\b|\s-T\s|\s-F\s|\s--form\s)`),
			Decision:       Escalate,
			RiskLevel:      RiskHigh,
			Reason:         "curl with write methods or request bodies can modify remote state and should be reviewed.",
			Priority:       500,
			Enabled:        true,
			Source:         "seed",
		},

		// ══════════════════════════════════════════════════════════════════════════
		// AutoAllow (Priority 100) — standard development operations
		// ══════════════════════════════════════════════════════════════════════════

		{
			ID:          "seed-allow-read-tools",
			Name:        "Allow read-only tools",
			ToolPattern: regexp.MustCompile(`(?i)^(Read|Glob|Grep|WebFetch|WebSearch|ListMcpResourcesTool|ReadMcpResourceTool)$`),
			Decision:    AutoAllow,
			RiskLevel:   RiskLow,
			Reason:      "Read-only operations pose no risk.",
			Priority:    100,
			Enabled:     true,
			Source:      "seed",
		},
		{
			// Note: "env" is intentionally excluded. `env` is a wrapper command
			// (e.g., `env git reset --hard`) and including it would bypass deny rules
			// because ExtractAllCommands sets Program="env" for wrapped invocations.
			ID:       "seed-allow-bash-ls-pwd",
			Name:     "Allow ls, pwd, echo, and inspection commands",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"ls", "pwd", "echo", "printenv", "which", "type", "date", "whoami", "id", "hostname"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Listing and inspection commands are read-only.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// find without -exec/-delete is read-only; the deny rule catches dangerous patterns.
			ID:       "seed-bash-find-name",
			Name:     "Allow find (no exec/delete)",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"find"},
			},
			Decision:    AutoAllow,
			RiskLevel:   RiskLow,
			Reason:      "Simple find is read-only.",
			Alternative: "Use the Glob tool for file pattern matching instead.",
			Priority:    100,
			Enabled:     true,
			Source:      "seed",
		},
		{
			ID:       "seed-allow-bash-cat-read",
			Name:     "Allow cat, head, tail, wc, file, stat",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"cat", "head", "tail", "wc", "file", "stat", "less", "more", "diff", "md5sum", "sha256sum"},
			},
			Decision:    AutoAllow,
			RiskLevel:   RiskLow,
			Reason:      "Read-only file inspection commands.",
			Alternative: "Consider using the Read or Grep tools for file inspection.",
			Priority:    100,
			Enabled:     true,
			Source:      "seed",
		},
		{
			// cat > /tmp/... << 'EOF' is a common Claude Code pattern for writing temp scripts,
			// queries, or helper files to /tmp before execution. Writing to /tmp is low-risk
			// (ephemeral, world-readable) and should not require manual review.
			ID:             "seed-allow-bash-cat-tmp-write",
			Name:           "Allow cat heredoc writes to /tmp",
			ToolName:       "Bash",
			CommandPattern: regexp.MustCompile(`\bcat\s*>+\s*/tmp/`),
			Decision:       AutoAllow,
			RiskLevel:      RiskLow,
			Reason:         "Writing temporary files to /tmp is ephemeral and low-risk.",
			Priority:       100,
			Enabled:        true,
			Source:         "seed",
		},
		{
			// Criteria-based matching correctly handles git -C <path> <subcmd> by skipping
			// the -C flag and its value before extracting the subcommand.
			// Note: "branch" is included here for listing (git branch, git branch -a).
			// The deny/escalate rules at higher priority handle -D and -d deletion.
			ID:       "seed-allow-git-read",
			Name:     "Allow read-only git commands",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"git"},
				Subcommands: []string{
					"status", "log", "diff", "show", "branch", "remote",
					"fetch", "tag", "describe", "rev-parse", "ls-files",
					"shortlog", "blame", "stash", "worktree",
				},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Read-only git operations pose no risk.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			ID:          "seed-allow-file-tools",
			Name:        "Allow core file editing tools",
			ToolPattern: regexp.MustCompile(`(?i)^(Edit|Write|MultiEdit)$`),
			Decision:    AutoAllow,
			RiskLevel:   RiskLow,
			Reason:      "Core Claude Code file editing tools; .env and .git deny rules protect critical paths.",
			Priority:    100,
			Enabled:     true,
			Source:      "seed",
		},
		{
			ID:       "seed-allow-bash-cd",
			Name:     "Allow cd/pushd/popd",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"cd", "pushd", "popd"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Shell navigation commands have no side effects.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			ID:       "seed-allow-bash-mkdir",
			Name:     "Allow mkdir",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"mkdir"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Directory creation is low risk.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			ID:       "seed-allow-bash-grep",
			Name:     "Allow grep/rg/ag",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"grep", "egrep", "fgrep", "rg", "ag"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Text search commands are read-only.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// Criteria-based matching correctly handles git -C <path> <subcmd>.
			// "pull" is included: it is fetch+merge and part of standard workflow.
			// "push" is intentionally excluded — it escalates via seed-escalate-git-push.
			ID:       "seed-allow-git-write",
			Name:     "Allow standard git write operations",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"git"},
				Subcommands: []string{
					"add", "commit", "checkout", "switch", "stash", "pull",
					"merge", "rebase", "restore", "reset",
				},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Standard git development workflow; push remains escalated.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			ID:       "seed-allow-bash-sleep",
			Name:     "Allow sleep",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"sleep"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "sleep waits for a duration and has no side effects.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			ID:       "seed-allow-bash-go-safe",
			Name:     "Allow safe go subcommands",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:    []string{"go"},
				Subcommands: []string{"build", "test", "run", "fmt", "vet", "mod", "list", "env", "version", "clean", "generate", "tool"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Standard Go toolchain operations.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// Matches python/python3/python3.11/pypy/pypy3 running a script, module, or version check.
			// python -c "..." (inline) is intentionally excluded → escalates for review.
			ID:       "seed-allow-bash-python-run",
			Name:     "Allow python running a script or module",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:    []string{"python", "python2", "python3", "pypy", "pypy3"},
				PythonModes: []string{"script", "module", "version"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Python running a project script or module. Inline -c execution escalates for review.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			ID:       "seed-allow-bash-pytest",
			Name:     "Allow pytest test runner",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"pytest"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "pytest runs project tests.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// Only known pip subcommands are allowed; arbitrary invocations escalate.
			ID:       "seed-allow-bash-pip",
			Name:     "Allow pip package management subcommands",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:    []string{"pip", "pip3"},
				Subcommands: []string{"install", "uninstall", "list", "show", "freeze", "check", "download", "cache", "hash", "config", "wheel"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Standard pip package management operations.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// Known uv subcommands; compound analysis still enforces safety on piped/chained commands.
			ID:       "seed-allow-bash-uv",
			Name:     "Allow uv package manager subcommands",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:    []string{"uv"},
				Subcommands: []string{"run", "sync", "pip", "lock", "add", "remove", "init", "python", "tool", "venv", "export", "tree", "cache", "build", "publish"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "uv package manager standard operations.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// Note: "sed" is included here for read-only pipeline use (stdout only).
			// The escalate rule at 500 catches "sed -i" (in-place editing) first.
			ID:       "seed-allow-bash-text-proc",
			Name:     "Allow text processing tools",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"jq", "awk", "tr", "sort", "uniq", "cut", "paste", "column", "tee", "sed"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Text processing and pipeline tools.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// ExtractAllCommands strips the ./ path prefix, so "./gradlew" → program "gradlew".
			ID:       "seed-allow-bash-gradlew",
			Name:     "Allow Gradle build tool",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"gradlew", "gradle"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Gradle/Gradlew is a standard JVM build tool.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			ID:       "seed-allow-bash-node-tools",
			Name:     "Allow Node.js runtime and TypeScript tools",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"node", "tsc", "ts-node", "tsx"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Node.js runtime and TypeScript compiler for project code.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// publish/adduser/login/logout are escalated at priority 500 before this rule fires.
			ID:       "seed-allow-bash-npm",
			Name:     "Allow npm, npx, yarn, pnpm",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"npm", "npx", "yarn", "pnpm"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Node.js package management and script execution.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			ID:       "seed-allow-bash-make",
			Name:     "Allow make",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"make"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Make is a standard build tool for running project tasks.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			ID:       "seed-allow-bash-file-ops",
			Name:     "Allow cp, mv, touch, ln",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"cp", "mv", "touch", "ln"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Standard file management operations.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// Deep subcommand matching: extractSubcommand captures 2 tokens for gh.
			// gh api and write operations are escalated at priority 500 before this rule fires.
			ID:       "seed-allow-bash-gh-read",
			Name:     "Allow read-only GitHub CLI commands",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"gh"},
				Subcommands: []string{
					"pr view", "pr list", "pr show", "pr status", "pr checks", "pr diff",
					"issue view", "issue list", "issue show",
					"run view", "run list", "run log", "run watch",
					"release view", "release list",
					"repo view", "repo list",
					"workflow view", "workflow list",
					"auth status",
				},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Read-only GitHub CLI operations.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// publish/login/logout/owner/yank are escalated at priority 500 before this rule fires.
			ID:       "seed-allow-bash-cargo",
			Name:     "Allow Rust cargo build subcommands",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"cargo"},
				Subcommands: []string{
					"build", "test", "run", "fmt", "clippy", "check", "doc",
					"clean", "bench", "update", "tree", "search", "fix",
					"fetch", "vendor", "metadata", "install", "uninstall",
					"generate-lockfile", "verify-project",
				},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Standard Rust toolchain operations.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// mvn and the ./mvnw wrapper (path-stripped to "mvnw" by ExtractAllCommands).
			// All lifecycle phases (compile, test, package, verify, install) are allowed.
			// "deploy" (remote repository upload) is intentionally omitted — escalates by default.
			ID:       "seed-allow-bash-mvn",
			Name:     "Allow Maven build operations",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"mvn", "mvnw"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Maven build lifecycle operations for Java projects.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// Covers both legacy (docker ps) and modern (docker container ls) subcommand forms.
			// docker is in deepSubcommandPrograms, so 2-token subcommands are captured.
			ID:       "seed-allow-bash-docker-read",
			Name:     "Allow read-only Docker commands",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"docker"},
				Subcommands: []string{
					// Legacy 1-level subcommands
					"ps", "images", "logs", "inspect", "info", "version",
					"stats", "top", "diff", "history", "events",
					// Modern container subcommands
					"container ls", "container list", "container ps",
					"container inspect", "container logs",
					"container stats", "container top", "container diff",
					// Modern image subcommands
					"image ls", "image list", "image inspect", "image history",
					// System subcommands
					"system info", "system df", "system events",
					// Network/volume read
					"network ls", "network list", "network inspect",
					"volume ls", "volume list", "volume inspect",
				},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Read-only Docker inspection commands.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},

		{
			// Core Claude Code agent interaction and task management tools.
			// These tools pose no risk (they ask questions, manage task lists, or signal
			// plan approval) and should never require manual review.
			// Uses ToolCategory so new agent tools are auto-matched without rule updates.
			ID:           "seed-allow-agent-tools",
			Name:         "Allow Claude Code agent and planning tools",
			ToolCategory: ToolCategoryBuiltinAgent,
			Decision:     AutoAllow,
			RiskLevel:    RiskLow,
			Reason:       "Core Claude Code agent interaction and task management tools.",
			Priority:     100,
			Enabled:      true,
			Source:       "seed",
		},
		{
			// MCP read-only tools: filesystem reads, documentation lookup, sequential thinking,
			// and codebase analysis output reading. Write/mutate MCP tools are excluded and escalate.
			// Uses ToolCategory so newly registered read-only MCP operations are auto-matched.
			ID:           "seed-allow-mcp-read",
			Name:         "Allow read-only MCP tools",
			ToolCategory: ToolCategoryMCPRead,
			Decision:     AutoAllow,
			RiskLevel:    RiskLow,
			Reason:       "Read-only MCP tools pose no risk.",
			Priority:     100,
			Enabled:      true,
			Source:       "seed",
		},
		{
			// curl read-only: GET requests without file output or write methods.
			// The 500-priority rules (seed-escalate-curl-output, seed-escalate-curl-write-method)
			// intercept unsafe curl invocations before this rule fires.
			ID:       "seed-allow-curl-read",
			Name:     "Allow curl read-only (GET, no file output)",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"curl"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "curl GET requests without output flags or write methods are read-only.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},

		// ══════════════════════════════════════════════════════════════════════════
		// Escalate catch-all (Priority 50) — no allow rule exists; provides a reason
		// ══════════════════════════════════════════════════════════════════════════

		{
			// All git push operations escalate; force pushes are denied at priority 1000.
			ID:       "seed-escalate-git-push",
			Name:     "Escalate git push",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:    []string{"git"},
				Subcommands: []string{"push"},
			},
			Decision:  Escalate,
			RiskLevel: RiskHigh,
			Reason:    "git push modifies remote state and should be reviewed.",
			Priority:  50,
			Enabled:   true,
			Source:    "seed",
		},
		{
			ID:             "seed-escalate-network-write",
			Name:           "Escalate curl/wget with output flags",
			ToolName:       "Bash",
			CommandPattern: regexp.MustCompile(`^\s*(curl|wget)\s+.*(-o\s|-O\s|--output)`),
			Decision:       Escalate,
			RiskLevel:      RiskHigh,
			Reason:         "Downloading files to disk should be reviewed.",
			Priority:       50,
			Enabled:        true,
			Source:         "seed",
		},
		{
			ID:       "seed-escalate-brew",
			Name:     "Escalate Homebrew package management",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"brew"},
			},
			Decision:    Escalate,
			RiskLevel:   RiskMedium,
			Reason:      "Homebrew operations install or modify system-level packages.",
			Alternative: "Review the package and its dependencies before installing.",
			Priority:    50,
			Enabled:     true,
			Source:      "seed",
		},
		{
			ID:       "seed-escalate-chmod-chown",
			Name:     "Escalate chmod/chown",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"chmod", "chown"},
			},
			Decision:    Escalate,
			RiskLevel:   RiskMedium,
			Reason:      "Changing file permissions or ownership can affect system security.",
			Alternative: "Confirm the intended permissions and target files before proceeding.",
			Priority:    50,
			Enabled:     true,
			Source:      "seed",
		},
		{
			// docker exec runs commands inside containers; docker run creates and starts new
			// containers; docker compose manages multi-container stacks; docker rm/stop/kill
			// mutate container state. The read-only commands are allowed at 100 by
			// seed-allow-bash-docker-read.
			ID:       "seed-escalate-docker-write",
			Name:     "Escalate docker container lifecycle and execution operations",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"docker"},
				Subcommands: []string{
					// Execution
					"exec", "run", "attach",
					// Container lifecycle
					"rm", "stop", "start", "restart", "kill", "pause", "unpause", "rename", "update",
					// Compose
					"compose",
					// Modern container subcommands
					"container rm", "container stop", "container start",
					"container restart", "container kill", "container exec", "container run",
					"container prune",
					// Image write
					"build", "pull", "push", "tag", "import", "load", "save",
					"image build", "image pull", "image push", "image tag", "image rm", "image prune",
					// System
					"system prune",
					// Network/volume write
					"network create", "network rm", "network prune", "network connect", "network disconnect",
					"volume create", "volume rm", "volume prune",
				},
			},
			Decision:    Escalate,
			RiskLevel:   RiskMedium,
			Reason:      "docker operations that create, modify, execute in, or remove containers should be reviewed.",
			Alternative: "Review the container configuration and command before proceeding.",
			Priority:    50,
			Enabled:     true,
			Source:      "seed",
		},
	}
}
