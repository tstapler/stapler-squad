package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/internal/claudehooks"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/session"
	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	subcommand := os.Args[1]
	switch subcommand {
	case "check":
		handleCheck()
	case "serve":
		handleServe()
	case "proxy":
		handleProxy()
	case "install":
		handleInstall()
	case "version":
		fmt.Println("ssq-hooks version 0.2.0 (SQLite enabled)")
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: ssq-hooks <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Subcommands:")
	fmt.Fprintln(os.Stderr, "  check   - Classify a single request from JSON on stdin")
	fmt.Fprintln(os.Stderr, "  serve   - Start an HTTP server for remote classification")
	fmt.Fprintln(os.Stderr, "  proxy   - Check permissions before executing a command")
	fmt.Fprintln(os.Stderr, "  install - Install binary and register hooks (targets: claude, gemini, agy, open-code, service)")
	fmt.Fprintln(os.Stderr, "  version - Print version information")
}

func handleCheck() {
	checkCmd := flag.NewFlagSet("check", flag.ExitOnError)
	dbPath := checkCmd.String("db", "", "Path to SQLite database (defaults to workspace-specific database)")
	geminiMode := checkCmd.Bool("gemini", false, "Translate Gemini TOOL_INPUT payload (exit-code output)")
	agyMode := checkCmd.Bool("antigravity", false, "Translate Antigravity TOOL_INPUT payload (hooks.json format)")
	opencodeMode := checkCmd.Bool("opencode", false, "Translate OpenCode tool.execute.before payload (exit-code output)")
	checkCmd.Parse(os.Args[2:]) //nolint:errcheck

	var payload classifier.PermissionRequestPayload
	if *geminiMode || *agyMode {
		payload = parseGeminiPayload()
		// Gemini/agy payload typically lacks cwd; fall back to process working directory.
		if payload.Cwd == "" {
			payload.Cwd, _ = os.Getwd()
		}
	} else if *opencodeMode {
		payload = parseOpenCodePayload()
		if payload.Cwd == "" {
			payload.Cwd, _ = os.Getwd()
		}
	} else {
		if err := json.NewDecoder(os.Stdin).Decode(&payload); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
			os.Exit(1)
		}
		// AskUserQuestion is not a permission gate — Claude is asking the user a question.
		// Return no output (empty stdout) so the hook defers to Claude Code's native terminal dialog.
		// This mirrors the writeDeferDecision path in the HTTP approval handler.
		if strings.EqualFold(payload.ToolName, "AskUserQuestion") {
			os.Exit(0)
		}
	}

	storagePath := *dbPath
	if storagePath == "" {
		storagePath = getDBPathForCwd(payload.Cwd)
	}

	storage := loadStorage(storagePath)
	defer storage.Close()

	c := loadClassifier(storage)
	ctx := c.BuildContext(payload.Cwd)
	result := c.Classify(payload, ctx)

	// Record analytics
	recordResult(storage, payload, result, 0)

	if *geminiMode {
		writeGeminiHookDecision(result)
	} else if *agyMode {
		writeAntigravityHookDecision(result)
	} else if *opencodeMode {
		writeOpenCodeHookDecision(result)
	} else {
		writeHookDecision(result)
	}
}

// hookOutput is the Claude Code PreToolUse hook response format.
type hookOutput struct {
	HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
}

type hookSpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
}

// writeHookDecision writes the Claude Code PreToolUse hook JSON for allow/deny decisions.
// For Escalate, it writes nothing — Claude Code then shows its own permission prompt.
func writeHookDecision(result classifier.ClassificationResult) {
	switch result.Decision {
	case classifier.AutoAllow:
		reason := result.Reason
		if result.RuleName != "" {
			reason = result.RuleName + ": " + reason
		}
		json.NewEncoder(os.Stdout).Encode(hookOutput{
			HookSpecificOutput: hookSpecificOutput{
				HookEventName:            "PreToolUse",
				PermissionDecision:       "allow",
				PermissionDecisionReason: reason,
			},
		})
	case classifier.AutoDeny:
		reason := result.Reason
		if result.Alternative != "" {
			reason += " " + result.Alternative
		}
		json.NewEncoder(os.Stdout).Encode(hookOutput{
			HookSpecificOutput: hookSpecificOutput{
				HookEventName:            "PreToolUse",
				PermissionDecision:       "deny",
				PermissionDecisionReason: reason,
			},
		})
	default:
		// Escalate: write nothing; Claude Code shows its own permission prompt.
	}
}

// writeGeminiHookDecision communicates the classification result to Gemini/agy via exit codes.
//
// Gemini/agy BeforeTool hook contract (confirmed from install-gemini-hook.sh + architecture.md):
//
//	exit 0  — allow the tool call (also used for Escalate: agy shows its own permission dialog)
//	exit 1  — deny the tool call; agy blocks execution
//
// Denial reason is written to stderr (not stdout) to avoid being misinterpreted as
// a blocking output signal if Gemini ever inspects stdout.
func writeGeminiHookDecision(result classifier.ClassificationResult) {
	switch result.Decision {
	case classifier.AutoDeny:
		reason := result.Reason
		if result.Alternative != "" {
			reason += " " + result.Alternative
		}
		ruleInfo := ""
		if result.RuleID != "" {
			ruleInfo = fmt.Sprintf(" [rule: %s]", result.RuleID)
		}
		fmt.Fprintf(os.Stderr, "SSQ-Hooks: blocked%s — %s\n", ruleInfo, reason)
		os.Exit(1)
	default:
		// AutoAllow or Escalate: exit 0.
		// Escalate: empty stdout/stderr — agy shows its own permission dialog.
	}
}

// writeAntigravityHookDecision communicates the classification result to Antigravity CLI via stdout JSON.
func writeAntigravityHookDecision(result classifier.ClassificationResult) {
	type agyOutput struct {
		Decision   string `json:"decision"`
		DenyReason string `json:"deny_reason,omitempty"`
		AllowTool  *bool  `json:"allow_tool,omitempty"`
	}
	var output agyOutput
	switch result.Decision {
	case classifier.AutoAllow:
		output.Decision = "allow"
		t := true
		output.AllowTool = &t
	case classifier.AutoDeny:
		output.Decision = "deny"
		f := false
		output.AllowTool = &f
		reason := result.Reason
		if result.Alternative != "" {
			reason += " " + result.Alternative
		}
		output.DenyReason = reason
	default:
		output.Decision = "ask"
	}
	json.NewEncoder(os.Stdout).Encode(output)
	// Antigravity hook scripts must always exit with 0 (even on deny).
	os.Exit(0)
}

