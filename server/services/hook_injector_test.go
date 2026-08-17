package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// TestRemoveHooksConfig_should_StripOnlyTheNamedHook_When_MultipleHooksPresent
// (git-drift-check steering hook, symmetric add/remove): RemoveHooksConfig removes
// only the hook(s) named, leaving every other hook (ours or the user's own) intact —
// including other entries under the same PostToolUse event key.
func TestRemoveHooksConfig_should_StripOnlyTheNamedHook_When_MultipleHooksPresent(t *testing.T) {
	tmpDir := t.TempDir()

	// Inject both PostToolUse-mapped hooks plus an unrelated event.
	if err := InjectHooksConfig(tmpDir, "sess", []HookName{HookPostToolLogging, HookGitDriftCheck, HookStopNotification}); err != nil {
		t.Fatalf("InjectHooksConfig: %v", err)
	}

	if err := RemoveHooksConfig(tmpDir, []HookName{HookGitDriftCheck}); err != nil {
		t.Fatalf("RemoveHooksConfig: %v", err)
	}

	top := readSettings(t, tmpDir)
	var hooksMap map[string]json.RawMessage
	if err := json.Unmarshal(top["hooks"], &hooksMap); err != nil {
		t.Fatalf("parse hooks: %v", err)
	}

	var postToolGroups []hookMatcherGroup
	if err := json.Unmarshal(hooksMap["PostToolUse"], &postToolGroups); err != nil {
		t.Fatalf("parse PostToolUse groups: %v", err)
	}
	foundDrift, foundLogging := false, false
	for _, g := range postToolGroups {
		for _, h := range g.Hooks {
			if strings.Contains(h.Command, "/api/hooks/post-tool-use-drift-check") {
				foundDrift = true
			}
			if strings.Contains(h.Command, "/api/hooks/post-tool-use") && !strings.Contains(h.Command, "drift-check") {
				foundLogging = true
			}
		}
	}
	if foundDrift {
		t.Error("git-drift-check hook command still present after RemoveHooksConfig")
	}
	if !foundLogging {
		t.Error("unrelated post_tool_logging hook was removed along with git-drift-check")
	}

	// Stop and PermissionRequest (always injected) must be untouched.
	if _, ok := hooksMap["Stop"]; !ok {
		t.Error("Stop hook was removed even though it wasn't named in RemoveHooksConfig")
	}
	if _, ok := hooksMap["PermissionRequest"]; !ok {
		t.Error("PermissionRequest hook was removed even though it wasn't named in RemoveHooksConfig")
	}
}

// Test_hookCommandReferencesURL_should_NotMatchWhenURLIsAStrictPrefixOfAnother is the
// deterministic regression test for the root cause behind
// TestRemoveHooksConfig_should_StripOnlyTheNamedHook_When_MultipleHooksPresent's flakiness:
// HookPostToolLogging's endpoint (".../api/hooks/post-tool-use") is a strict string prefix of
// HookGitDriftCheck's (".../api/hooks/post-tool-use-drift-check"). A bare
// strings.Contains(command, url) treated the shorter URL as present inside the longer URL's
// command, which (via Go's randomized map iteration order in InjectHooksConfig's `for hookName
// := range wanted` loop) intermittently caused one of the two PostToolUse hooks to never be
// injected at all. hookCommandReferencesURL must reject that false-positive match while still
// matching a command that genuinely references the URL.
func Test_hookCommandReferencesURL_should_NotMatchWhenURLIsAStrictPrefixOfAnother(t *testing.T) {
	shortURL := "http://localhost:8543/api/hooks/post-tool-use"
	longURL := "http://localhost:8543/api/hooks/post-tool-use-drift-check"
	commandForLongURL := "curl -s --max-time 300 -X POST '" + longURL + "' -H 'Content-Type: application/json' -d @-"

	if hookCommandReferencesURL(commandForLongURL, shortURL) {
		t.Fatalf("expected command for the longer URL %q to NOT match the shorter, prefix URL %q", longURL, shortURL)
	}
	if !hookCommandReferencesURL(commandForLongURL, longURL) {
		t.Fatalf("expected command for %q to match itself", longURL)
	}
}

