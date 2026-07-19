package services

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestInjectHooksConfigAllTypes (U-3.7): InjectHooksConfig injects all five hook types
// with correct event keys and session-ID headers.
func TestInjectHooksConfigAllTypes(t *testing.T) {
	tmpDir := t.TempDir()
	hooks := []HookName{
		HookPermissionApproval,
		HookStopNotification,
		HookPreToolLogging,
		HookPostToolLogging,
		HookPromptSubmit,
	}

	if err := InjectHooksConfig(tmpDir, "my-session", hooks); err != nil {
		t.Fatalf("InjectHooksConfig: %v", err)
	}

	top := readSettings(t, tmpDir)
	hooksRaw, ok := top["hooks"]
	if !ok {
		t.Fatal("hooks key not present in settings")
	}

	var hooksMap map[string]json.RawMessage
	if err := json.Unmarshal(hooksRaw, &hooksMap); err != nil {
		t.Fatalf("parse hooks: %v", err)
	}

	type expectation struct {
		eventKey string
		urlFrag  string
	}
	expectations := []expectation{
		{"PermissionRequest", "/api/hooks/permission-request"},
		{"Stop", "/api/hooks/stop"},
		{"PreToolUse", "/api/hooks/pre-tool-use"},
		{"PostToolUse", "/api/hooks/post-tool-use"},
		{"UserPromptSubmit", "/api/hooks/prompt-submit"},
	}

	for _, exp := range expectations {
		raw, ok := hooksMap[exp.eventKey]
		if !ok {
			t.Errorf("event %s not found in hooks", exp.eventKey)
			continue
		}
		var groups []hookMatcherGroup
		if err := json.Unmarshal(raw, &groups); err != nil {
			t.Errorf("parse %s groups: %v", exp.eventKey, err)
			continue
		}
		found := false
		sessionHeader := false
		for _, g := range groups {
			for _, h := range g.Hooks {
				if strings.Contains(h.Command, exp.urlFrag) {
					found = true
					if strings.Contains(h.Command, "X-CS-Session-ID: my-session") {
						sessionHeader = true
					}
				}
			}
		}
		if !found {
			t.Errorf("event %s: no hook command containing %q", exp.eventKey, exp.urlFrag)
		}
		if !sessionHeader {
			t.Errorf("event %s: X-CS-Session-ID header with 'my-session' not found", exp.eventKey)
		}
	}
}

// TestInjectHooksConfigPreservesUserHooks (U-3.8): existing user hooks are preserved
// and our hook is prepended.
func TestInjectHooksConfigPreservesUserHooks(t *testing.T) {
	tmpDir := t.TempDir()
	existing := `{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"lint-hook","timeout":10}]}]}}`
	writeSettings(t, tmpDir, existing)

	if err := InjectHooksConfig(tmpDir, "test-session", []HookName{HookPreToolLogging}); err != nil {
		t.Fatalf("InjectHooksConfig: %v", err)
	}

	top := readSettings(t, tmpDir)
	hooksRaw := top["hooks"]
	var hooksMap map[string]json.RawMessage
	if err := json.Unmarshal(hooksRaw, &hooksMap); err != nil {
		t.Fatalf("parse hooks: %v", err)
	}

	raw, ok := hooksMap["PreToolUse"]
	if !ok {
		t.Fatal("PreToolUse not found")
	}
	var groups []hookMatcherGroup
	if err := json.Unmarshal(raw, &groups); err != nil {
		t.Fatalf("parse PreToolUse groups: %v", err)
	}

	if len(groups) < 2 {
		t.Fatalf("expected at least 2 hook groups (ours + user's), got %d", len(groups))
	}

	// Our hook must be first.
	firstHooks := groups[0].Hooks
	ourFound := false
	for _, h := range firstHooks {
		if strings.Contains(h.Command, "/api/hooks/pre-tool-use") {
			ourFound = true
			break
		}
	}
	if !ourFound {
		t.Error("our pre-tool-use hook is not the first group")
	}

	// User's hook must still exist somewhere.
	userFound := false
	for _, g := range groups {
		for _, h := range g.Hooks {
			if h.Command == "lint-hook" {
				userFound = true
				break
			}
		}
	}
	if !userFound {
		t.Error("user's lint-hook was removed")
	}
}

// TestPermissionApprovalAlwaysInjected (U-3.9): even when the hooks slice is empty,
// PermissionRequest is always injected.
func TestPermissionApprovalAlwaysInjected(t *testing.T) {
	tmpDir := t.TempDir()

	if err := InjectHooksConfig(tmpDir, "test-session", []HookName{}); err != nil {
		t.Fatalf("InjectHooksConfig: %v", err)
	}

	top := readSettings(t, tmpDir)
	hooksRaw, ok := top["hooks"]
	if !ok {
		t.Fatal("hooks key not present")
	}
	var hooksMap map[string]json.RawMessage
	if err := json.Unmarshal(hooksRaw, &hooksMap); err != nil {
		t.Fatalf("parse hooks: %v", err)
	}

	if _, ok := hooksMap["PermissionRequest"]; !ok {
		t.Error("PermissionRequest not present even though permission_approval is always injected")
	}

	// Stop, PreToolUse, PostToolUse, UserPromptSubmit must NOT be present.
	for _, absent := range []string{"Stop", "PreToolUse", "PostToolUse", "UserPromptSubmit"} {
		if _, ok := hooksMap[absent]; ok {
			t.Errorf("event %s should not be present when not requested", absent)
		}
	}
}