// writeOpenCodeHookDecision communicates the classification result to the generated OpenCode
// plugin via exit code + stderr reason, mirroring writeGeminiHookDecision's contract.
//
// OpenCode plugin contract (@opencode-ai/plugin's Hooks.tool.execute.before, confirmed live —
// see project_plans/opencode-native-hooks/live-verification-notes.md): the only channel a
// plugin has to block a tool call is throwing inside the handler; there is no structured
// decision field to write, unlike Claude/Antigravity's stdout-JSON adapters.
//
//	exit 0  — allow the tool call (the plugin does not throw)
//	exit 1  — deny the tool call; the plugin throws with the stderr text as the error message
//
// Escalate maps to deny (fail-closed), not allow: tool.execute.before has no "ask the user"
// fallback the way Claude Code, Gemini/agy, and Antigravity's own hooks do, and treating the
// classifier's "no rule matched" catch-all as auto-allow would silently weaken the policy for
// the plurality of commands that don't match an explicit rule. See
// docs/adr/ADR-027-opencode-escalate-fail-closed.md for the full decision record.
func writeOpenCodeHookDecision(result classifier.ClassificationResult) {
	switch result.Decision {
	case classifier.AutoDeny:
		reason := result.Reason
		if result.Alternative != "" {
			reason += " " + result.Alternative
		}
		ruleInfo := ""
		if result.RuleID != "" {
			ruleInfo = fmt.Sprintf(" [rule: %s]", result.RuleID)
		}
		fmt.Fprintf(os.Stderr, "SSQ-Hooks: blocked%s — %s\n", ruleInfo, reason)
		os.Exit(1)
	case classifier.Escalate:
		fmt.Fprintln(os.Stderr, "SSQ-Hooks: requires manual review (no rule matched); OpenCode's "+
			"tool.execute.before hook has no ask/dialog fallback, so this is blocked rather than "+
			"silently allowed — approve manually via the review queue or add a classifier rule")
		os.Exit(1)
	default:
		// AutoAllow: exit 0, no output — the plugin does not throw.
	}
}

// openCodePayload is the JSON shape the generated OpenCode plugin (see
// openCodePluginContent) pipes to `ssq-hooks check --opencode` on stdin, assembled from
// tool.execute.before's (input, output) pair plus the plugin's init-time PluginInput.directory.
type openCodePayload struct {
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
	Cwd       string                 `json:"cwd"`
	SessionID string                 `json:"session_id"`
}

// parseOpenCodePayload reads the OpenCode plugin's JSON payload from stdin and translates it
// to a PermissionRequestPayload. Falls back gracefully to ToolName: "Unknown" on any parse
// error or missing tool name — results in Escalate (blocked, per ADR-027), not a crash or a
// silent allow, same defensive shape as parseGeminiPayload.
func parseOpenCodePayload() classifier.PermissionRequestPayload {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SSQ-Hooks: error reading stdin: %v\n", err)
		return classifier.PermissionRequestPayload{ToolName: "Unknown"}
	}
	if os.Getenv("STAPLER_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "SSQ-Hooks [debug] raw OpenCode payload: %s\n", string(raw))
	}
	var p openCodePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		fmt.Fprintf(os.Stderr, "SSQ-Hooks: failed to parse OpenCode payload: %v\n", err)
		return classifier.PermissionRequestPayload{ToolName: "Unknown"}
	}
	if p.ToolName == "" {
		return classifier.PermissionRequestPayload{ToolName: "Unknown"}
	}
	normalizeOpenCodeToolInput(p.ToolInput)
	return classifier.PermissionRequestPayload{
		SessionID: p.SessionID,
		Cwd:       p.Cwd,
		ToolName:  p.ToolName,
		ToolInput: p.ToolInput,
	}
}

// normalizeOpenCodeToolInput adds a "file_path" key (Claude's convention, which
// classifier.Classify's FilePattern rules match against — see pkg/classifier/classifier.go's
// `payload.ToolInput["file_path"]` lookup) mirroring OpenCode's own "filePath" key, when the
// latter is present and the former isn't. Confirmed live: OpenCode's write/edit tool args use
// camelCase "filePath" (e.g. `{"content":"...","filePath":"/tmp/x/.env"}`), not "file_path" —
// without this, .env/.git write-protection rules silently never matched a single OpenCode file
// write, because the FilePattern lookup found an empty string every time. No other translation
// exists for this gap: PermissionRequestPayload's ToolInput is passed through as an opaque
// map[string]interface{}, so a mismatched key name fails silently (empty-string match), not
// with an error — the mismatch was only found by an end-to-end live test hitting a real
// AutoDeny rule, not by unit tests against synthetic payloads.
func normalizeOpenCodeToolInput(toolInput map[string]interface{}) {
	if toolInput == nil {
		return
	}
	if _, hasSnake := toolInput["file_path"]; hasSnake {
		return
	}
	if camel, ok := toolInput["filePath"]; ok {
		toolInput["file_path"] = camel
	}
}

