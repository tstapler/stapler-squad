package services

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// runHookHandler runs scripts/ssq-hook-handler <hookType> with input on stdin and
// a fake ssq-notify that records its argv (one line per invocation).
func runHookHandler(t *testing.T, hookType, input string) []string {
	t.Helper()
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not installed")
	}
	_, file, _, _ := runtime.Caller(0)
	handler := filepath.Join(filepath.Dir(file), "..", "..", "scripts", "ssq-hook-handler")

	dir := t.TempDir()
	record := filepath.Join(dir, "calls.txt")
	fake := filepath.Join(dir, "fake-notify")
	script := "#!/usr/bin/env bash\nprintf '%s\\n' \"$*\" >> " + record + "\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", handler, hookType)
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(os.Environ(), "CS_NOTIFY="+fake, "CS_SESSION_ID=hook-test-session", "CS_HOOKS_DISABLED=false", "TMUX=")
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("hook handler: %v\n%s", err, out.String())
	}
	data, err := os.ReadFile(record)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// T-SN-02: routine stamps go on post-tool errors and subagent failures only.
func TestHookHandler_ShouldStampRoutineOnPostToolAndSubagentOnly_WhenCurlPayloadRecorded(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		hook, input string
		wantType    string
		wantRoutine bool
	}{
		{"post-tool error", "post-tool", `{"tool_name":"Bash","tool_output":{"error":"boom"}}`, "--type error", true},
		{"subagent failed", "subagent-stop", `{"stop_reason":"error: x"}`, "--type task_failed", true},
		{"subagent context limit", "subagent-stop", `{"stop_reason":"max_tokens"}`, "--type task_failed", true},
		{"main stop failed", "stop", `{"stop_reason":"error: x"}`, "--type task_failed", false},
		{"main stop context limit", "stop", `{"stop_reason":"max_tokens"}`, "--type task_failed", false},
		{"main stop complete", "stop", `{"stop_reason":"end_turn"}`, "--type task_complete", false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			calls := runHookHandler(t, c.hook, c.input)
			if len(calls) != 1 {
				t.Fatalf("want 1 ssq-notify call, got %d: %v", len(calls), calls)
			}
			if !strings.Contains(calls[0], c.wantType) {
				t.Errorf("call %q lacks %q", calls[0], c.wantType)
			}
			if got := strings.Contains(calls[0], "-d delivery_class=routine"); got != c.wantRoutine {
				t.Errorf("routine stamp = %v, want %v in %q", got, c.wantRoutine, calls[0])
			}
		})
	}
}

// Every --type the hook handler sends is a name the corrected ssq-notify maps
// (Task 2.5c): an unmapped name would silently become INFO.
func TestHookHandler_ShouldOnlyUseMappedTypeNames_WhenScanningScript(t *testing.T) {
	t.Parallel()
	_, file, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "scripts", "ssq-hook-handler"))
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{"info": true, "approval_needed": true, "question": true, "error": true,
		"warning": true, "task_complete": true, "task_failed": true, "progress": true, "custom": true}
	for _, line := range strings.Split(string(data), "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#") {
			continue
		}
		if i := strings.Index(line, "--type "); i >= 0 {
			name := strings.Fields(line[i+len("--type "):])[0]
			if !known[name] {
				t.Errorf("hook handler sends unmapped --type %q: %s", name, trim)
			}
		}
	}
}
