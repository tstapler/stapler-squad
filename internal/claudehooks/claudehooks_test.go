package claudehooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func readBack(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	return m
}

func TestDetectStatus_MissingFile_ReportsNotInstalled(t *testing.T) {
	st, err := DetectStatus(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("DetectStatus: %v", err)
	}
	if st.RulesInstalled || st.NotificationsInstalled {
		t.Errorf("expected nothing installed for missing file, got %+v", st)
	}
}

func TestInstallRules_AddsPreToolUse_AndIsDetected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := InstallRules(path, "/home/u/.local/bin/ssq-hooks"); err != nil {
		t.Fatalf("InstallRules: %v", err)
	}
	st, _ := DetectStatus(path)
	if !st.RulesInstalled {
		t.Error("rules should be detected after install")
	}
	if st.NotificationsInstalled {
		t.Error("notifications should NOT be installed")
	}
}

func TestInstallRules_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := InstallRules(path, "/bin/ssq-hooks"); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if err := InstallRules(path, "/bin/ssq-hooks"); err != nil {
		t.Fatalf("second install: %v", err)
	}
	hooks := readBack(t, path)["hooks"].(map[string]interface{})
	groups := hooks["PreToolUse"].([]interface{})
	if len(groups) != 1 {
		t.Errorf("expected exactly 1 PreToolUse group after double install, got %d", len(groups))
	}
}

func TestInstallNotifications_AddsNotificationAndStop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := InstallNotifications(path, "/repo/scripts/ssq-hook-handler"); err != nil {
		t.Fatalf("InstallNotifications: %v", err)
	}
	st, _ := DetectStatus(path)
	if !st.NotificationsInstalled {
		t.Error("notifications should be detected after install")
	}
	if st.RulesInstalled {
		t.Error("rules should NOT be installed")
	}
	hooks := readBack(t, path)["hooks"].(map[string]interface{})
	if _, ok := hooks["Notification"]; !ok {
		t.Error("Notification event missing")
	}
	if _, ok := hooks["Stop"]; !ok {
		t.Error("Stop event missing")
	}
}

func TestInstall_PreservesExistingUnrelatedHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	seed := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Bash",
					"hooks": []interface{}{
						map[string]interface{}{"type": "command", "command": "/other/tool guard"},
					},
				},
			},
		},
		"someUserSetting": "keep-me",
	}
	raw, _ := json.MarshalIndent(seed, "", "  ")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := InstallRules(path, "/bin/ssq-hooks"); err != nil {
		t.Fatalf("InstallRules: %v", err)
	}

	got := readBack(t, path)
	if got["someUserSetting"] != "keep-me" {
		t.Error("unrelated top-level setting was lost")
	}
	groups := got["hooks"].(map[string]interface{})["PreToolUse"].([]interface{})
	if len(groups) != 2 {
		t.Fatalf("expected our hook + the existing one (2 groups), got %d", len(groups))
	}
	// Our entry is prepended.
	first := groups[0].(map[string]interface{})["hooks"].([]interface{})[0].(map[string]interface{})
	if first["command"] != "/bin/ssq-hooks check" {
		t.Errorf("our hook should be first, got %v", first["command"])
	}
}

func TestInstall_ConcurrentWrites_NoCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = InstallRules(path, "/bin/ssq-hooks") }()
		go func() { defer wg.Done(); _ = InstallNotifications(path, "/bin/ssq-hook-handler") }()
	}
	wg.Wait()

	// File must be valid JSON and both hooks present exactly once.
	got := readBack(t, path)
	st, err := DetectStatus(path)
	if err != nil {
		t.Fatalf("DetectStatus after concurrent writes: %v", err)
	}
	if !st.RulesInstalled || !st.NotificationsInstalled {
		t.Errorf("both hooks should be installed after concurrent writes, got %+v", st)
	}
	pre := got["hooks"].(map[string]interface{})["PreToolUse"].([]interface{})
	if len(pre) != 1 {
		t.Errorf("expected exactly 1 PreToolUse group (idempotent under races), got %d", len(pre))
	}
}

func TestMutate_RejectsNonObjectHooksField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"hooks": "oops"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := InstallRules(path, "/bin/ssq-hooks"); err == nil {
		t.Error("expected error when hooks field is not an object")
	}
}