// parseGeminiPayload reads the Gemini/agy $TOOL_INPUT JSON from stdin
// and translates it to a PermissionRequestPayload.
//
// Supported schemas:
//
//	Variant A (Gemini CLI open-source): {"name": "run_shell_command", "args": {"command": "..."}}
//	Variant B (Claude-compatible):      {"tool_name": "...", "tool_input": {...}}
//	Variant C (Antigravity toolCall):   {"toolCall": {"name": "...", "args": {...}}, "workspacePaths": [...], "cwd": "..."}
//
// Variant C's cwd resolves through up to four layers, in priority order:
//  1. top-level "cwd"
//  2. args.Cwd (shell-command tools only, pulled up into the top-level cwd during name normalization)
//  3. workspacePaths[0]
//  4. os.Getwd(), applied by the caller (handleCheck) — not by this function, which can
//     return Cwd == "" if the first three layers are all absent.
//
// Variant C's shape was captured from commit b12652f78 but has never been confirmed
// against a live Antigravity payload — Story 4.1.2 in
// project_plans/agy-support/implementation/plan.md was never completed with a real
// captured fixture.
//
// Falls back gracefully to PermissionRequestPayload{ToolName: "Unknown"} on any
// parse error or unrecognized schema — results in Escalate (not crash, not false-allow).
func parseGeminiPayload() classifier.PermissionRequestPayload {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SSQ-Hooks: error reading stdin: %v\n", err)
		return classifier.PermissionRequestPayload{ToolName: "Unknown"}
	}
	// Debug: dump raw payload when STAPLER_DEBUG=1 (P-1: field capture on first real run)
	if os.Getenv("STAPLER_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "SSQ-Hooks [debug] raw $TOOL_INPUT: %s\n", string(raw))
	}
	type ToolCall struct {
		Name string                 `json:"name"`
		Args map[string]interface{} `json:"args"`
	}
	// GeminiToolPayload covers both known schema variants and Antigravity toolCall.
	// Zero values for absent fields allow detecting which variant is present.
	type GeminiToolPayload struct {
		// Variant A: {"name": "run_shell_command", "args": {"command": "..."}}
		Name string                 `json:"name"`
		Args map[string]interface{} `json:"args"`
		// Variant B: {"tool_name": "...", "tool_input": {...}}
		ToolName  string                 `json:"tool_name"`
		ToolInput map[string]interface{} `json:"tool_input"`
		// Variant C (Antigravity toolCall) — see the parseGeminiPayload doc comment
		// above for the full 4-layer cwd-resolution order.
		ToolCall       *ToolCall `json:"toolCall,omitempty"`
		WorkspacePaths []string  `json:"workspacePaths,omitempty"`
		// Context fields
		Cwd string `json:"cwd"`
	}
	var p GeminiToolPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		fmt.Fprintf(os.Stderr, "SSQ-Hooks: failed to parse Gemini/Antigravity payload: %v\n", err)
		return classifier.PermissionRequestPayload{ToolName: "Unknown"}
	}

	var toolName string
	var toolInput map[string]interface{}

	if p.ToolCall != nil {
		toolName = p.ToolCall.Name
		toolInput = p.ToolCall.Args
	} else if p.ToolName != "" {
		toolName = p.ToolName
		toolInput = p.ToolInput
	} else if p.Name != "" {
		toolName = p.Name
		toolInput = p.Args
	}

	if toolName == "" {
		fmt.Fprintf(os.Stderr, "SSQ-Hooks: unrecognized Gemini/Antigravity payload schema (no tool name field)\n")
		return classifier.PermissionRequestPayload{ToolName: "Unknown"}
	}

	// P-7: pass-through user-input tool (equivalent of AskUserQuestion guard)
	if strings.EqualFold(toolName, "ask_for_user_input") || strings.EqualFold(toolName, "ask_user_question") {
		os.Exit(0)
	}

	// Normalize tool names to classifier-expected names.
	switch strings.ToLower(toolName) {
	case "run_shell_command", "execute_bash", "run_bash_command", "run_command", "bash":
		toolName = "Bash"
		// Antigravity sends args with "CommandLine" (capital C) instead of "command" — normalize.
		if toolInput != nil {
			if v, ok := toolInput["CommandLine"]; ok {
				if _, hasCmd := toolInput["command"]; !hasCmd {
					toolInput["command"] = v
				}
			}
			// Pull Cwd from args if not already set at the top level.
			if p.Cwd == "" {
				if v, ok := toolInput["Cwd"]; ok {
					if s, ok := v.(string); ok {
						p.Cwd = s
					}
				}
			}
		}
	case "read_file", "read_many_files", "read":
		toolName = "Read"
	case "write_file", "write":
		toolName = "Write"
	}

	// Prefer explicit cwd; fall back to first workspacePath.
	cwd := p.Cwd
	if cwd == "" && len(p.WorkspacePaths) > 0 {
		cwd = p.WorkspacePaths[0]
	}

	return classifier.PermissionRequestPayload{
		ToolName:  toolName,
		ToolInput: toolInput,
		Cwd:       cwd,
	}
}

func handleServe() {
	serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
	port := serveCmd.Int("port", 8544, "Port to listen on")
	dbPath := serveCmd.String("db", getDefaultDBPath(), "Path to SQLite database")
	serveCmd.Parse(os.Args[2:])

	storage := loadStorage(*dbPath)
	defer storage.Close()

	c := loadClassifier(storage)

	http.HandleFunc("/classify", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var payload classifier.PermissionRequestPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		start := time.Now()
		ctx := c.BuildContext(payload.Cwd)
		result := c.Classify(payload, ctx)
		durationMs := time.Since(start).Milliseconds()

		// Record analytics
		recordResult(storage, payload, result, durationMs)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	fmt.Fprintf(os.Stderr, "SSQ-Hooks server starting on port %d (DB: %s)...\n", *port, *dbPath)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func handleProxy() {
	// Usage: ssq-hooks proxy -- <command> <args...>
	var cmdArgs []string
	for i, arg := range os.Args {
		if arg == "--" {
			cmdArgs = os.Args[i+1:]
			break
		}
	}

	if len(cmdArgs) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: ssq-hooks proxy -- <command> [args...]")
		os.Exit(1)
	}

	escapedArgs := make([]string, 0, len(cmdArgs))
	for _, arg := range cmdArgs {
		escapedArgs = append(escapedArgs, shellEscape(arg))
	}
	escapedCmd := strings.Join(escapedArgs, " ")

	payload := classifier.PermissionRequestPayload{
		ToolName: "Bash",
		ToolInput: map[string]interface{}{
			"command": escapedCmd,
		},
	}

	cwd, _ := os.Getwd()
	payload.Cwd = cwd

	storage := loadStorage(getDefaultDBPath())
	defer storage.Close()

	c := loadClassifier(storage)
	start := time.Now()
	ctx := c.BuildContext(cwd)
	result := c.Classify(payload, ctx)
	durationMs := time.Since(start).Milliseconds()

	// Record analytics
	recordResult(storage, payload, result, durationMs)

	if result.Decision == classifier.AutoDeny {
		fmt.Fprintf(os.Stderr, "SSQ-Hooks: Command blocked by rule %s (%s)\n", result.RuleID, result.Reason)
		if result.Alternative != "" {
			fmt.Fprintf(os.Stderr, "Alternative: %s\n", result.Alternative)
		}
		os.Exit(1)
	}

	if result.Decision == classifier.AutoAllow {
		fmt.Print(escapedCmd)
		return
	}

	fmt.Fprintf(os.Stderr, "SSQ-Hooks: Command requires manual review (escalated). Currently unsupported in standalone proxy mode.\n")
	os.Exit(1)
}

func loadStorage(path string) *session.Storage {
	repo, err := session.NewEntRepository(session.WithDatabasePath(path))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database %s: %v\n", path, err)
		os.Exit(1)
	}
	storage, err := session.NewStorageWithRepository(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing storage: %v\n", err)
		os.Exit(1)
	}
	return storage
}