// TestInjectHooksNeverCorruptsJSON (P-3, property-based): InjectHooksConfig always
// produces valid JSON and preserves existing top-level keys.
func TestInjectHooksNeverCorruptsJSON(t *testing.T) {
	bases := []string{
		`{}`,
		`{"other": "data"}`,
		`{"hooks": {}}`,
		`{"mcpServers": {"other": {"type": "stdio", "command": "other"}}}`,
		`{"hooks": {"PreToolUse": []}}`,
	}

	for _, base := range bases {
		base := base // capture
		t.Run(base, func(t *testing.T) {
			tmpDir := t.TempDir()
			writeSettings(t, tmpDir, base)

			if err := InjectHooksConfig(tmpDir, "s", []HookName{HookPermissionApproval}); err != nil {
				t.Fatalf("InjectHooksConfig: %v", err)
			}

			// Must produce valid JSON.
			top := readSettings(t, tmpDir)

			// Verify original top-level keys (other than hooks/mcpServers) are preserved.
			var original map[string]json.RawMessage
			if err := json.Unmarshal([]byte(base), &original); err != nil {
				t.Fatalf("parse base: %v", err)
			}
			for k := range original {
				if k == "hooks" {
					continue // hooks gets merged, not removed, so skip exact check
				}
				if _, ok := top[k]; !ok {
					t.Errorf("top-level key %q from base was removed", k)
				}
			}
		})
	}
}

// TestInjectHooksIdempotent: calling InjectHooksConfig twice with the same arguments
// must not duplicate hook entries.
func TestInjectHooksIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	hooks := []HookName{HookPermissionApproval, HookStopNotification}

	if err := InjectHooksConfig(tmpDir, "sess", hooks); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := InjectHooksConfig(tmpDir, "sess", hooks); err != nil {
		t.Fatalf("second call: %v", err)
	}

	top := readSettings(t, tmpDir)
	hooksRaw := top["hooks"]
	var hooksMap map[string]json.RawMessage
	if err := json.Unmarshal(hooksRaw, &hooksMap); err != nil {
		t.Fatalf("parse hooks: %v", err)
	}

	// Count PermissionRequest hook groups that contain our URL.
	prRaw, ok := hooksMap["PermissionRequest"]
	if !ok {
		t.Fatal("PermissionRequest not found")
	}
	var groups []hookMatcherGroup
	if err := json.Unmarshal(prRaw, &groups); err != nil {
		t.Fatalf("parse PermissionRequest: %v", err)
	}

	count := 0
	for _, g := range groups {
		for _, h := range g.Hooks {
			if strings.Contains(h.Command, hookApprovalURL()) {
				count++
			}
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 PermissionRequest hook entry after 2 calls, got %d", count)
	}
}

// Test_hookEndpoints_should_ReflectCurrentBaseURLFn_When_CalledTwiceWithDifferentAddresses
// (REQ-3 test #3, plan.md Task 1.3.1c).
//
// Calls hookEndpoints(fn) once with fn returning http://localhost:0, then again with fn
// returning http://localhost:54211, and asserts the second call's map contains the new
// address -- proving the map is rebuilt fresh from baseURLFn() on every call rather than
// cached at package-construction time (the old hookEndpoint package-level var's behavior).
func Test_hookEndpoints_should_ReflectCurrentBaseURLFn_When_CalledTwiceWithDifferentAddresses(t *testing.T) {
	first := hookEndpoints(func() string { return "http://localhost:0" })
	for name, url := range first {
		if !strings.Contains(url, "http://localhost:0") {
			t.Fatalf("expected first call's %s endpoint to use http://localhost:0, got %q", name, url)
		}
	}
	if got := first[HookStopNotification]; got != "http://localhost:0/api/hooks/stop" {
		t.Fatalf("expected first call's Stop endpoint to be http://localhost:0/api/hooks/stop, got %q", got)
	}

	second := hookEndpoints(func() string { return "http://localhost:54211" })
	if got := second[HookStopNotification]; got != "http://localhost:54211/api/hooks/stop" {
		t.Fatalf("expected second call's Stop endpoint to be http://localhost:54211/api/hooks/stop, got %q", got)
	}
	if got := second[HookPermissionApproval]; got != "http://localhost:54211/api/hooks/permission-request" {
		t.Fatalf("expected second call's PermissionRequest endpoint to be http://localhost:54211/api/hooks/permission-request, got %q", got)
	}

	// The second call must not retain any trace of the first call's address --
	// proving the map is rebuilt fresh each call, never cached.
	for name, url := range second {
		if strings.Contains(url, "localhost:0") {
			t.Fatalf("expected hookEndpoints to be rebuilt fresh per call, but %s endpoint leaked the first call's stale address: %q", name, url)
		}
	}
}
