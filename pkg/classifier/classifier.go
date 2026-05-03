package classifier

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
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

// PermissionRequestPayload is the JSON payload from Claude Code's PermissionRequest HTTP hook.
type PermissionRequestPayload struct {
	SessionID      string                 `json:"session_id"`
	TranscriptPath string                 `json:"transcript_path"`
	Cwd            string                 `json:"cwd"`
	PermissionMode string                 `json:"permission_mode"`
	HookEventName  string                 `json:"hook_event_name"`
	ToolName       string                 `json:"tool_name"`
	ToolInput      map[string]interface{} `json:"tool_input"`
}

// ClassificationContext provides local-environment context to the classifier.
type ClassificationContext struct {
	Cwd        string
	IsGitRepo  bool
	RepoRoot   string
	IsWorktree bool
	// Env is an optional map of environment variable names to values used to expand
	// $VAR and ${VAR} references in Bash commands before classification. This is
	// primarily useful in tests and CI to evaluate commands that contain dynamic
	// variables (e.g. BRANCH=main "git checkout $BRANCH" → "git checkout main").
	// Variables not present in the map are left unexpanded.
	Env map[string]string
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
	// SafePythonImportsOnly restricts inline Python (-c) matches to commands whose
	// import statements use only known-safe stdlib modules (see safeStdlibModules).
	// When true the rule will not match if any import is outside the safelist, or if
	// the invocation is not inline. Combine with PythonModes: ["inline"].
	SafePythonImportsOnly bool
	// RedirectionPattern matches against any file paths targeted by shell redirections.
	RedirectionPattern *regexp.Regexp
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

	// SafePythonImportsOnly: inline Python is safe only when every import is from the
	// curated stdlib safelist AND the code avoids dangerous builtin call patterns.
	if cc.SafePythonImportsOnly {
		pyInfo := ParsePythonCommand(pc.Raw)
		if !pyInfo.IsInline {
			return false
		}
		// Require at least one import statement — bare code (e.g. print('hello'))
		// uses builtins whose safety cannot be determined without deeper analysis.
		if len(pyInfo.Imports) == 0 {
			return false
		}
		// All imports must be from the curated safelist.
		for _, imp := range pyInfo.Imports {
			if !safeStdlibModules[imp] {
				return false
			}
		}
		// Reject code that contains dangerous call patterns even if imports are safe.
		// Covers: dangerous builtins (eval, exec, open, …) and pathlib write methods
		// (.write_text, .unlink, .mkdir, …). See bannedInlinePythonPatterns for the
		// full list and the rationale for each entry.
		for _, banned := range bannedInlinePythonPatterns {
			if strings.Contains(pc.Raw, banned) {
				return false
			}
		}
	}

	// Redirection check.
	if cc.RedirectionPattern != nil {
		found := false
		for _, redir := range pc.Redirections {
			if cc.RedirectionPattern.MatchString(redir) {
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

// maxRecursionDepth limits how deeply recursive-eval wrappers (xargs, sudo, rtk, …)
// can be nested. Prevents infinite loops from pathological input like `xargs xargs xargs`.
const maxRecursionDepth = 5

// Classify acquires the read lock and evaluates payload against all rules.
// For Bash commands, compound commands (&&, |, ;, $(), etc.) are split and each
// sub-command evaluated independently. Recursive-eval programs (xargs, sudo, rtk, …)
// have their inner command extracted and classified through the full rule engine.
// If no rule matches, returns Escalate for human review.
func (c *RuleBasedClassifier) Classify(payload PermissionRequestPayload, ctx ClassificationContext) ClassificationResult {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.classifyInternal(payload, ctx, 0)
}

// classifyInternal is the lock-free recursive implementation. depth tracks the current
// recursion level to prevent infinite loops from chained recursive-eval programs.
// Must be called with c.mu at least RLocked.
func (c *RuleBasedClassifier) classifyInternal(payload PermissionRequestPayload, ctx ClassificationContext, depth int) ClassificationResult {
	if strings.EqualFold(payload.ToolName, "Bash") {
		cmd, _ := payload.ToolInput["command"].(string)
		if cmd != "" {
			// Expand $VAR / ${VAR} references, mirroring Python os.path.expandvars:
			// ctx.Env overrides take precedence, then the real OS environment is used
			// as a fallback. Only applied when ctx.Env is non-nil (opt-in).
			if ctx.Env != nil {
				cmd = ExpandEnvVars(cmd, ctx.Env)
				payload = payloadWithCommand(payload, cmd)
			}

			// Deep security audit via AST analysis — runs on every level.
			findings := AuditCommand(cmd, ctx.Cwd)
			for _, f := range findings {
				if f.RiskLevel == RiskCritical {
					return ClassificationResult{
						Decision:    AutoDeny,
						RiskLevel:   f.RiskLevel,
						Reason:      f.Reason,
						Alternative: f.Alternative,
						RuleID:      f.ID,
						RuleName:    f.Name,
					}
				}
			}

			cmds := ExtractAllCommands(cmd)
			if len(cmds) > 1 {
				return c.classifyCompound(payload, cmds, ctx, depth)
			}

			// Single command: if it is a recursive-eval wrapper (xargs, sudo, rtk, …),
			// extract the inner command and classify it through the full rule engine.
			if len(cmds) == 1 && depth < maxRecursionDepth {
				pc := cmds[0]

				// Shell expansion as program: $VAR or $(cmd) after path-stripping means
				// the actual executable cannot be determined statically. Escalate with a
				// specific, actionable reason instead of the generic "no matching rule".
				if pc.HasShellExpansionProgram {
					return ClassificationResult{
						Decision:    Escalate,
						RiskLevel:   RiskMedium,
						Reason:      fmt.Sprintf("Command program is a shell expansion (%q); cannot determine the actual executable without evaluating the shell. Review the command before approving.", pc.Program),
						Alternative: "Use the concrete program name directly, or expand the variable in a separate step.",
						RuleID:      "shell-expansion-program",
					}
				}

				if innerCmd := ExtractInnerCommand(pc.Program, pc.Args); innerCmd != "" {
					return c.classifyInternal(payloadWithCommand(payload, innerCmd), ctx, depth+1)
				}
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

// classifyOneSubCmd classifies a single parsed sub-command from a compound expression.
// If the sub-command is a recursive-eval wrapper (xargs, sudo, rtk, …), the inner
// command is extracted and classified recursively. Otherwise, static rule matching is used.
func (c *RuleBasedClassifier) classifyOneSubCmd(sub ParsedCommand, payload PermissionRequestPayload, ctx ClassificationContext, depth int) ClassificationResult {
	// Shell expansion as program: always escalate with a specific reason regardless
	// of whether this sub-command came from the top level or a compound expression.
	if sub.HasShellExpansionProgram {
		return ClassificationResult{
			Decision:    Escalate,
			RiskLevel:   RiskMedium,
			Reason:      fmt.Sprintf("Command program is a shell expansion (%q); cannot determine the actual executable without evaluating the shell. Review the command before approving.", sub.Program),
			Alternative: "Use the concrete program name directly, or expand the variable in a separate step.",
			RuleID:      "shell-expansion-program",
		}
	}
	if depth < maxRecursionDepth {
		if innerCmd := ExtractInnerCommand(sub.Program, sub.Args); innerCmd != "" {
			return c.classifyInternal(payloadWithCommand(payload, innerCmd), ctx, depth+1)
		}
	}
	return c.classifySingle(payloadWithCommand(payload, sub.Raw))
}

// classifyCompound evaluates each sub-command extracted from a compound Bash command.
// Recursive-eval wrappers (xargs, sudo, rtk, …) have their inner command extracted and
// classified through the full rule engine.
//
// Commands extracted from inside $(...) substitutions (FromCmdSubst=true) are treated
// differently: they produce argument values for the outer command rather than executing
// independently. The rules are:
//   - Pass 1 (deny/escalate): CmdSubst inner commands skip catch-all escalations (RuleID=="")
//     but still block on explicit deny/escalate rules — e.g. make $(git push) still escalates.
//   - Pass 2 (require AutoAllow): CmdSubst inner commands are exempt; only top-level
//     commands must have an explicit AutoAllow rule.
func (c *RuleBasedClassifier) classifyCompound(payload PermissionRequestPayload, cmds []ParsedCommand, ctx ClassificationContext, depth int) ClassificationResult {
	// Pass 1: deny/escalate takes priority.
	for _, sub := range cmds {
		result := c.classifyOneSubCmd(sub, payload, ctx, depth)
		if result.Decision == AutoDeny || result.Decision == Escalate {
			// CmdSubst inner commands: only block on explicit rules, not catch-all escalations.
			// Catch-all escalations (RuleID=="" and reason="No matching rule…") mean the inner
			// command is simply unknown — not dangerous. E.g. make $(TARGET) should not escalate
			// because TARGET has no seed rule; it's just a value producer.
			if sub.FromCmdSubst && result.RuleID == "" && result.Decision == Escalate {
				continue
			}
			return ClassificationResult{
				Decision:    result.Decision,
				RiskLevel:   result.RiskLevel,
				Reason:      fmt.Sprintf("Sub-command %q: %s", sub.Raw, result.Reason),
				Alternative: result.Alternative,
				RuleID:      result.RuleID,
				RuleName:    result.RuleName,
			}
		}
	}

	// Pass 2: every top-level sub-command must produce AutoAllow.
	// CmdSubst inner commands are exempt — they are argument producers, not independent commands.
	var firstResult *ClassificationResult
	for _, sub := range cmds {
		if sub.FromCmdSubst {
			continue // exempt from AutoAllow requirement
		}
		result := c.classifyOneSubCmd(sub, payload, ctx, depth)
		if result.Decision != AutoAllow {
			return ClassificationResult{
				Decision:  Escalate,
				RiskLevel: RiskMedium,
				Reason:    fmt.Sprintf("Sub-command %q has no matching allow rule; escalated for manual review.", sub.Raw),
			}
		}
		if firstResult == nil {
			r := result
			firstResult = &r
		}
	}

	if firstResult != nil {
		return *firstResult
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

// BuildContext detects git repository state for the given working directory and
// populates Env from the current OS environment so that simple variable expansions
// ($VAR, ${VAR}) in Bash commands are resolved before classification. This lets
// commands like "$RUSTC --version" expand to "rustc --version" and match seed rules
// instead of hitting the generic HasShellExpansionProgram escalation path.
func (c *RuleBasedClassifier) BuildContext(cwd string) ClassificationContext {
	ctx := ClassificationContext{Cwd: cwd}

	// Populate Env from the current process environment.
	envSlice := os.Environ()
	ctx.Env = make(map[string]string, len(envSlice))
	for _, kv := range envSlice {
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			ctx.Env[kv[:idx]] = kv[idx+1:]
		}
	}

	if cwd == "" {
		return ctx
	}
	gitCtx, gitCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer gitCancel()
	toplevelCmd := exec.CommandContext(gitCtx, "git", "-C", cwd, "rev-parse", "--show-toplevel")
	toplevelCmd.WaitDelay = 2 * time.Second
	if out, err := toplevelCmd.Output(); err == nil {
		ctx.RepoRoot = strings.TrimSpace(string(out))
		ctx.IsGitRepo = true
	}
	if ctx.IsGitRepo {
		gitDirCtx, gitDirCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer gitDirCancel()
		gitDirCmd := exec.CommandContext(gitDirCtx, "git", "-C", cwd, "rev-parse", "--git-dir")
		gitDirCmd.WaitDelay = 2 * time.Second
		if out, err := gitDirCmd.Output(); err == nil {
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
			if rule.ToolCategory != ToolCategoryBuiltin || cat != ToolCategoryBuiltinAgent {
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
			CommandPattern: regexp.MustCompile(`rm\s+(-[a-zA-Z]*r[a-zA-Z]*f|-[a-zA-Z]*f[a-zA-Z]*r)\s+(/|~|\$HOME)/?(\s|$)`),
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
		// AutoAllow-before-escalate (Priority 525/520/515/510) — targeted allow/escalate
		// overrides for the generic gh api escalation at priority 500.
		// 525: safety guard — escalate .../replies with destructive methods (pre-empts 520)
		// 520: allow — specific known-safe writes (PR reply, resolveReviewThread mutation)
		// 515: safety guard — escalate gh api calls with ANY -f/-F/--field flag or explicit
		//      HTTP write method; catches combos like `gh api ... -f title=x --jq '...'`
		// 510: allow — read-only gh api patterns (--jq, --paginate); safe because 515
		//      already blocked any -f/-F/--field before reaching here
		// ══════════════════════════════════════════════════════════════════════════

		{
			// Guard against using a destructive HTTP method on the replies endpoint.
			// The 520 allow rule below permits -f body= replies (POST), but DELETE/PUT/
			// PATCH on /replies would remove or alter existing comments. This rule fires
			// first (525 > 520) to block those cases.
			ID:             "seed-escalate-gh-api-pr-review-replies-write",
			Name:           "Escalate gh api replies endpoint with destructive HTTP method",
			ToolName:       "Bash",
			CommandPattern: regexp.MustCompile(`\bgh\s+api\b.*\brepos/[^/\s]+/[^/\s]+/pulls/[^/\s]+/comments/[^/\s]+/replies\b.*(\s-X\s+(DELETE|PUT|PATCH)\b|\s--method\s+(DELETE|PUT|PATCH)\b|\s--input\b)`),
			Decision:       Escalate,
			RiskLevel:      RiskMedium,
			Reason:         "Using a destructive HTTP method on the PR review replies endpoint can modify or delete existing comments.",
			Priority:       525,
			Enabled:        true,
			Source:         "seed",
		},
		{
			// Posting a reply to a PR review comment is a low-risk write: the only
			// effect is adding a text comment to a specific review thread. This is the
			// standard reply step in the address-review-comments skill workflow.
			// Must be at 520 (above the 515 guard) because it uses -f body=.
			// Matches both literal paths (repos/owner/repo/pulls/123/comments/456/replies)
			// and shell-variable forms (repos/$OWNER/$REPO/pulls/$PR_NUMBER/...).
			ID:             "seed-allow-gh-api-pr-review-replies",
			Name:           "Allow gh api to post PR review comment replies",
			ToolName:       "Bash",
			CommandPattern: regexp.MustCompile(`\bgh\s+api\b.*\brepos/[^/\s]+/[^/\s]+/pulls/[^/\s]+/comments/[^/\s]+/replies\b`),
			Decision:       AutoAllow,
			RiskLevel:      RiskLow,
			Reason:         "Posting replies to PR review comments is a standard PR review workflow operation.",
			Priority:       520,
			Enabled:        true,
			Source:         "seed",
		},
		{
			// Resolving a PR review thread marks it as done so it no longer blocks the
			// merge. The resolveReviewThread GraphQL mutation uses -f query=... which
			// would be caught by the 515 guard, so this must be at 520.
			ID:             "seed-allow-gh-api-graphql-resolve-thread",
			Name:           "Allow gh api graphql resolveReviewThread mutation",
			ToolName:       "Bash",
			CommandPattern: regexp.MustCompile(`\bgh\s+api\s+graphql\b.*\bresolveReviewThread\b`),
			Decision:       AutoAllow,
			RiskLevel:      RiskLow,
			Reason:         "Resolving a PR review thread is a standard PR review workflow operation.",
			Priority:       520,
			Enabled:        true,
			Source:         "seed",
		},
		{
			// Safety guard: escalate any gh api call that sends request body fields via
			// -f/-F/--field flags, uses an explicit write HTTP method (-X POST/PUT/DELETE/
			// PATCH or --method), or reads a body from a file (--input).
			// This prevents bypass via combos like `gh api ... -f title=x --jq '...'`
			// where --jq is a response filter but -f is still a write indicator.
			// The 510 --jq / --paginate rules are safe to assume GET semantics because
			// any -f/-F/--field flag is caught here first.
			ID:             "seed-escalate-gh-api-explicit-write",
			Name:           "Escalate gh api calls with field flags, write method, or --input",
			ToolName:       "Bash",
			CommandPattern: regexp.MustCompile(`\bgh\s+api\b.*(\s-X\s+(POST|PUT|DELETE|PATCH)\b|\s--method\s+(POST|PUT|DELETE|PATCH)\b|\s(-f|-F)\s|\s--field\s|\s--input\b)`),
			Decision:       Escalate,
			RiskLevel:      RiskMedium,
			Reason:         "gh api calls with field flags or explicit write methods can modify GitHub resources and should be reviewed.",
			Priority:       515,
			Enabled:        true,
			Source:         "seed",
		},
		{
			// gh api REST calls that include --jq are always GET + jq filter: --jq is a
			// response post-processor and has no effect on the HTTP method. Safe to
			// auto-allow here because the 515 guard above has already blocked any command
			// that also contains -f/-F/--field flags (which would indicate a write).
			// Covers the most common analytics pattern: gh api repos/.../X --jq '...'
			ID:             "seed-allow-gh-api-rest-jq",
			Name:           "Allow read-only gh api calls with --jq",
			ToolName:       "Bash",
			CommandPattern: regexp.MustCompile(`\bgh\s+api\b.*\s--jq\b`),
			Decision:       AutoAllow,
			RiskLevel:      RiskLow,
			Reason:         "gh api with --jq filters a GET response; without field flags or an explicit write method this is a read-only GitHub API operation.",
			Priority:       510,
			Enabled:        true,
			Source:         "seed",
		},
		{
			// gh api REST calls with --paginate read all pages of a resource; --paginate
			// has no HTTP method implication and is used exclusively for large reads.
			// Safe here because the 515 guard has blocked any -f/-F/--field combos.
			ID:             "seed-allow-gh-api-rest-paginate",
			Name:           "Allow read-only gh api calls with --paginate",
			ToolName:       "Bash",
			CommandPattern: regexp.MustCompile(`\bgh\s+api\b.*\s--paginate\b`),
			Decision:       AutoAllow,
			RiskLevel:      RiskLow,
			Reason:         "gh api --paginate reads all pages of a resource and is a read-only GitHub API operation.",
			Priority:       510,
			Enabled:        true,
			Source:         "seed",
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
					// CI/CD write operations.
					"workflow run",
					// Secrets and variables (write).
					"secret set", "secret delete",
					"env set", "env delete",
					"variable set", "variable delete",
					// Auth state changes.
					"auth login", "auth logout",
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
			// git config write flags modify repository or global settings. The allow rule at 100
			// includes "config" as a subcommand for read operations (--get, --list, bare reads).
			// This escalate fires first for any invocation that uses a write flag.
			ID:       "seed-escalate-git-config-write",
			Name:     "Escalate git config write operations",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:      []string{"git"},
				Subcommands:   []string{"config"},
				RequiredFlags: []string{"--unset", "--unset-all", "--add", "--replace-all", "--remove-section", "--rename-section"},
			},
			Decision:    Escalate,
			RiskLevel:   RiskMedium,
			Reason:      "git config with write flags modifies repository or global settings and should be reviewed.",
			Alternative: "Use git config --get or git config --list to inspect the current value first.",
			Priority:    500,
			Enabled:     true,
			Source:      "seed",
		},
		{
			// git filter-repo and filter-branch rewrite history, potentially discarding commits
			// and making backups mandatory. These must fire at 500 to override the git allow rules.
			ID:       "seed-escalate-git-filter-history",
			Name:     "Escalate git history-rewrite operations",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:    []string{"git"},
				Subcommands: []string{"filter-repo", "filter-branch"},
			},
			Decision:    Escalate,
			RiskLevel:   RiskHigh,
			Reason:      "git filter-repo/filter-branch rewrites history and cannot be undone without a backup.",
			Alternative: "Ensure a complete backup exists (e.g. git clone --mirror) before proceeding.",
			Priority:    500,
			Enabled:     true,
			Source:      "seed",
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
		// AutoAllow (Priority 101/100) — standard development operations
		// 101: targeted escalate guards that fire just before the 100 allow rules
		// 100: broad auto-allow for common development tools
		// ══════════════════════════════════════════════════════════════════════════

		{
			// base64 -o / --output writes decoded/encoded data directly to a file (BSD/macOS
			// semantics). All other base64 invocations write to stdout and are harmless
			// pipeline steps. This rule fires at 101, before text-proc allows base64 at 100.
			ID:             "seed-escalate-base64-file-output",
			Name:           "Escalate base64 with file output flag",
			ToolName:       "Bash",
			CommandPattern: regexp.MustCompile(`\bbase64\b.*\s(-o|--output)\s`),
			Decision:       Escalate,
			RiskLevel:      RiskMedium,
			Reason:         "base64 -o / --output writes to a file; use base64 -d to decode to stdout instead.",
			Priority:       101,
			Enabled:        true,
			Source:         "seed",
		},
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
					// Additional read-only plumbing and inspection subcommands.
					"merge-base", "merge-tree", "ls-tree", "grep", "check-ignore",
					"diff-tree", "cat-file", "for-each-ref", "count-objects",
					// Configuration and remote inspection.
					"config", "ls-remote",
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
			// "git reset --hard" is denied at 1000; plain "git reset" (e.g. HEAD~1) is allowed here.
			ID:       "seed-allow-git-write",
			Name:     "Allow standard git write operations",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"git"},
				Subcommands: []string{
					"add", "commit", "checkout", "switch", "stash", "pull",
					"merge", "rebase", "restore", "reset", "clone",
					// Additional standard workflow subcommands.
					"cherry-pick", "rm", "apply", "mv", "submodule",
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
				Subcommands: []string{"build", "test", "run", "fmt", "vet", "mod", "list", "env", "version", "clean", "generate", "tool", "install", "get", "work", "doc"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Standard Go toolchain operations.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			ID:       "seed-allow-bash-gofmt",
			Name:     "Allow gofmt (Go code formatter)",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"gofmt"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "gofmt is a read-only code formatter equivalent to go fmt.",
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
			// python -c "..." that only imports from the safe stdlib safelist (safeStdlibModules)
			// is auto-allowed. If any import falls outside the safelist the rule does not match,
			// and the escalate rule at priority 50 fires instead.
			// Examples that auto-allow: python3 -c "import json, sys; print(json.dumps(sys.argv))"
			// Examples that escalate: python3 -c "import requests; r = requests.get(url)"
			ID:       "seed-allow-python-inline-stdlib",
			Name:     "Allow python -c with stdlib-only imports",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:              []string{"python", "python2", "python3", "pypy", "pypy3"},
				PythonModes:           []string{"inline"},
				SafePythonImportsOnly: true,
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Inline Python using only safe stdlib modules poses no network or execution risk.",
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
			// "base64" encodes/decodes data (e.g. GitHub API returns file contents as
			// base64; decoding with "base64 -d" is a common pipeline step). The 101
			// escalate rule at the top of this section blocks the -o/--output variant.
			ID:       "seed-allow-bash-text-proc",
			Name:     "Allow text processing tools",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"jq", "awk", "tr", "sort", "uniq", "cut", "paste", "column", "tee", "sed", "base64"},
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
					"run view", "run list", "run log", "run watch", "run download", "run rerun",
					"release view", "release list",
					"repo view", "repo list",
					"workflow view", "workflow list",
					"auth status",
					"secret list",
					"label list",
					"milestone list",
					"codespace list",
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

		{
			// tmux is a terminal multiplexer used extensively in development workflows:
			// Only read-only tmux subcommands are auto-allowed. Subcommands like
			// new-session, run-shell, and send-keys can execute arbitrary shell code
			// and must not bypass the normal Bash rule evaluation.
			ID:       "seed-allow-bash-tmux",
			Name:     "Allow tmux read-only subcommands",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:    []string{"tmux"},
				Subcommands: []string{"list-sessions", "ls", "list-windows", "list-panes", "display-message", "show-options", "show-environment", "info", "has-session"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Read-only tmux queries pose no execution risk.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// javap disassembles Java .class files to show their bytecode and signatures.
			// It is a read-only inspection tool — no files are created or modified.
			ID:       "seed-allow-bash-javap",
			Name:     "Allow javap (Java bytecode disassembler)",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"javap"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "javap disassembles Java class files; it is a read-only inspection tool.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// jar is the standard Java archive tool used for creating, listing, and
			// extracting JARs/AARs/WARs. All forms (tf=list, xf=extract, cf=create) are
			// standard build-step operations. The deny rules protect .env and .git.
			ID:       "seed-allow-bash-jar",
			Name:     "Allow jar (Java archive tool)",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"jar"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "jar creates, lists, and extracts Java archives; a standard build step.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// unzip is commonly used in JVM/Android workflows to inspect AAR/APK archives
			// and in general for extracting downloaded packages.
			ID:       "seed-allow-bash-unzip",
			Name:     "Allow unzip archive extraction",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"unzip"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "unzip extracts archives; a standard build and inspection step.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// jest and vitest are the standard JavaScript/TypeScript test runners.
			// They mirror the pytest rule and should be auto-allowed.
			ID:       "seed-allow-bash-jest",
			Name:     "Allow jest and vitest test runners",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"jest", "vitest"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "jest/vitest runs JavaScript/TypeScript project tests.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// buf is the standard Protocol Buffer toolchain used for code generation,
			// linting, and format checking. All operations are local build steps.
			ID:       "seed-allow-bash-buf",
			Name:     "Allow buf Protocol Buffer tooling",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:    []string{"buf"},
				Subcommands: []string{"generate", "lint", "format", "build", "dep", "check", "breaking", "config", "registry", "beta", "alpha", "push"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "buf generates and validates Protocol Buffer definitions; a standard build step.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// jfr (Java Flight Recorder) CLI reads JVM recording files for profiling.
			// summary, print, view, metadata, and disassemble are all read-only operations.
			ID:       "seed-allow-bash-jfr",
			Name:     "Allow jfr (Java Flight Recorder) read-only commands",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:    []string{"jfr"},
				Subcommands: []string{"summary", "print", "view", "metadata", "assemble", "disassemble"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "jfr reads JVM flight recording files; summary and print are read-only.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// aapt/aapt2 is the Android Asset Packaging Tool used to inspect APKs.
			// dump, list, and version are read-only operations on the archive.
			ID:       "seed-allow-bash-aapt",
			Name:     "Allow aapt/aapt2 APK inspection",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:    []string{"aapt", "aapt2"},
				Subcommands: []string{"dump", "list", "version", "v"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "aapt dump reads APK metadata without modifying it.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// pixi is a conda-compatible package manager that creates isolated project
			// environments. Its standard operations mirror those of npm/pip.
			ID:       "seed-allow-bash-pixi",
			Name:     "Allow pixi package manager",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:    []string{"pixi"},
				Subcommands: []string{"run", "install", "add", "remove", "list", "search", "info", "init", "task", "shell", "update", "clean", "global", "auth"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "pixi manages isolated project environments (conda-compatible).",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// mktemp only creates a temporary filename/directory; it does not execute code
			// or write data. Cleanup is automatic on the next system reboot at latest.
			ID:       "seed-allow-bash-mktemp",
			Name:     "Allow mktemp",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"mktemp"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "mktemp creates temporary files/directories with no lasting side effects.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// Read-only systemctl queries check service state without modifying it.
			// Write operations (start, stop, restart, enable, daemon-reload, …) escalate
			// at priority 50 via seed-escalate-systemctl-write.
			ID:       "seed-allow-bash-systemctl-read",
			Name:     "Allow systemctl read-only operations",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:    []string{"systemctl"},
				Subcommands: []string{"status", "is-active", "is-enabled", "is-failed", "cat", "list-units", "list-unit-files", "list-sockets", "list-timers", "show"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Read-only systemctl commands check service state without modifying it.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// hugo is a static site generator. build/serve are standard local dev operations.
			ID:       "seed-allow-bash-hugo",
			Name:     "Allow hugo static site generator",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:    []string{"hugo"},
				Subcommands: []string{"build", "serve", "server", "new", "list", "version", "config", "gen", "convert", "completion"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "hugo is a static site generator; build and serve are standard dev operations.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// tailscale read-only queries inspect VPN status and routing without changing them.
			ID:       "seed-allow-bash-tailscale-read",
			Name:     "Allow tailscale read-only commands",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:    []string{"tailscale"},
				Subcommands: []string{"status", "ip", "dns", "ping", "netcheck", "version", "whois"},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Read-only tailscale commands inspect VPN state without modifying it.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},

		{
			// ip read-only: inspection commands (route, addr, link, neigh, rule) that
			// show current network state without modifying it.
			//
			// Because ip is in deepSubcommandPrograms, extractSubcommand captures two
			// tokens (e.g., "route show", "addr add"). BlockedSubcommands lists all
			// known write verb pairs so that ip route add, ip addr add, ip link set, etc.
			// do NOT match this rule and fall through to seed-escalate-ip-networking at 50.
			//
			// Bare invocations (ip route, ip addr) capture only one token ("route") which
			// is not blocked, so they auto-allow (bare = show).
			ID:       "seed-allow-bash-ip-read",
			Name:     "Allow ip read-only network inspection",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"ip"},
				BlockedSubcommands: []string{
					// route write ops
					"route add", "route del", "route delete", "route change",
					"route replace", "route flush", "route append",
					// addr write ops
					"addr add", "addr del", "addr delete", "addr change",
					"addr flush", "addr replace",
					// link write ops
					"link set", "link add", "link delete", "link change",
					// neigh write ops
					"neigh add", "neigh del", "neigh delete", "neigh change",
					"neigh flush", "neigh replace",
					// rule write ops
					"rule add", "rule del", "rule delete",
					// short-alias write ops (ip a, ip r, ip n, ip l)
					"a add", "a del", "a delete", "a change", "a flush",
					"r add", "r del", "r delete", "r change", "r replace", "r flush",
					"n add", "n del", "n delete", "n change", "n flush",
					"l set", "l add", "l delete", "l change",
				},
			},
			Decision:  AutoAllow,
			RiskLevel: RiskLow,
			Reason:    "Read-only ip commands inspect network state without modifying it.",
			Priority:  100,
			Enabled:   true,
			Source:    "seed",
		},
		{
			// pacman read-only: -Q (query installed packages), -F (file database query),
			// and -Ss/-Si (sync database search/info) are information-only operations.
			// All other -S modes (-S install, -Su upgrade, -Sy sync) and -R/-U/-D are
			// caught by seed-escalate-pacman at 50.
			//
			// Matches any -Q variant (-Qs, -Qi, -Ql, etc.), -F variants, and the two
			// safe -S sub-modes: -Ss (search) and -Si (show package info).
			ID:             "seed-allow-bash-pacman-query",
			Name:           "Allow pacman read-only query operations",
			ToolName:       "Bash",
			CommandPattern: regexp.MustCompile(`^pacman\s+(-Q[a-zA-Z]*\b|--query\b|-F[a-zA-Z]*\b|--files\b|-[Vh]\b|--version\b|--help\b|-Ss\b|-Si\b)`),
			Decision:       AutoAllow,
			RiskLevel:      RiskLow,
			Reason:         "pacman -Q, -F, -Ss, and -Si operations query packages without modifying them.",
			Priority:       100,
			Enabled:        true,
			Source:         "seed",
		},
		{
			// sqlite3 read-only: allow a safe subset of dot commands that only inspect
			// schema and metadata. SQL queries and DML (INSERT, UPDATE, DELETE) are not
			// matched and fall through to seed-escalate-sqlite3 at priority 50.
			//
			// Matched: .tables, .schema [table], .indexes [table], .databases, .pragma <name>
			// NOT matched: "SELECT ...", "INSERT INTO ...", bare invocations without dot cmds.
			ID:             "seed-allow-bash-sqlite3-read",
			Name:           "Allow sqlite3 read-only schema inspection",
			ToolName:       "Bash",
			CommandPattern: regexp.MustCompile(`\bsqlite3\b\s+\S+\s+["']?\.(tables|databases?|schema(\s+\w+)?|indexes?(\s+\w+)?|pragma\s+\w+)["']?\s*$`),
			Decision:       AutoAllow,
			RiskLevel:      RiskLow,
			Reason:         "sqlite3 dot commands (.tables, .schema, .indexes, .pragma) are read-only metadata inspection.",
			Priority:       100,
			Enabled:        true,
			Source:         "seed",
		},

		// ══════════════════════════════════════════════════════════════════════════
		// Escalate catch-all (Priority 50) — no allow rule exists; provides a reason
		// ══════════════════════════════════════════════════════════════════════════

		{
			// rm (without -rf on root/home) is not caught by the deny rule, but it still
			// deletes files permanently. Escalate with a helpful reason so reviewers can
			// confirm the target before proceeding.
			ID:       "seed-escalate-rm",
			Name:     "Escalate rm (file deletion)",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"rm", "rmdir"},
			},
			Decision:    Escalate,
			RiskLevel:   RiskMedium,
			Reason:      "rm deletes files permanently. Confirm the target path before proceeding.",
			Alternative: "Move the file to /tmp first if you want a recoverable deletion.",
			Priority:    50,
			Enabled:     true,
			Source:      "seed",
		},
		{
			// pkill/killall send signals to processes matched by name or pattern.
			// A broad pattern (pkill -f "gradle") can inadvertently kill unrelated processes.
			// `kill` is excluded: PID-targeted signals are safe and common in scripts.
			ID:       "seed-escalate-pkill",
			Name:     "Escalate pkill/killall (process termination by name)",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"pkill", "killall"},
			},
			Decision:    Escalate,
			RiskLevel:   RiskMedium,
			Reason:      "pkill/killall terminates processes by name and can accidentally affect unrelated processes.",
			Alternative: "Use 'pgrep <name>' to preview which PIDs would be affected before killing.",
			Priority:    50,
			Enabled:     true,
			Source:      "seed",
		},
		{
			// python3 -c with imports outside the stdlib safelist should be reviewed.
			// Inline code that only uses safe stdlib modules is auto-allowed at priority 100
			// by seed-allow-python-inline-stdlib. This rule catches the rest.
			ID:       "seed-escalate-python-inline",
			Name:     "Escalate python -c with non-stdlib imports",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:    []string{"python", "python2", "python3", "pypy", "pypy3"},
				PythonModes: []string{"inline"},
			},
			Decision:    Escalate,
			RiskLevel:   RiskMedium,
			Reason:      "python -c with non-stdlib imports (e.g. requests, httpx) can make network calls or execute arbitrary code.",
			Alternative: "Write the code to a .py file and run it with python script.py instead.",
			Priority:    50,
			Enabled:     true,
			Source:      "seed",
		},
		{
			// systemctl write operations modify service state. Read-only subcommands are
			// allowed at priority 100 by seed-allow-bash-systemctl-read.
			ID:       "seed-escalate-systemctl-write",
			Name:     "Escalate systemctl write operations",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs:    []string{"systemctl"},
				Subcommands: []string{"start", "stop", "restart", "reload", "enable", "disable", "mask", "unmask", "daemon-reload", "daemon-reexec", "reset-failed"},
			},
			Decision:    Escalate,
			RiskLevel:   RiskMedium,
			Reason:      "systemctl write operations modify service state and should be reviewed.",
			Alternative: "Confirm the service name and intended state change before proceeding.",
			Priority:    50,
			Enabled:     true,
			Source:      "seed",
		},
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
			// . (dot-source) is matched as a program name by the AST parser when it appears
			// as a command (e.g., `. ~/.bashrc`). This is distinct from . as an argument to
			// find or other programs, which would have Program: "find", not ".".
			ID:       "seed-escalate-source",
			Name:     "Escalate source/dot-source (shell script sourcing)",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"source", "."},
			},
			Decision:    Escalate,
			RiskLevel:   RiskMedium,
			Reason:      "Sourcing shell scripts executes arbitrary code in the current shell and modifies the environment.",
			Alternative: "For Python virtualenvs, use 'python -m venv .venv && .venv/bin/python' directly instead of sourcing activate scripts.",
			Priority:    50,
			Enabled:     true,
			Source:      "seed",
		},
		{
			ID:       "seed-escalate-asdf",
			Name:     "Escalate asdf (runtime version manager)",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"asdf"},
			},
			Decision:    Escalate,
			RiskLevel:   RiskMedium,
			Reason:      "asdf installs and activates language runtimes, modifying system-level tool state.",
			Alternative: "Review the asdf command and plugin before proceeding. Consider using project-local .tool-versions.",
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
			// sqlite3 is an interactive/batch database CLI. Queries can be read-only
			// (SELECT) or destructive (DROP, DELETE, INSERT). Escalate so the user can
			// confirm the query before it runs against a production database.
			ID:       "seed-escalate-sqlite3",
			Name:     "Escalate sqlite3 database CLI",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"sqlite3"},
			},
			Decision:    Escalate,
			RiskLevel:   RiskMedium,
			Reason:      "sqlite3 can read or modify databases; review the query before proceeding.",
			Alternative: "Add '.mode readonly' at the start of the script for safe inspection.",
			Priority:    50,
			Enabled:     true,
			Source:      "seed",
		},
		{
			// seed-escalate-xvfb-run removed: xvfb-run is now in recursiveEvalPrograms.
			// Its inner command is extracted and classified through the full rule engine.
			// pacman is the Arch Linux system package manager. Installing or removing
			// packages modifies system-wide state and should be reviewed.
			ID:       "seed-escalate-pacman",
			Name:     "Escalate pacman Arch package manager",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"pacman"},
			},
			Decision:    Escalate,
			RiskLevel:   RiskMedium,
			Reason:      "pacman installs or modifies system packages.",
			Alternative: "Review the package name and its dependencies before installing.",
			Priority:    50,
			Enabled:     true,
			Source:      "seed",
		},
		{
			// ip is the Linux IP networking tool. While read-only invocations (ip route,
			// ip neigh) are common, ip can also add/delete routes and addresses. Because
			// the first positional argument (e.g., "route") does not distinguish show from
			// add/del, we escalate all ip operations rather than allow potentially
			// destructive network changes.
			ID:       "seed-escalate-ip-networking",
			Name:     "Escalate ip networking commands",
			ToolName: "Bash",
			Criteria: &CommandCriteria{
				Programs: []string{"ip"},
			},
			Decision:    Escalate,
			RiskLevel:   RiskMedium,
			Reason:      "ip can modify network routing, addresses, and interfaces; review before proceeding.",
			Alternative: "Use 'ip route show' or 'ip addr show' to confirm current state first.",
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