func loadClassifier(storage *session.Storage) *classifier.RuleBasedClassifier {
	c := classifier.NewRuleBasedClassifier()
	rules, err := storage.AllRules(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to load rules from DB: %v\n", err)
		return c
	}

	var classifierRules []classifier.Rule
	for _, r := range rules {
		// Convert domain model to classifier rule
		cr := classifier.Rule{
			ID:          r.ID,
			Name:        r.Name,
			ToolName:    r.ToolName,
			Decision:    classifier.ClassificationDecision(r.Decision),
			RiskLevel:   classifier.RiskLevel(r.RiskLevel),
			Reason:      r.Reason,
			Alternative: r.Alternative,
			Priority:    r.Priority,
			Enabled:     r.Enabled,
			Source:      r.Source,
		}
		// Pattern compilation happens in AddRules if we use strings,
		// but here we might need to compile them if we use the Rule struct directly.
		// For now, let's assume we need to compile them.
		if r.ToolPattern != "" {
			compiled, err := regexp.Compile(r.ToolPattern)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: invalid tool pattern %q in rule %s: %v\n", r.ToolPattern, r.ID, err)
				continue
			}
			cr.ToolPattern = compiled
		}
		if r.CommandPattern != "" {
			compiled, err := regexp.Compile(r.CommandPattern)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: invalid command pattern %q in rule %s: %v\n", r.CommandPattern, r.ID, err)
				continue
			}
			cr.CommandPattern = compiled
		}
		if r.FilePattern != "" {
			compiled, err := regexp.Compile(r.FilePattern)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: invalid file pattern %q in rule %s: %v\n", r.FilePattern, r.ID, err)
				continue
			}
			cr.FilePattern = compiled
		}
		// Populate Criteria from structured fields (mirrors specsToRules in rules_store.go).
		if len(r.Programs) > 0 || len(r.Subcommands) > 0 || len(r.BlockedSubcommands) > 0 ||
			len(r.RequiredFlags) > 0 || len(r.ForbiddenFlags) > 0 || len(r.RequiredFlagPrefixes) > 0 ||
			len(r.PythonModes) > 0 || r.SafePythonImportsOnly {
			cr.Criteria = &classifier.CommandCriteria{
				Programs:              r.Programs,
				Subcommands:           r.Subcommands,
				BlockedSubcommands:    r.BlockedSubcommands,
				RequiredFlags:         r.RequiredFlags,
				ForbiddenFlags:        r.ForbiddenFlags,
				RequiredFlagPrefixes:  r.RequiredFlagPrefixes,
				PythonModes:           r.PythonModes,
				SafePythonImportsOnly: r.SafePythonImportsOnly,
			}
		}
		classifierRules = append(classifierRules, cr)
	}
	c.AddRules(classifierRules)

	// Also load config file rules from ~/.config/stapler-squad/shared_rules.yaml.
	configPath := filepath.Join(os.Getenv("HOME"), ".config", "stapler-squad", "shared_rules.yaml")
	if data, err := os.ReadFile(configPath); err == nil {
		var configFile struct {
			Rules []struct {
				Name           string   `yaml:"name"`
				Tool           string   `yaml:"tool"`
				ToolPattern    string   `yaml:"tool_pattern"`
				Programs       []string `yaml:"programs"`
				Subcommands    []string `yaml:"subcommands"`
				BlockedSubs    []string `yaml:"blocked_subcommands"`
				CommandPattern string   `yaml:"command_pattern"`
				FilePattern    string   `yaml:"file_pattern"`
				Decision       string   `yaml:"decision"`
				Priority       int      `yaml:"priority"`
				Enabled        *bool    `yaml:"enabled"`
			} `yaml:"rules"`
		}
		if yamlErr := yaml.Unmarshal(data, &configFile); yamlErr == nil {
			var configRules []classifier.Rule
			for _, r := range configFile.Rules {
				if r.Name == "" {
					continue
				}
				enabled := true
				if r.Enabled != nil {
					enabled = *r.Enabled
				}
				priority := r.Priority
				if priority == 0 {
					priority = 10
				}
				decision := classifier.Escalate
				switch r.Decision {
				case "allow":
					decision = classifier.AutoAllow
				case "deny":
					decision = classifier.AutoDeny
				}
				cr := classifier.Rule{
					ID:       "config-" + strings.ReplaceAll(r.Name, " ", "-"),
					Name:     r.Name,
					ToolName: r.Tool,
					Decision: decision,
					Priority: priority,
					Enabled:  enabled,
					Source:   "config",
				}
				if r.ToolPattern != "" {
					if compiled, err := regexp.Compile(r.ToolPattern); err == nil {
						cr.ToolPattern = compiled
					}
				}
				if r.CommandPattern != "" {
					if compiled, err := regexp.Compile(r.CommandPattern); err == nil {
						cr.CommandPattern = compiled
					}
				}
				if r.FilePattern != "" {
					if compiled, err := regexp.Compile(r.FilePattern); err == nil {
						cr.FilePattern = compiled
					}
				}
				if len(r.Programs) > 0 || len(r.Subcommands) > 0 || len(r.BlockedSubs) > 0 {
					cr.Criteria = &classifier.CommandCriteria{
						Programs:           r.Programs,
						Subcommands:        r.Subcommands,
						BlockedSubcommands: r.BlockedSubs,
					}
				}
				configRules = append(configRules, cr)
			}
			if len(configRules) > 0 {
				c.AddRules(configRules)
			}
		}
	}

	return c
}

