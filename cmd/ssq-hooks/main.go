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
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/session"
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
	dbPath := checkCmd.String("db", getDefaultDBPath(), "Path to SQLite database")
	geminiMode := checkCmd.Bool("gemini", false, "Translate Gemini/agy TOOL_INPUT payload (exit-code output)")
	checkCmd.Parse(os.Args[2:]) //nolint:errcheck

	var payload classifier.PermissionRequestPayload
	if *geminiMode {
		payload = parseGeminiPayload()
		// Gemini payload typically lacks cwd; fall back to process working directory.
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

	storage := loadStorage(*dbPath)
	defer storage.Close()

	c := loadClassifier(storage)
	ctx := c.BuildContext(payload.Cwd)
	result := c.Classify(payload, ctx)

	// Record analytics
	recordResult(storage, payload, result, 0)

	if *geminiMode {
		writeGeminiHookDecision(result)
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

// parseGeminiPayload reads the Gemini/agy $TOOL_INPUT JSON from stdin
// and translates it to a PermissionRequestPayload.
//
// Supported schemas:
//
//	Variant A (Gemini CLI open-source): {"name": "run_shell_command", "args": {"command": "..."}}
//	Variant B (Claude-compatible):      {"tool_name": "Bash", "tool_input": {"command": "..."}}
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
	// GeminiToolPayload covers both known schema variants.
	// Zero values for absent fields allow detecting which variant is present.
	type GeminiToolPayload struct {
		// Variant A: {"name": "run_shell_command", "args": {"command": "..."}}
		Name string                 `json:"name"`
		Args map[string]interface{} `json:"args"`
		// Variant B: {"tool_name": "...", "tool_input": {...}}
		ToolName  string                 `json:"tool_name"`
		ToolInput map[string]interface{} `json:"tool_input"`
	}
	var p GeminiToolPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		fmt.Fprintf(os.Stderr, "SSQ-Hooks: failed to parse Gemini payload: %v\n", err)
		return classifier.PermissionRequestPayload{ToolName: "Unknown"}
	}
	// Prefer Variant B (Claude-compatible) if present.
	if p.ToolName != "" {
		payload := classifier.PermissionRequestPayload{
			ToolName:  p.ToolName,
			ToolInput: p.ToolInput,
		}
		// P-7: pass-through user-input tool (equivalent of AskUserQuestion guard)
		if strings.EqualFold(payload.ToolName, "ask_for_user_input") {
			os.Exit(0)
		}
		return payload
	}
	// Fall back to Variant A: name + args.
	if p.Name == "" {
		// Neither variant matched: unknown schema.
		fmt.Fprintf(os.Stderr, "SSQ-Hooks: unrecognized Gemini payload schema (no 'name' or 'tool_name' field)\n")
		return classifier.PermissionRequestPayload{ToolName: "Unknown"}
	}
	toolName := p.Name
	// Normalize Gemini tool names to classifier-expected names.
	switch strings.ToLower(toolName) {
	case "run_shell_command", "execute_bash", "run_bash_command":
		toolName = "Bash"
	case "read_file", "read_many_files":
		toolName = "Read"
	case "write_file":
		toolName = "Write"
	case "ask_for_user_input":
		// P-7: pass-through user-input tool (equivalent of AskUserQuestion guard)
		os.Exit(0)
	}
	return classifier.PermissionRequestPayload{
		ToolName:  toolName,
		ToolInput: p.Args,
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
		classifierRules = append(classifierRules, cr)
	}
	c.AddRules(classifierRules)
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
	if err := patchClaudeSettings(settingsPath, destBin); err != nil {
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

// patchClaudeSettings adds the ssq-hooks PreToolUse entry to settingsPath.
// The hook entry is prepended to the PreToolUse array so it runs before other
// hooks (e.g. rtk-rewrite). Idempotent: no-ops if the entry already exists.
func patchClaudeSettings(settingsPath, binPath string) error {
	hookCmd := binPath + " check"

	// Read existing settings (create minimal file if absent).
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

	// Navigate to hooks.PreToolUse, creating intermediate maps as needed.
	if existing, ok := settings["hooks"]; ok {
		if _, ok := existing.(map[string]interface{}); !ok {
			return fmt.Errorf("parsing %s: \"hooks\" field is not an object", settingsPath)
		}
	}
	hooks, _ := settings["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = map[string]interface{}{}
		settings["hooks"] = hooks
	}
	if existing, ok := hooks["PreToolUse"]; ok {
		if _, ok := existing.([]interface{}); !ok {
			return fmt.Errorf("parsing %s: hooks.\"PreToolUse\" field is not an array", settingsPath)
		}
	}
	preToolUse, _ := hooks["PreToolUse"].([]interface{})

	// Check if the hook is already present (idempotency).
	for _, entry := range preToolUse {
		m, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		hookList, _ := m["hooks"].([]interface{})
		for _, h := range hookList {
			hm, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			if cmd, _ := hm["command"].(string); cmd == hookCmd {
				fmt.Println("Hook already present, nothing to do.")
				return nil
			}
		}
	}

	// Prepend the ssq-hooks entry so it gets first say.
	newEntry := map[string]interface{}{
		"matcher": ".*",
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": hookCmd,
			},
		},
	}
	hooks["PreToolUse"] = append([]interface{}{newEntry}, preToolUse...)

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	// Ensure parent directory exists (e.g. ~/.claude/ may not exist yet).
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0700); err != nil {
		return err
	}
	return os.WriteFile(settingsPath, append(out, '\n'), 0644)
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
	// 2. Patch ~/.gemini/antigravity-cli/settings.json.
	settingsPath := filepath.Join(home, ".gemini", "antigravity-cli", "settings.json")
	hookCmd := destBin + " check --gemini"
	if err := patchBeforeToolHook(settingsPath, hookCmd); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating %s: %v\n", settingsPath, err)
		os.Exit(1)
	}
	fmt.Printf("Updated hook:     %s\n", settingsPath)
	fmt.Println("Done. Restart agy for the hook to take effect.")
}

func installOpenCode() {
	home, _ := os.UserHomeDir()
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", binDir, err)
		os.Exit(1)
	}

	wrapperPath := filepath.Join(binDir, "open-code")
	ssqPath, err := os.Executable()
	if err != nil {
		ssqPath = "ssq-hooks"
	}

	content := fmt.Sprintf(`#!/usr/bin/env bash
# Intercepts calls to open-code and routes them through ssq-hooks proxy
set -euo pipefail
CMD=$(%s proxy -- open-code "$@")
eval "$CMD"
`, ssqPath)

	if err := os.WriteFile(wrapperPath, []byte(content), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing wrapper to %s: %v\n", wrapperPath, err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Successfully installed open-code wrapper to %s\n", wrapperPath)
	fmt.Fprintf(os.Stderr, "Ensure %s is in your PATH.\n", binDir)
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
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".stapler-squad", "sessions.db")
}
