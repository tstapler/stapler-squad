package services

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// ssqNotifyScript returns the repo's scripts/ssq-notify path regardless of cwd.
func ssqNotifyScript(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "scripts", "ssq-notify")
}

// ssqNotifyDryRun runs `ssq-notify --dry-run` and returns the request body it
// would have POSTed. No server is contacted.
func ssqNotifyDryRun(t *testing.T, typeName string, extra ...string) map[string]any {
	t.Helper()
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not installed; ssq-notify requires it")
	}
	args := []string{ssqNotifyScript(t), "--dry-run", "-s", "sess", "-t", "title", "--type", typeName}
	args = append(args, extra...)
	cmd := exec.Command("bash", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("ssq-notify --dry-run --type %s: %v\nstderr: %s", typeName, err, stderr.String())
	}
	var body map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("dry-run output is not JSON: %v\n%s", err, stdout.String())
	}
	return body
}

// T-SN-01: ssq-notify type names must map to the proto NotificationType numbers
// (ADR-003). Before the fix `info` mapped to 1 (APPROVAL_NEEDED).
func TestSsqNotify_ShouldMapNamesToProtoNumbers_WhenDryRunPrintsType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		want  int
		proto sessionv1.NotificationType
	}{
		{"info", 10, sessionv1.NotificationType_NOTIFICATION_TYPE_INFO},
		{"approval_needed", 1, sessionv1.NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED},
		{"question", 2, sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED},
		{"error", 7, sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR},
		{"warning", 8, sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING},
		{"task_complete", 4, sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE},
		{"task_failed", 9, sessionv1.NotificationType_NOTIFICATION_TYPE_FAILURE},
		{"progress", 5, sessionv1.NotificationType_NOTIFICATION_TYPE_PROCESS_STARTED},
		{"reminder", 10, sessionv1.NotificationType_NOTIFICATION_TYPE_INFO},
		{"system", 10, sessionv1.NotificationType_NOTIFICATION_TYPE_INFO},
		{"custom", 100, sessionv1.NotificationType_NOTIFICATION_TYPE_CUSTOM},
		{"no-such-type", 10, sessionv1.NotificationType_NOTIFICATION_TYPE_INFO},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if int(tc.proto) != tc.want {
				t.Fatalf("test table drifted from proto: %s = %d, table says %d", tc.proto, int(tc.proto), tc.want)
			}
			got, ok := ssqNotifyDryRun(t, tc.name)["notificationType"].(float64)
			if !ok {
				t.Fatalf("dry-run body has no numeric notificationType")
			}
			if int(got) != tc.want {
				t.Errorf("--type %s => %d, want %d (%s)", tc.name, int(got), tc.want, tc.proto)
			}
		})
	}
}