func recordResult(storage *session.Storage, payload classifier.PermissionRequestPayload, result classifier.ClassificationResult, durationMs int64) {
	cmd, _ := payload.ToolInput["command"].(string)

	entry := session.AnalyticsData{
		ID:             uuid.New().String(),
		ToolName:       payload.ToolName,
		CommandPreview: cmd,
		Cwd:            payload.Cwd,
		Decision:       decisionString(result.Decision),
		RiskLevel:      riskLevelString(result.RiskLevel),
		RuleID:         result.RuleID,
		RuleName:       result.RuleName,
		Reason:         result.Reason,
		Alternative:    result.Alternative,
		DurationMs:     durationMs,
		CreatedAt:      time.Now(),
	}

	if len(entry.CommandPreview) > 200 {
		entry.CommandPreview = entry.CommandPreview[:200]
	}

	// Extract program info
	if payload.ToolName == "Bash" && cmd != "" {
		info := classifier.ParseBashCommand(cmd)
		entry.CommandProgram = info.Program
		entry.CommandCategory = info.Category
		entry.CommandSubcategory = info.Subcommand
		if classifier.PythonPrograms[info.Program] {
			pyInfo := classifier.ParsePythonCommand(cmd)
			entry.PythonImports = pyInfo.Imports
		}
	}

	_ = storage.RecordAnalytics(context.Background(), entry)
}

func decisionString(d classifier.ClassificationDecision) string {
	switch d {
	case classifier.AutoAllow:
		return "auto_allow"
	case classifier.AutoDeny:
		return "auto_deny"
	default:
		return "escalate"
	}
}

func riskLevelString(r classifier.RiskLevel) string {
	switch r {
	case classifier.RiskLow:
		return "low"
	case classifier.RiskMedium:
		return "medium"
	case classifier.RiskHigh:
		return "high"
	case classifier.RiskCritical:
		return "critical"
	default:
		return "medium"
	}
}

func shellEscape(arg string) string {
	if len(arg) == 0 {
		return "''"
	}
	safe := true
	for _, c := range arg {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '_' && c != '/' && c != '.' && c != '+' && c != '=' && c != ':' && c != '@' {
			safe = false
			break
		}
	}
	if safe {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
}

func handleInstall() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: ssq-hooks install <target>")
		fmt.Fprintln(os.Stderr, "Targets: claude, gemini, agy, open-code, service")
		os.Exit(1)
	}

	target := os.Args[2]
	switch target {
	case "claude":
		installClaude()
	case "gemini":
		installGemini()
	case "agy":
		installAgy()
	case "antigravity": // hidden alias for agy
		installAgy()
	case "open-code":
		installOpenCode()
	case "service":
		installService()
	default:
		fmt.Fprintf(os.Stderr, "Unknown install target: %s\n", target)
		os.Exit(1)
	}
}

// installClaude copies the ssq-hooks binary to ~/.local/bin and registers it as
// a PreToolUse hook in ~/.claude/settings.json. Safe to run multiple times.
func installClaude() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving home directory: %v\n", err)
		os.Exit(1)
	}

	// 1. Copy binary to ~/.local/bin/ssq-hooks.
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", binDir, err)
		os.Exit(1)
	}
	destBin := filepath.Join(binDir, "ssq-hooks")
	srcBin, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving current binary: %v\n", err)
		os.Exit(1)
	}
	// Resolve symlinks so we copy the real binary.
	if resolved, err := filepath.EvalSymlinks(srcBin); err == nil {
		srcBin = resolved
	}
	if err := copyBinary(srcBin, destBin); err != nil {
		fmt.Fprintf(os.Stderr, "Error copying binary to %s: %v\n", destBin, err)
		os.Exit(1)
	}
	fmt.Printf("Installed binary: %s\n", destBin)

	// 2. Patch ~/.claude/settings.json.
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := claudehooks.InstallRules(settingsPath, destBin); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating %s: %v\n", settingsPath, err)
		os.Exit(1)
	}
	fmt.Printf("Updated hook:     %s\n", settingsPath)
	fmt.Println("Done. Restart Claude Code for the hook to take effect.")
}

// copyBinary copies src to dst as an executable file, replacing dst if it exists.
func copyBinary(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// Write to a temp file first, then atomically rename to avoid partial writes.
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer func() { os.Remove(tmp) }() //nolint:errcheck

	if _, err := out.ReadFrom(in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// patchBeforeToolHook patches any Gemini-family settings file (flat JSON) to set
// hooks.BeforeTool to hookCmd. It is idempotent: if the exact string is already
// present, it prints a message and returns nil. If BeforeTool is a non-string type
// (e.g. an array), it returns a descriptive error rather than silently overwriting.
// The write is atomic: data is written to settingsPath+".tmp" then renamed.
func patchBeforeToolHook(settingsPath, hookCmd string) error {
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		raw = []byte("{}")
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return fmt.Errorf("parsing %s: %w", settingsPath, err)
	}
	hooks, _ := settings["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = map[string]interface{}{}
		settings["hooks"] = hooks
	}
	// Guard: existing BeforeTool must be a string, not an array/object.
	if existing, ok := hooks["BeforeTool"]; ok {
		if _, ok := existing.(string); !ok {
			return fmt.Errorf("parsing %s: hooks.\"BeforeTool\" is not a string (found %T); cannot patch", settingsPath, existing)
		}
		if existing.(string) == hookCmd {
			fmt.Println("Hook already present, nothing to do.")
			return nil
		}
	}
	hooks["BeforeTool"] = hookCmd
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0700); err != nil {
		return err
	}
	// Atomic write (P-4: avoid partial-read race with running agy/Gemini process).
	tmpPath := settingsPath + ".tmp"
	if err := os.WriteFile(tmpPath, append(out, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, settingsPath)
}

func installGemini() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving home directory: %v\n", err)
		os.Exit(1)
	}
	// 1. Copy binary to ~/.local/bin/ssq-hooks.
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", binDir, err)
		os.Exit(1)
	}
	destBin := filepath.Join(binDir, "ssq-hooks")
	srcBin, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving current binary: %v\n", err)
		os.Exit(1)
	}
	if resolved, err := filepath.EvalSymlinks(srcBin); err == nil {
		srcBin = resolved
	}
	if err := copyBinary(srcBin, destBin); err != nil {
		fmt.Fprintf(os.Stderr, "Error copying binary to %s: %v\n", destBin, err)
		os.Exit(1)
	}
	fmt.Printf("Installed binary: %s\n", destBin)
	// 2. Discover Gemini settings file (P-5: patch only the first found).
	candidates := []string{
		filepath.Join(home, ".gemini", "settings.json"), // authoritative (observed live)
		filepath.Join(home, ".gemini", "config.json"),   // legacy fallback
	}
	settingsPath := ""
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			settingsPath = c
			break
		}
	}
	if settingsPath == "" {
		// Neither found: create ~/.gemini/settings.json.
		settingsPath = candidates[0]
	}
	// 3. Patch the selected file.
	hookCmd := destBin + " check --gemini"
	if err := patchBeforeToolHook(settingsPath, hookCmd); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating %s: %v\n", settingsPath, err)
		os.Exit(1)
	}
	fmt.Printf("Updated hook:     %s\n", settingsPath)
	fmt.Println("Done. Restart Gemini CLI for the hook to take effect.")
}