// TestInjectHooksConfig_should_InjectBothHooks_When_OneEndpointIsAPrefixOfAnother is the
// end-to-end regression test for the same bug, run over many iterations because the original
// failure depended on Go's randomized map iteration order (over InjectHooksConfig's `wanted`
// set) rather than any goroutine or true data race -- a single run had roughly even odds of
// passing by luck. Pre-fix, this loop reliably failed within the first few iterations (see
// hookCommandReferencesURL's doc comment); post-fix it must pass every time regardless of
// iteration order.
func TestInjectHooksConfig_should_InjectBothHooks_When_OneEndpointIsAPrefixOfAnother(t *testing.T) {
	for i := 0; i < 50; i++ {
		tmpDir := t.TempDir()
		if err := InjectHooksConfig(tmpDir, "sess", []HookName{HookPostToolLogging, HookGitDriftCheck}); err != nil {
			t.Fatalf("iteration %d: InjectHooksConfig: %v", i, err)
		}

		settings := readSettings(t, tmpDir)
		var hooksMap map[string]json.RawMessage
		if err := json.Unmarshal(settings["hooks"], &hooksMap); err != nil {
			t.Fatalf("iteration %d: parse hooks: %v", i, err)
		}
		postToolRaw, ok := hooksMap["PostToolUse"]
		if !ok {
			t.Fatalf("iteration %d: PostToolUse key missing entirely from hooks", i)
		}
		var groups []hookMatcherGroup
		if err := json.Unmarshal(postToolRaw, &groups); err != nil {
			t.Fatalf("iteration %d: parse PostToolUse groups: %v", i, err)
		}
		foundLogging, foundDrift := false, false
		for _, g := range groups {
			for _, h := range g.Hooks {
				if strings.Contains(h.Command, "/api/hooks/post-tool-use-drift-check") {
					foundDrift = true
				} else if strings.Contains(h.Command, "/api/hooks/post-tool-use") {
					foundLogging = true
				}
			}
		}
		if !foundLogging {
			t.Fatalf("iteration %d: post_tool_logging hook missing -- it was dropped because its URL is a prefix of git_drift_check's", i)
		}
		if !foundDrift {
			t.Fatalf("iteration %d: git_drift_check hook missing", i)
		}
	}
}

// TestWriteSettingsAtomic_ConcurrentWritesToSameSettingsPath_NeverProduceCorruptJSON
// guards against the fixed-tmp-filename hazard writeSettingsAtomic previously had:
// tmpPath was settingsPath+".tmp" (not unique per call), so two concurrent writers
// targeting the same rootDir — e.g. InjectHooksConfig and RemoveHooksConfig racing
// from two goroutines on the same session's worktree, or two backlog spawns hitting
// the same reused branch — could clobber each other's temp file mid-write and leave
// a truncated/corrupt settings.local.json after rename (observed in CI as "parse
// PostToolUse groups: unexpected end of JSON input" on this package's other tests).
// internal/claudehooks/claudehooks.go's mutate() already documents this exact
// failure mode and uses a unique os.CreateTemp name for the same reason.
func TestWriteSettingsAtomic_ConcurrentWritesToSameSettingsPath_NeverProduceCorruptJSON(t *testing.T) {
	tmpDir := t.TempDir()

	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			hooks := []HookName{HookPostToolLogging, HookGitDriftCheck}
			_ = InjectHooksConfig(tmpDir, "sess", hooks)
			_ = RemoveHooksConfig(tmpDir, []HookName{HookGitDriftCheck})
		}(i)
	}
	wg.Wait()

	settingsPath := filepath.Join(tmpDir, ".claude", "settings.local.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after concurrent writes: %v", err)
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("settings.local.json is corrupt after concurrent writes: %v\ncontent: %s", err, data)
	}
	if _, ok := parsed["hooks"]; !ok {
		t.Error("expected a \"hooks\" key to survive concurrent InjectHooksConfig/RemoveHooksConfig calls")
	}
}

