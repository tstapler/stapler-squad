package goalrunner

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Static half of the hook-coexistence check: the settings file carries the SSQ
// Stop hook and never a /goal hook (that one is added by claude at runtime, so
// the two cannot collide in this file). The runtime half is documented in
// docs/explanation/goal-pilot-results.md.
func TestBuildSettings_should_KeepSSQStopHookAndOmitGoalHook_When_Built(t *testing.T) {
	t.Parallel()
	raw, err := BuildSettings("/h/ssq stop", "/h/ssq perm")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Hooks map[string][]hookGroup `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Hooks["Stop"]) != 1 || got.Hooks["Stop"][0].Hooks[0].Command != "/h/ssq stop" {
		t.Fatalf("Stop hooks = %+v", got.Hooks["Stop"])
	}
	if got.Hooks["PermissionRequest"][0].Hooks[0].Command != "/h/ssq perm" {
		t.Fatalf("PermissionRequest hooks = %+v", got.Hooks["PermissionRequest"])
	}
	if strings.Contains(string(raw), "/goal") {
		t.Fatal("settings file must not contain a /goal hook")
	}
}

func TestWriteSettings_should_WriteOwnerOnlyFile_When_DirGiven(t *testing.T) {
	t.Parallel()
	path, err := WriteSettings(t.TempDir(), "s", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "goal-settings.json") {
		t.Fatal(path)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", fi.Mode().Perm())
	}
	if raw, err := os.ReadFile(path); err != nil || strings.Contains(string(raw), "PermissionRequest") {
		t.Fatalf("empty permissionHook must be omitted: err=%v raw=%s", err, raw)
	}
}