func installAgy() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving home directory: %v\n", err)
		os.Exit(1)
	}
	// 1. Copy binary to ~/.local/bin/ssq-hooks.
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", binDir, err)
		os.Exit(1)
	}
	destBin := filepath.Join(binDir, "ssq-hooks")
	srcBin, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving current binary: %v\n", err)
		os.Exit(1)
	}
	if resolved, err := filepath.EvalSymlinks(srcBin); err == nil {
		srcBin = resolved
	}
	if err := copyBinary(srcBin, destBin); err != nil {
		fmt.Fprintf(os.Stderr, "Error copying binary to %s: %v\n", destBin, err)
		os.Exit(1)
	}
	fmt.Printf("Installed binary: %s\n", destBin)
	// 2. Discover agy hooks file — patch only the first found (mirrors installGemini).
	// ~/.gemini/antigravity-cli/ is agy's primary runtime state dir.
	// ~/.gemini/config/hooks.json is the fallback global config location.
	candidates := []string{
		filepath.Join(home, ".gemini", "antigravity-cli", "hooks.json"),
		filepath.Join(home, ".gemini", "config", "hooks.json"),
	}
	hooksPath := ""
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			hooksPath = c
			break
		}
	}
	if hooksPath == "" {
		// Neither found: create the primary (antigravity-cli is where agy reads hooks).
		hooksPath = candidates[0]
	}
	// 3. Patch the selected file.
	if err := patchAntigravityHooks(hooksPath, destBin); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating %s: %v\n", hooksPath, err)
		os.Exit(1)
	}
	fmt.Printf("Updated hook:     %s\n", hooksPath)
	// 4. Cleanup: remove any stale ssq-hooks entry from the other candidate.
	// This handles users who ran an old version that patched both paths.
	for _, c := range candidates {
		if c != hooksPath {
			_ = removeAntigravityHookEntry(c)
		}
	}
	fmt.Println("Done. Restart agy for the hook to take effect.")
}

// removeAntigravityHookEntry removes the "stapler-squad" key from hooksPath if it
// contains any ssq-hooks check --antigravity command. No-ops if the file doesn't
// exist, the key is absent, or no matching command is found.
func removeAntigravityHookEntry(hooksPath string) error {
	raw, err := os.ReadFile(hooksPath)
	if err != nil {
		return nil // file absent — nothing to clean up
	}
	var hooksData map[string]interface{}
	if err := json.Unmarshal(raw, &hooksData); err != nil {
		return nil // not valid JSON — leave it alone
	}
	existing, ok := hooksData["stapler-squad"]
	if !ok {
		return nil // no entry — nothing to clean up
	}
	existingMap, ok := existing.(map[string]interface{})
	if !ok {
		return nil
	}
	preToolUse, _ := existingMap["PreToolUse"].([]interface{})
	found := false
	for _, entry := range preToolUse {
		m, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		innerHooks, _ := m["hooks"].([]interface{})
		for _, h := range innerHooks {
			hm, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			if cmd, _ := hm["command"].(string); strings.HasSuffix(cmd, " check --antigravity") {
				found = true
				break
			}
		}
	}
	if !found {
		return nil // our command not present — leave it alone
	}
	delete(hooksData, "stapler-squad")
	out, err := json.MarshalIndent(hooksData, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := hooksPath + ".tmp"
	if err := os.WriteFile(tmpPath, append(out, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, hooksPath)
}

// patchAntigravityHooks patches the agy hooks.json file to register the ssq-hooks check command.
func patchAntigravityHooks(hooksPath, binPath string) error {
	hookCmd := binPath + " check --antigravity"

	// Read existing settings (create minimal file if absent).
	raw, err := os.ReadFile(hooksPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		raw = []byte("{}")
	}

	var hooksData map[string]interface{}
	if err := json.Unmarshal(raw, &hooksData); err != nil {
		return fmt.Errorf("parsing %s: %w", hooksPath, err)
	}

	// Build the new/updated hook structure.
	newHookConfig := map[string]interface{}{
		"enabled": true,
		"PreToolUse": []interface{}{
			map[string]interface{}{
				"matcher": "*",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": hookCmd,
						"timeout": 10,
					},
				},
			},
		},
	}

	// Check if the hook is already present and matches.
	if existing, ok := hooksData["stapler-squad"]; ok {
		if existingMap, ok := existing.(map[string]interface{}); ok {
			if preToolUse, ok := existingMap["PreToolUse"].([]interface{}); ok && len(preToolUse) > 0 {
				if firstEntry, ok := preToolUse[0].(map[string]interface{}); ok {
					if innerHooks, ok := firstEntry["hooks"].([]interface{}); ok && len(innerHooks) > 0 {
						if hookObj, ok := innerHooks[0].(map[string]interface{}); ok {
							if cmd, _ := hookObj["command"].(string); cmd == hookCmd {
								fmt.Println("Antigravity hook already present, nothing to do.")
								return nil
							}
						}
					}
				}
			}
		}
	}

	// Update or insert the hook.
	hooksData["stapler-squad"] = newHookConfig

	out, err := json.MarshalIndent(hooksData, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(hooksPath), 0700); err != nil {
		return err
	}

	// Atomic write
	tmpPath := hooksPath + ".tmp"
	if err := os.WriteFile(tmpPath, append(out, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, hooksPath)
}

// openCodePluginTemplate is the JS plugin installOpenCode() writes to opencode's global plugin
// directory. It subscribes to @opencode-ai/plugin's Hooks.tool.execute.before, which fires
// before a tool call executes; throwing inside the handler blocks the call (confirmed live —
// see project_plans/opencode-native-hooks/live-verification-notes.md). tool.execute.before has
// no structured decision channel, so the plugin shells out to `ssq-hooks check --opencode` and
// interprets its exit code exactly like writeOpenCodeHookDecision's contract (exit 0 = allow,
// non-zero = deny with the reason on stderr, thrown as the JS Error).
//
// Deliberately does not `import` anything from @opencode-ai/plugin: the Hooks/PluginInput
// types are TypeScript-only and stripped at runtime, and importing the package would tie this
// generated file to whichever version happens to be resolved on a given user's machine — this
// dev machine alone has a declared/resolved version mismatch (opencode.json's package.json
// declares 1.4.0, node_modules resolves 1.3.10; see research/stack.md §1/§4 and
// live-verification-notes.md). Treating the (input, output) shape as a stable plain-JS runtime
// contract, verified live rather than assumed from either version's types, sidesteps that drift
// entirely instead of trying to detect and branch on it.
const openCodePluginTemplate = `// Generated by ssq-hooks install open-code. Do not hand-edit — re-run the installer instead.
import { execFileSync } from "node:child_process";

export const StaplerSquad = async (ctx) => {
  return {
    "tool.execute.before": async (input, output) => {
      const payload = JSON.stringify({
        tool_name: input.tool,
        tool_input: output.args,
        session_id: input.sessionID,
        cwd: ctx.directory,
      });
      try {
        execFileSync(%q, ["check", "--opencode"], { input: payload, encoding: "utf8" });
      } catch (err) {
        throw new Error((err.stderr || "").trim() || "blocked by stapler-squad policy");
      }
    },
  };
};
`

func openCodePluginContent(ssqHooksPath string) string {
	return fmt.Sprintf(openCodePluginTemplate, ssqHooksPath)
}

// patchOpenCodeHooks writes the ssq-hooks OpenCode plugin to pluginPath. Safe to run multiple
// times: the generated content is a pure function of ssqHooksPath, so re-running with the same
// binary path produces byte-identical output (no explicit "already present" check needed, unlike
// the JSON-config installers, since there's no third-party config structure to merge into).
func patchOpenCodeHooks(pluginPath, ssqHooksPath string) error {
	if err := os.MkdirAll(filepath.Dir(pluginPath), 0755); err != nil {
		return err
	}
	content := openCodePluginContent(ssqHooksPath)
	tmpPath := pluginPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, pluginPath)
}