// TestRemoveHooksConfig_should_BeNoOp_When_HookWasNeverInjected covers the common
// case: a freshly-created (or always-manual) worktree that never had the hook, so
// every backlog spawn's "reconcile in both directions" call is a no-op there.
func TestRemoveHooksConfig_should_BeNoOp_When_HookWasNeverInjected(t *testing.T) {
	tmpDir := t.TempDir()

	// No settings file at all.
	if err := RemoveHooksConfig(tmpDir, []HookName{HookGitDriftCheck}); err != nil {
		t.Fatalf("RemoveHooksConfig on missing settings file: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(tmpDir, ".claude", "settings.local.json")); statErr == nil {
		t.Error("RemoveHooksConfig created a settings file where none existed")
	}

	// A settings file exists but has unrelated hooks only.
	if err := InjectHooksConfig(tmpDir, "sess", []HookName{HookStopNotification}); err != nil {
		t.Fatalf("InjectHooksConfig: %v", err)
	}
	before := readSettings(t, tmpDir)
	if err := RemoveHooksConfig(tmpDir, []HookName{HookGitDriftCheck}); err != nil {
		t.Fatalf("RemoveHooksConfig: %v", err)
	}
	after := readSettings(t, tmpDir)
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if string(beforeJSON) != string(afterJSON) {
		t.Errorf("settings changed even though the named hook was never present:\nbefore=%s\nafter=%s", beforeJSON, afterJSON)
	}
}

// Test_InjectHooksConfig_should_TargetRelaySocket_When_WithRemoteHookTargetPassed
// (Story 5.2.1, plan.md's first bullet under Task 5.2.1b): a remote session's generated
// PermissionRequest hook command must target RemoteApprovalRelay's remote-side Unix socket
// -- not hookBaseURLFn()'s http://localhost:8543 -- and must carry the relay's bearer token
// so RemoteApprovalRelay.verifyToken accepts the payload once written.
func Test_InjectHooksConfig_should_TargetRelaySocket_When_WithRemoteHookTargetPassed(t *testing.T) {
	tmpDir := t.TempDir()
	target := RemoteHookTarget{
		SocketPath:  "/home/agent/work/.stapler-squad-approval.sock",
		BearerToken: "test-bearer-token-abc123",
	}

	if err := InjectHooksConfig(tmpDir, "remote-sess", nil, WithRemoteHookTarget(target)); err != nil {
		t.Fatalf("InjectHooksConfig: %v", err)
	}

	top := readSettings(t, tmpDir)
	var hooksMap map[string]json.RawMessage
	if err := json.Unmarshal(top["hooks"], &hooksMap); err != nil {
		t.Fatalf("parse hooks: %v", err)
	}
	prRaw, ok := hooksMap["PermissionRequest"]
	if !ok {
		t.Fatal("PermissionRequest not found")
	}
	var groups []hookMatcherGroup
	if err := json.Unmarshal(prRaw, &groups); err != nil {
		t.Fatalf("parse PermissionRequest groups: %v", err)
	}

	var command string
	for _, g := range groups {
		for _, h := range g.Hooks {
			if strings.Contains(h.Command, target.SocketPath) {
				command = h.Command
			}
		}
	}
	if command == "" {
		t.Fatalf("no PermissionRequest hook command references socket path %q; groups=%+v", target.SocketPath, groups)
	}

	if strings.Contains(command, "http://localhost:8543") {
		t.Errorf("remote hook command must not reference localhost:8543, got: %s", command)
	}
	if !strings.Contains(command, "UNIX-CONNECT:"+target.SocketPath) {
		t.Errorf("remote hook command must target the relay socket via UNIX-CONNECT, got: %s", command)
	}
	if !strings.Contains(command, target.BearerToken) {
		t.Errorf("remote hook command must embed the relay's bearer token, got: %s", command)
	}
	if !strings.Contains(command, `"token"`) || !strings.Contains(command, `"request"`) {
		t.Errorf("remote hook command must build a JSON payload with \"token\" and \"request\" fields (relayedApprovalPayload's shape), got: %s", command)
	}
}

// Test_InjectHooksConfig_should_ProduceByteIdenticalLocalCommand_When_NoRemoteTargetPassed
// (Story 5.2.1's explicit acceptance criterion): a local session -- InjectHooksConfig called
// with no WithRemoteHookTarget option, exactly as every pre-Phase-5 call site still does --
// must generate the exact same PermissionRequest hook command as before this change.
func Test_InjectHooksConfig_should_ProduceByteIdenticalLocalCommand_When_NoRemoteTargetPassed(t *testing.T) {
	tmpDir := t.TempDir()

	if err := InjectHooksConfig(tmpDir, "local-sess", nil); err != nil {
		t.Fatalf("InjectHooksConfig: %v", err)
	}

	top := readSettings(t, tmpDir)
	var hooksMap map[string]json.RawMessage
	if err := json.Unmarshal(top["hooks"], &hooksMap); err != nil {
		t.Fatalf("parse hooks: %v", err)
	}
	var groups []hookMatcherGroup
	if err := json.Unmarshal(hooksMap["PermissionRequest"], &groups); err != nil {
		t.Fatalf("parse PermissionRequest groups: %v", err)
	}
	if len(groups) != 1 || len(groups[0].Hooks) != 1 {
		t.Fatalf("expected exactly one PermissionRequest hook group/entry, got groups=%+v", groups)
	}

	want := fmt.Sprintf(
		"curl -s --max-time %d -X POST '%s' -H 'Content-Type: application/json' -H 'X-CS-Session-ID: %s' -d @-",
		hookTimeout, hookApprovalURL(), "local-sess",
	)
	got := groups[0].Hooks[0].Command
	if got != want {
		t.Errorf("local session's PermissionRequest hook command changed by Phase 5:\n  want: %s\n  got:  %s", want, got)
	}
}

// Test_InjectHooksConfig_should_LeaveOtherHookTypesOnHTTP_When_WithRemoteHookTargetPassed
// documents and locks in WithRemoteHookTarget's scope decision: only HookPermissionApproval
// is remote-routed. RemoteApprovalRelay (Epic 5.1) only understands the approval-request
// payload shape, so routing e.g. Stop's differently-shaped JSON at the same socket would be
// silently wrong, not just unimplemented -- see WithRemoteHookTarget's doc comment.
func Test_InjectHooksConfig_should_LeaveOtherHookTypesOnHTTP_When_WithRemoteHookTargetPassed(t *testing.T) {
	tmpDir := t.TempDir()
	target := RemoteHookTarget{
		SocketPath:  "/home/agent/work/.stapler-squad-approval.sock",
		BearerToken: "test-bearer-token-abc123",
	}

	if err := InjectHooksConfig(tmpDir, "remote-sess", []HookName{HookStopNotification}, WithRemoteHookTarget(target)); err != nil {
		t.Fatalf("InjectHooksConfig: %v", err)
	}

	top := readSettings(t, tmpDir)
	var hooksMap map[string]json.RawMessage
	if err := json.Unmarshal(top["hooks"], &hooksMap); err != nil {
		t.Fatalf("parse hooks: %v", err)
	}
	var groups []hookMatcherGroup
	if err := json.Unmarshal(hooksMap["Stop"], &groups); err != nil {
		t.Fatalf("parse Stop groups: %v", err)
	}

	found := false
	for _, g := range groups {
		for _, h := range g.Hooks {
			if strings.Contains(h.Command, "/api/hooks/stop") {
				found = true
			}
			if strings.Contains(h.Command, "UNIX-CONNECT") {
				t.Errorf("Stop hook must never be routed at the relay socket, got: %s", h.Command)
			}
		}
	}
	if !found {
		t.Error("Stop hook command missing or not using the HTTP endpoint")
	}
}

// Test_WithRemoteHookTarget_should_BeNoOp_When_SocketPathEmpty guards the documented
// zero-value behavior: a caller that hasn't resolved a real relay yet (RemoteHookTarget{})
// must never emit a broken empty UNIX-CONNECT -- it silently falls back to local behavior.
func Test_WithRemoteHookTarget_should_BeNoOp_When_SocketPathEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	if err := InjectHooksConfig(tmpDir, "sess", nil, WithRemoteHookTarget(RemoteHookTarget{})); err != nil {
		t.Fatalf("InjectHooksConfig: %v", err)
	}

	top := readSettings(t, tmpDir)
	var hooksMap map[string]json.RawMessage
	if err := json.Unmarshal(top["hooks"], &hooksMap); err != nil {
		t.Fatalf("parse hooks: %v", err)
	}
	var groups []hookMatcherGroup
	if err := json.Unmarshal(hooksMap["PermissionRequest"], &groups); err != nil {
		t.Fatalf("parse PermissionRequest groups: %v", err)
	}
	for _, g := range groups {
		for _, h := range g.Hooks {
			if strings.Contains(h.Command, "UNIX-CONNECT") {
				t.Errorf("zero-value RemoteHookTarget must not produce a UNIX-CONNECT command, got: %s", h.Command)
			}
		}
	}
}

