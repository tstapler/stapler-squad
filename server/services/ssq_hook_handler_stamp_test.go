package services

import (
	"bytes"
	"context"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
)

// runHookHandler runs scripts/ssq-hook-handler <hookType> with input on stdin and
// a fake ssq-notify that records its argv (one line per invocation).
func runHookHandler(t *testing.T, hookType, input string) []string {
	t.Helper()
	return runHookHandlerEnv(t, hookType, input)
}

// runHookHandlerEnv is runHookHandler with extra KEY=VALUE env entries, which
// override the defaults (last wins).
func runHookHandlerEnv(t *testing.T, hookType, input string, extraEnv ...string) []string {
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

	cmd := safeexec.CommandContext(context.Background(), "bash", handler, hookType)
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(os.Environ(), "CS_NOTIFY="+fake, "CS_SESSION_ID=hook-test-session", "CS_HOOKS_DISABLED=false", "TMUX=")
	cmd.Env = append(cmd.Env, extraEnv...)
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

// hookCallToRequest turns one recorded ssq-notify argv line into the type,
// priority and metadata ssq-notify would send (names per T-SN-01's table).
func hookCallToRequest(t *testing.T, call string) (sessionv1.NotificationType, sessionv1.NotificationPriority, map[string]string) {
	t.Helper()
	types := map[string]sessionv1.NotificationType{
		"error":       sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR,
		"task_failed": sessionv1.NotificationType_NOTIFICATION_TYPE_FAILURE,
	}
	prios := map[string]sessionv1.NotificationPriority{
		"high":   sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH,
		"medium": sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM,
	}
	md := map[string]string{events.MetadataKeySSQNotifySchema: events.SSQNotifySchemaVersion}
	var typ sessionv1.NotificationType
	var prio sessionv1.NotificationPriority
	toks := strings.Fields(call)
	for i := 0; i+1 < len(toks); i++ {
		switch toks[i] {
		case "--type":
			typ = types[toks[i+1]]
		case "-p":
			prio = prios[toks[i+1]]
		case "-d":
			if k, v, ok := strings.Cut(toks[i+1], "="); ok {
				md[k] = v
			}
		}
	}
	if typ == 0 || prio == 0 {
		t.Fatalf("unmapped type/priority in %q", call)
	}
	return typ, prio, md
}

// T-SN-03: end to end, a hidden session's tool error (stamped routine) yields
// zero deliveries while its main-agent Stop failure yields exactly one, with
// the gate on.
func TestHookEvents_ShouldYieldZeroRowsForHiddenPostToolErrorAndOneFailureForMainStop_WhenGateOn(t *testing.T) {
	f := newGatedNotifFixture(t, true)

	postTool := runHookHandler(t, "post-tool", `{"tool_name":"Bash","tool_output":{"error":"boom"}}`)
	require.Len(t, postTool, 1)
	typ, prio, md := hookCallToRequest(t, postTool[0])
	f.send(t, "review-h", typ, prio, md)
	assert.Nil(t, f.next(), "hidden post-tool error must yield zero rows")

	stop := runHookHandler(t, "stop", `{"stop_reason":"error: x"}`)
	require.Len(t, stop, 1)
	typ, prio, md = hookCallToRequest(t, stop[0])
	f.send(t, "review-h", typ, prio, md)
	got := f.next()
	require.NotNil(t, got, "hidden main Stop failure must be delivered")
	assert.Equal(t, "uuid-hidden", got.SessionID)
	assert.Nil(t, f.next(), "exactly one failure row")
}

// T-SN-08 (OQ-2 characterization): a Claude "Notification" hook fired inside a
// stapler-squad tmux pane keeps source_app=tmux and is still delivered for a
// VISIBLE session with the gate on; the idle/info notification is not a
// hidden-session concern (the session has hidden=0).
func TestSendNotification_ShouldDeliverClaudeNotificationViaTmux_WhenSessionVisibleAndGateOn(t *testing.T) {
	calls := runHookHandlerEnv(t, "notification", `{"message":"Claude is waiting for your input","level":"info"}`,
		"TMUX=/tmp/tmux-test/default,1,0", "CS_SESSION_ID=work-v",
		"TERM_PROGRAM=", "VSCODE_PID=", "CURSOR_PID=", "IDEA_INITIAL_DIRECTORY=", "TERMINAL_EMULATOR=")
	require.Len(t, calls, 1)
	require.Contains(t, calls[0], "-d source_app=tmux", "hook must stamp the tmux origin")

	f := newGatedNotifFixture(t, true)
	md := map[string]string{events.MetadataKeySSQNotifySchema: events.SSQNotifySchemaVersion, "source_app": "tmux"}
	f.send(t, "work-v", sessionv1.NotificationType_NOTIFICATION_TYPE_INFO, prioMedium, md)
	got := f.next()
	require.NotNil(t, got, "visible session's tmux notification must be delivered with the gate on")
	assert.Equal(t, "uuid-visible", got.SessionID)
	assert.Equal(t, "tmux", got.NotificationMetadata["source_app"])
}