// removeStaleOpenCodeWrapper deletes the old ~/.local/bin/open-code bash-wrapper proxy left
// behind by installOpenCode()'s pre-plugin implementation, if present and if its content still
// matches the old wrapper. No-ops if the file is absent or doesn't match (e.g. a user's own
// unrelated open-code script) — mirrors removeAntigravityHookEntry's caution about not touching
// content ssq-hooks didn't write.
func removeStaleOpenCodeWrapper(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil // absent — nothing to clean up
	}
	if !strings.Contains(string(raw), "proxy -- open-code") {
		return nil // not our old wrapper — leave it alone
	}
	return os.Remove(path)
}

// installOpenCode copies the ssq-hooks binary to ~/.local/bin, writes the OpenCode plugin to
// opencode's global plugin directory (~/.config/opencode/plugins/ — confirmed auto-loaded with
// no opencode.json registration needed, see live-verification-notes.md), and removes any stale
// ~/.local/bin/open-code wrapper from a prior install. Safe to run multiple times.
func installOpenCode() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving home directory: %v\n", err)
		os.Exit(1)
	}

	// 1. Copy binary to ~/.local/bin/ssq-hooks.
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", binDir, err)
		os.Exit(1)
	}
	destBin := filepath.Join(binDir, "ssq-hooks")
	srcBin, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving current binary: %v\n", err)
		os.Exit(1)
	}
	if resolved, err := filepath.EvalSymlinks(srcBin); err == nil {
		srcBin = resolved
	}
	if err := copyBinary(srcBin, destBin); err != nil {
		fmt.Fprintf(os.Stderr, "Error copying binary to %s: %v\n", destBin, err)
		os.Exit(1)
	}
	fmt.Printf("Installed binary: %s\n", destBin)

	// 2. Write the plugin to opencode's global plugin directory.
	pluginPath := filepath.Join(home, ".config", "opencode", "plugins", "ssq-hooks.js")
	if err := patchOpenCodeHooks(pluginPath, destBin); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing plugin to %s: %v\n", pluginPath, err)
		os.Exit(1)
	}
	fmt.Printf("Installed plugin: %s\n", pluginPath)

	// 3. Clean up the old bash-wrapper proxy, if a prior install left one behind.
	staleWrapper := filepath.Join(binDir, "open-code")
	if err := removeStaleOpenCodeWrapper(staleWrapper); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove stale wrapper %s: %v\n", staleWrapper, err)
	}

	// Best-effort, informational only: report the detected opencode CLI version so a user can
	// judge for themselves whether it's far from what this plugin shape was verified against
	// (1.4.0). Deliberately not a hard version gate — the plugin template avoids depending on
	// @opencode-ai/plugin's version at all (see openCodePluginTemplate's doc comment), so a
	// mismatch here is not a reason to fail the install.
	versionCtx, versionCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if out, err := safeexec.CommandContext(versionCtx, "opencode", "--version").Output(); err == nil {
		fmt.Printf("Detected opencode CLI version: %s", out)
	}
	versionCancel()

	fmt.Println("Done. Restart OpenCode for the hook to take effect.")
}