// Test_InjectHooksConfig_should_BeIdempotent_When_RemoteTargetCalledTwice mirrors
// TestInjectHooksIdempotent for the remote-socket branch: calling InjectHooksConfig twice
// with the same RemoteHookTarget must not duplicate the PermissionRequest hook entry.
// Exercises hookCommandTargetsSocket, the remote-branch analog of hookCommandReferencesURL.
func Test_InjectHooksConfig_should_BeIdempotent_When_RemoteTargetCalledTwice(t *testing.T) {
	tmpDir := t.TempDir()
	target := RemoteHookTarget{
		SocketPath:  "/home/agent/work/.stapler-squad-approval.sock",
		BearerToken: "test-bearer-token-abc123",
	}

	if err := InjectHooksConfig(tmpDir, "sess", nil, WithRemoteHookTarget(target)); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := InjectHooksConfig(tmpDir, "sess", nil, WithRemoteHookTarget(target)); err != nil {
		t.Fatalf("second call: %v", err)
	}

	top := readSettings(t, tmpDir)
	var hooksMap map[string]json.RawMessage
	if err := json.Unmarshal(top["hooks"], &hooksMap); err != nil {
		t.Fatalf("parse hooks: %v", err)
	}
	var groups []hookMatcherGroup
	if err := json.Unmarshal(hooksMap["PermissionRequest"], &groups); err != nil {
		t.Fatalf("parse PermissionRequest groups: %v", err)
	}

	count := 0
	for _, g := range groups {
		for _, h := range g.Hooks {
			if strings.Contains(h.Command, "UNIX-CONNECT:"+target.SocketPath) {
				count++
			}
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 remote PermissionRequest hook entry after 2 calls, got %d", count)
	}
}

// TestRemoveHooksConfig_should_BeIdempotent_When_CalledTwice mirrors
// TestInjectHooksIdempotent for the removal direction.
func TestRemoveHooksConfig_should_BeIdempotent_When_CalledTwice(t *testing.T) {
	tmpDir := t.TempDir()
	if err := InjectHooksConfig(tmpDir, "sess", []HookName{HookGitDriftCheck}); err != nil {
		t.Fatalf("InjectHooksConfig: %v", err)
	}
	if err := RemoveHooksConfig(tmpDir, []HookName{HookGitDriftCheck}); err != nil {
		t.Fatalf("first RemoveHooksConfig: %v", err)
	}
	if err := RemoveHooksConfig(tmpDir, []HookName{HookGitDriftCheck}); err != nil {
		t.Fatalf("second RemoveHooksConfig: %v", err)
	}

	top := readSettings(t, tmpDir)
	var hooksMap map[string]json.RawMessage
	if err := json.Unmarshal(top["hooks"], &hooksMap); err != nil {
		t.Fatalf("parse hooks: %v", err)
	}
	if raw, ok := hooksMap["PostToolUse"]; ok {
		var groups []hookMatcherGroup
		_ = json.Unmarshal(raw, &groups)
		for _, g := range groups {
			for _, h := range g.Hooks {
				if strings.Contains(h.Command, "drift-check") {
					t.Error("git-drift-check hook still present after two RemoveHooksConfig calls")
				}
			}
		}
	}
}
