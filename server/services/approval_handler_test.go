package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/server/events"
)

// TestApprovalHandler_should_UseBaseURLFnValueAtCallTime_When_ThreeUsageSitesInvoked
// (REQ-3 test #4, plan.md Task 1.3.1b).
//
// Stubs the shared hookBaseURLFn (hook_injector.go) to return a distinct value on every
// invocation and drives InjectHookConfig twice against the same settings file so all three
// known usage sites (approval_handler.go: building the curl command, the "already present"
// short-circuit check, and the legacy-URL migration filter) each execute and each
// must reflect whatever baseURLFn() returns at *their* point of use -- never a
// value snapshotted once at ApprovalHandler construction time.
func TestApprovalHandler_should_UseBaseURLFnValueAtCallTime_When_ThreeUsageSitesInvoked(t *testing.T) {
	// hookApprovalURL() delegates to hook_injector.go's shared hookBaseURLFn (set via
	// SetHookBaseURLFn) -- the same mechanism InjectHooksConfig uses -- rather than a
	// separate ApprovalHandler-owned mechanism. Save/restore it so this test's
	// deliberately-unstable stub base URL doesn't leak into other tests in this
	// package that call hookApprovalURL()/InjectHookConfig and expect the stable
	// default.
	original := hookBaseURLFn
	t.Cleanup(func() { hookBaseURLFn = original })

	calls := 0
	nextAddr := func() string {
		calls++
		return fmt.Sprintf("http://localhost:%d", 20000+calls)
	}

	// baseURLFn resolves to whatever nextAddr() currently returns.
	// This exercises usage site #1 (building the curl command) with the first address.
	SetHookBaseURLFn(nextAddr)
	_ = NewApprovalHandler(NewApprovalStore(""), nil, events.NewEventBus(1))

	tmpDir := t.TempDir()
	if err := InjectHookConfig(tmpDir, "session-a"); err != nil {
		t.Fatalf("InjectHookConfig (first write): %v", err)
	}
	firstAddr := fmt.Sprintf("http://localhost:%d", 20000+calls) // whatever nextAddr() returned during that call

	settings := readSettings(t, tmpDir)
	firstGroups := permissionRequestGroups(t, settings)
	if !commandsContain(firstGroups, firstAddr) {
		t.Fatalf("expected first InjectHookConfig call to bake in the base URL current at that call (%q), got groups: %+v", firstAddr, firstGroups)
	}

	// baseURLFn is the same nextAddr closure, but the counter keeps advancing, so the base
	// URL moves forward, mirroring a server that rebinds to a different port between
	// hook-injection events -- the exact scenario the lazy baseURLFn mechanism exists to
	// support (never a string baked in at construction time).
	if err := InjectHookConfig(tmpDir, "session-a"); err != nil {
		t.Fatalf("InjectHookConfig (second write): %v", err)
	}

	settings = readSettings(t, tmpDir)
	secondGroups := permissionRequestGroups(t, settings)

	// Usage site #2 (the "already present" short-circuit) must have compared against
	// the CURRENT base URL, not the first call's -- otherwise it would have incorrectly
	// matched the stale entry and skipped writing a fresh one entirely.
	//
	// Usage site #3 (the legacy-URL migration filter) must also have evaluated against
	// the CURRENT base URL when deciding what to strip -- our command-type entries have
	// no URL field, so both survive, proving the filter ran (using the current value)
	// without wrongly discarding the earlier entry.
	if len(secondGroups) < 2 {
		t.Fatalf("expected the second InjectHookConfig call to prepend a fresh entry alongside the first (proving the 'already present' check used the CURRENT base URL, not a stale snapshot), got %d group(s): %+v", len(secondGroups), secondGroups)
	}
	if !commandsContain(secondGroups, firstAddr) {
		t.Fatalf("expected the original entry (built with %q) to survive the legacy-URL migration filter, got groups: %+v", firstAddr, secondGroups)
	}
	if strings.Contains(fmt.Sprint(secondGroups), firstAddr) && !commandsContainNewerThan(secondGroups, firstAddr) {
		t.Fatalf("expected a newly prepended entry reflecting the base URL current at the SECOND call's point of use, got groups: %+v", secondGroups)
	}
}

// permissionRequestGroups extracts the PermissionRequest hookMatcherGroup slice from a
// parsed settings.local.json top-level map, failing the test on any parse error.
func permissionRequestGroups(t *testing.T, top map[string]json.RawMessage) []hookMatcherGroup {
	t.Helper()
	hooksRaw, ok := top["hooks"]
	if !ok {
		t.Fatal("hooks key not present in settings")
	}
	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(hooksRaw, &hooks); err != nil {
		t.Fatalf("parse hooks map: %v", err)
	}
	prRaw, ok := hooks["PermissionRequest"]
	if !ok {
		t.Fatal("PermissionRequest key not present in hooks")
	}
	var groups []hookMatcherGroup
	if err := json.Unmarshal(prRaw, &groups); err != nil {
		t.Fatalf("parse PermissionRequest groups: %v", err)
	}
	return groups
}

// commandsContain reports whether any command-type hook entry across groups contains substr.
func commandsContain(groups []hookMatcherGroup, substr string) bool {
	for _, g := range groups {
		for _, h := range g.Hooks {
			if h.Type == "command" && strings.Contains(h.Command, substr) {
				return true
			}
		}
	}
	return false
}

// commandsContainNewerThan reports whether any command-type hook entry contains an
// "http://localhost:<port>" address other than staleAddr, i.e. a freshly-written entry
// distinct from the one built with staleAddr.
func commandsContainNewerThan(groups []hookMatcherGroup, staleAddr string) bool {
	for _, g := range groups {
		for _, h := range g.Hooks {
			if h.Type != "command" {
				continue
			}
			if strings.Contains(h.Command, "http://localhost:") && !strings.Contains(h.Command, staleAddr) {
				return true
			}
		}
	}
	return false
}