func installService() {
	installCmd := flag.NewFlagSet("service", flag.ExitOnError)
	uninstall := installCmd.Bool("uninstall", false, "Remove the service and disable auto-start")
	installCmd.Parse(os.Args[3:])

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving home directory: %v\n", err)
		os.Exit(1)
	}

	logDir := filepath.Join(home, ".stapler-squad", "logs")
	currentPath := os.Getenv("PATH")

	// Resolve binary path: STAPLER_SQUAD_BIN env > which > os.Executable
	binPath := os.Getenv("STAPLER_SQUAD_BIN")
	if binPath == "" {
		if p, err := exec.LookPath("stapler-squad"); err == nil {
			binPath = p
		}
	}
	if binPath == "" {
		if p, err := os.Executable(); err == nil {
			binPath = p
		} else {
			fmt.Fprintln(os.Stderr, "Cannot find stapler-squad binary. Set STAPLER_SQUAD_BIN or ensure it is in PATH.")
			os.Exit(1)
		}
	}

	switch runtime.GOOS {
	case "linux":
		installServiceLinux(home, binPath, logDir, currentPath, *uninstall)
	case "darwin":
		installServiceMacOS(home, binPath, logDir, currentPath, *uninstall)
	default:
		fmt.Fprintf(os.Stderr, "Unsupported platform: %s\n", runtime.GOOS)
		fmt.Fprintln(os.Stderr, "Supported platforms: Linux (systemd user), macOS (LaunchAgent)")
		os.Exit(1)
	}
}

func installServiceLinux(home, binPath, logDir, envPath string, uninstall bool) {
	serviceDir := filepath.Join(home, ".config", "systemd", "user")
	serviceFile := filepath.Join(serviceDir, "stapler-squad.service")

	if uninstall {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		stopCmd := safeexec.CommandContext(stopCtx, "systemctl", "--user", "stop", "stapler-squad")
		stopCmd.Run() //nolint:errcheck
		disableCtx, disableCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer disableCancel()
		disableCmd := safeexec.CommandContext(disableCtx, "systemctl", "--user", "disable", "stapler-squad")
		disableCmd.Run() //nolint:errcheck
		if _, err := os.Stat(serviceFile); err == nil {
			os.Remove(serviceFile) //nolint:errcheck
			reloadCtx, reloadCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer reloadCancel()
			reloadCmd := safeexec.CommandContext(reloadCtx, "systemctl", "--user", "daemon-reload")
			reloadCmd.Run() //nolint:errcheck
			fmt.Printf("Removed: %s\n", serviceFile)
		} else {
			fmt.Printf("Service file not found (already removed?): %s\n", serviceFile)
		}
		fmt.Println("stapler-squad will no longer start automatically on login.")
		fmt.Println("Your data in ~/.stapler-squad/ has not been touched.")
		return
	}

	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", serviceDir, err)
		os.Exit(1)
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", logDir, err)
		os.Exit(1)
	}

	serviceLog := filepath.Join(logDir, "service.log")
	content := fmt.Sprintf(`[Unit]
Description=Stapler Squad — AI Agent Session Manager
Documentation=https://github.com/tstapler/stapler-squad
After=network.target

[Service]
Type=simple
ExecStart=%s
WorkingDirectory=%s
Restart=on-failure
RestartSec=5s
StandardOutput=append:%s
StandardError=append:%s
Environment=HOME=%s
Environment=PATH=%s

[Install]
WantedBy=default.target
`, binPath, home, serviceLog, serviceLog, home, envPath)

	if err := os.WriteFile(serviceFile, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing service file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Service file written to: %s\n\n", serviceFile)
	fmt.Println("Enable and start now:")
	fmt.Println("    systemctl --user daemon-reload")
	fmt.Println("    systemctl --user enable --now stapler-squad")
	fmt.Println()
	fmt.Println("Check status:")
	fmt.Println("    systemctl --user status stapler-squad")
	fmt.Println()
	fmt.Printf("View logs:\n    tail -f %s\n\n", serviceLog)
	fmt.Println("Optional — keep service running after logout (one-time setup):")
	fmt.Println("    loginctl enable-linger $USER")
	fmt.Println()
	fmt.Println("If you rebuild or move the binary, re-run this command to update the service file.")
}

func installServiceMacOS(home, binPath, logDir, envPath string, uninstall bool) {
	plistDir := filepath.Join(home, "Library", "LaunchAgents")
	plistFile := filepath.Join(plistDir, "com.stapler-squad.plist")

	if uninstall {
		unloadCtx, unloadCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer unloadCancel()
		unloadCmd := safeexec.CommandContext(unloadCtx, "launchctl", "unload", plistFile)
		unloadCmd.Run() //nolint:errcheck
		if _, err := os.Stat(plistFile); err == nil {
			os.Remove(plistFile) //nolint:errcheck
			fmt.Printf("Removed: %s\n", plistFile)
		} else {
			fmt.Printf("Plist not found (already removed?): %s\n", plistFile)
		}
		fmt.Println("stapler-squad will no longer start automatically on login.")
		fmt.Println("Your data in ~/.stapler-squad/ has not been touched.")
		return
	}

	if err := os.MkdirAll(plistDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", plistDir, err)
		os.Exit(1)
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", logDir, err)
		os.Exit(1)
	}

	serviceLog := filepath.Join(logDir, "service.log")
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
    "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.stapler-squad</string>

    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>

    <key>WorkingDirectory</key>
    <string>%s</string>

    <key>EnvironmentVariables</key>
    <dict>
        <key>HOME</key>
        <string>%s</string>
        <key>PATH</key>
        <string>%s</string>
    </dict>

    <key>StandardOutPath</key>
    <string>%s</string>

    <key>StandardErrorPath</key>
    <string>%s</string>

    <key>ThrottleInterval</key>
    <integer>5</integer>
</dict>
</plist>
`, binPath, home, home, envPath, serviceLog, serviceLog)

	if err := os.WriteFile(plistFile, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing plist: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("LaunchAgent plist written to: %s\n\n", plistFile)
	fmt.Println("Load and start now (macOS 12 and earlier):")
	fmt.Printf("    launchctl load -w %s\n\n", plistFile)
	fmt.Println("Load and start now (macOS 13 Ventura and later):")
	fmt.Printf("    launchctl bootstrap gui/$(id -u) %s\n\n", plistFile)
	fmt.Println("Check status:")
	fmt.Println("    launchctl list | grep stapler-squad")
	fmt.Println()
	fmt.Printf("View logs:\n    tail -f %s\n\n", serviceLog)
	fmt.Println("If you rebuild or move the binary, re-run this command to update the plist.")
}

func getDefaultDBPath() string {
	configDir, err := config.GetConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".stapler-squad", "sessions.db")
	}
	return filepath.Join(configDir, "sessions.db")
}

func getDBPathForCwd(cwd string) string {
	configDir, err := config.GetConfigDirForDir(cwd)
	if err != nil {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".stapler-squad", "sessions.db")
	}
	return filepath.Join(configDir, "sessions.db")
}
