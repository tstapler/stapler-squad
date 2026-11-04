package session

// Story 1.5.8: an open transition_denied row parks the automatic actions behind
// it (spawn points and detectors) instead of churning sessions, and the sweep
// releases it once the engine allows the transition again.

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session/domain"
)

// --- fixtures ----------------------------------------------------------------

func denOpenDenial(t *testing.T, h *gateHarness, itemID string, caller TransitionCaller, from, to BacklogStatus, reason string) {
	t.Helper()
	applied, err := h.storage.MarkStuck(context.Background(), itemID, domain.StuckReasonTransitionDenied, from,
		domain.FormatTransitionDeniedContext(domain.TransitionDenial{Caller: string(caller), From: string(from), To: string(to), Reason: reason}, "test"))
	require.NoError(t, err)
	require.True(t, applied, "the item must be at %s for the denial to anchor", from)
}

// denLiftGates removes every operator gate, as an approval or a config edit would.
func denLiftGates(t *testing.T, h *gateHarness) {
	t.Helper()
	ctx := context.Background()
	_, err := h.repo.client.TransitionGate.Delete().Exec(ctx)
	require.NoError(t, err)
	require.NoError(t, h.cfg.InvalidateCache(ctx))
}

func denBlock(t *testing.T, h *gateHarness, from, to BacklogStatus) {
	t.Helper()
	addGateToSeededEdge(t, h.repo.client, h.cfg, from, to, GateKindHumanApproval, nil)
}

func denOpenRows(t *testing.T, h *gateHarness, itemID string, reason domain.StuckReason) []OpenStuckStateData {
	t.Helper()
	rows, err := h.storage.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	var out []OpenStuckStateData
	for _, r := range rows {
		if r.ItemID == itemID && r.Reason == reason {
			out = append(out, r)
		}
	}
	return out
}

// --- predicate ---------------------------------------------------------------

func TestHasOpenTransitionDenied_should_MatchOnlyARowWhoseFromIsTheCurrentStatus(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	item := h.newItem(t, BacklogStatusIdea)

	held, err := h.storage.HasOpenTransitionDenied(ctx, item.ID, BacklogStatusIdea)
	require.NoError(t, err)
	assert.False(t, held, "no row")

	denOpenDenial(t, h, item.ID, CallerAutonomousComplete, BacklogStatusIdea, BacklogStatusReady, DeniedOperatorGate)
	held, err = h.storage.HasOpenTransitionDenied(ctx, item.ID, BacklogStatusIdea)
	require.NoError(t, err)
	assert.True(t, held, "row from idea, item in idea")

	held, err = h.storage.HasOpenTransitionDenied(ctx, item.ID, BacklogStatusReady)
	require.NoError(t, err)
	assert.False(t, held, "stale row: the item is no longer in the recorded from status")
}

func TestHasOpenTransitionDenied_should_IgnoreADefaultGuardDenial(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	item := h.newItem(t, BacklogStatusReady)
	denOpenDenial(t, h, item.ID, CallerAttachSessionToItem, BacklogStatusReady, BacklogStatusInProgress, DeniedDefaultGuard)

	held, err := h.storage.HasOpenTransitionDenied(context.Background(), item.ID, BacklogStatusReady)

	require.NoError(t, err)
	assert.False(t, held, "a default_guard denial is the caller's own input and is retried as before")
}

func TestHasOpenTransitionDenied_should_IgnoreASnoozedRow(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	item := h.newItem(t, BacklogStatusIdea)
	denOpenDenial(t, h, item.ID, CallerAutonomousComplete, BacklogStatusIdea, BacklogStatusReady, DeniedOperatorGate)
	_, err := h.storage.SnoozeStuckState(ctx, item.ID, domain.StuckReasonTransitionDenied, time.Now().Add(time.Hour))
	require.NoError(t, err)

	held, err := h.storage.HasOpenTransitionDenied(ctx, item.ID, BacklogStatusIdea)

	require.NoError(t, err)
	assert.False(t, held, "an operator snooze releases the item")
}

// --- hold, release and re-drive ------------------------------------------------

func TestTransitionDenialHolds_should_HoldWhileDeniedAndResolveTheRowOnceLifted(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	denBlock(t, h, BacklogStatusIdea, BacklogStatusReady)
	item := h.newItem(t, BacklogStatusIdea)
	denOpenDenial(t, h, item.ID, CallerAutonomousComplete, BacklogStatusIdea, BacklogStatusReady, DeniedOperatorGate)

	assert.True(t, h.storage.TransitionDenialHolds(ctx, item), "the gate is still unsatisfied")
	assert.Len(t, denOpenRows(t, h, item.ID, domain.StuckReasonTransitionDenied), 1)

	denLiftGates(t, h)
	assert.False(t, h.storage.TransitionDenialHolds(ctx, item), "the gate is satisfied: the action proceeds")
	assert.Empty(t, denOpenRows(t, h, item.ID, domain.StuckReasonTransitionDenied), "the lifted denial is resolved")
}

func TestTransitionDenialHolds_should_Release_When_TheClassIsNoLongerEnforced(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	denBlock(t, h, BacklogStatusIdea, BacklogStatusReady)
	item := h.newItem(t, BacklogStatusIdea)
	denOpenDenial(t, h, item.ID, CallerAutonomousComplete, BacklogStatusIdea, BacklogStatusReady, DeniedOperatorGate)
	require.True(t, h.storage.TransitionDenialHolds(ctx, item))

	h.sel.modes = allClasses(ModeShadow)

	assert.False(t, h.storage.TransitionDenialHolds(ctx, item), "rolling the flag back to Shadow frees the item")
}

func TestRedriveTransitionDenials_should_MoveTheItemThroughTheGate_When_TheGateIsSatisfied(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	denBlock(t, h, BacklogStatusIdea, BacklogStatusReady)
	item := h.newItem(t, BacklogStatusIdea)
	denOpenDenial(t, h, item.ID, CallerAutonomousComplete, BacklogStatusIdea, BacklogStatusReady, DeniedOperatorGate)

	for range 3 {
		assert.Zero(t, h.storage.RedriveTransitionDenials(ctx), "a persisting denial resolves nothing")
	}
	assert.Equal(t, "idea", h.status(t, item.ID))
	assert.Len(t, denOpenRows(t, h, item.ID, domain.StuckReasonTransitionDenied), 1, "one row however many ticks")
	assert.Empty(t, h.rep.snapshot(), "the sweep reports nothing new while the reason is unchanged")

	denLiftGates(t, h)
	assert.Equal(t, 1, h.storage.RedriveTransitionDenials(ctx))

	assert.Equal(t, "ready", h.status(t, item.ID), "the pure-status caller is re-driven through the gate")
	assert.Empty(t, denOpenRows(t, h, item.ID, domain.StuckReasonTransitionDenied))
}

func TestRedriveTransitionDenials_should_OnlyResolveTheRow_When_TheCallerIsNotAPureStatusWrite(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	denBlock(t, h, BacklogStatusQueued, BacklogStatusInProgress)
	item := h.newItem(t, BacklogStatusQueued)
	denOpenDenial(t, h, item.ID, CallerClaimAndSpawn, BacklogStatusQueued, BacklogStatusInProgress, DeniedOperatorGate)

	denLiftGates(t, h)
	assert.Equal(t, 1, h.storage.RedriveTransitionDenials(ctx))

	assert.Equal(t, "queued", h.status(t, item.ID), "the claim spawns a session; only its own spawn point may write in_progress")
	assert.Empty(t, denOpenRows(t, h, item.ID, domain.StuckReasonTransitionDenied))
}

func TestRedriveTransitionDenials_should_ReReportWithTheNewReason_When_TheTargetStageWasDeleted(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	rigs := denialRigs()
	item, to := rigs[len(rigs)-1].setup(t, h) // the target-stage-removed rig
	denOpenDenial(t, h, item.ID, CallerAutonomousComplete, BacklogStatus("gone-from"), to, DeniedOperatorGate)

	assert.Zero(t, h.storage.RedriveTransitionDenials(ctx))

	calls := h.rep.snapshot()
	require.Len(t, calls, 1, "the changed reason is reported, not dropped")
	assert.Equal(t, DeniedTargetStageRemoved, calls[0].reason)
	assert.Equal(t, to, calls[0].to)
}

func TestSelfHealStuck_should_ResolveATransitionDeniedRowOnlyOnceTheItemLeftItsFromStatus(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	item := h.newItem(t, BacklogStatusIdea)
	denOpenDenial(t, h, item.ID, CallerAutonomousComplete, BacklogStatusIdea, BacklogStatusReady, DeniedOperatorGate)
	l := NewBacklogLifecycleListener(h.storage)

	l.selfHealStuck(ctx, h.repo)
	assert.Len(t, denOpenRows(t, h, item.ID, domain.StuckReasonTransitionDenied), 1, "still anchored at idea")

	h.moveRaw(t, item.ID, BacklogStatusReady)
	l.selfHealStuck(ctx, h.repo)
	assert.Empty(t, denOpenRows(t, h, item.ID, domain.StuckReasonTransitionDenied), "the item moved on")
}

// --- detector-local guards (kind D) -----------------------------------------------

func TestReconcileOrphanedTriageItems_should_NotMarkOrphaned_When_IdeaHasAnOpenDenial(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		denied  bool
		wantRow int
	}{{"open denial", true, 0}, {"no denial", false, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newGateHarness(t, ModeEnforce)
			ctx := context.Background()
			denBlock(t, h, BacklogStatusIdea, BacklogStatusReady)
			item := newEndedTriageTestItem(t, h.storage, h.repo)
			if tc.denied {
				denOpenDenial(t, h, item.ID, CallerAutonomousComplete, BacklogStatusIdea, BacklogStatusReady, DeniedOperatorGate)
			}

			NewBacklogLifecycleListener(h.storage).reconcileOrphanedTriageItems(ctx, h.repo)

			assert.Len(t, denOpenRows(t, h, item.ID, domain.StuckReasonOrphanedTriage), tc.wantRow)
		})
	}
}

func TestReconcileOrphanedTriageRemediation_should_SpendNoAttempt_When_IdeaHasAnOpenDenial(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	denBlock(t, h, BacklogStatusIdea, BacklogStatusReady)
	item := newEndedTriageTestItem(t, h.storage, h.repo)
	_, err := h.storage.MarkStuck(ctx, item.ID, domain.StuckReasonOrphanedTriage, BacklogStatusIdea, "pre-existing orphan row")
	require.NoError(t, err)
	denOpenDenial(t, h, item.ID, CallerAutonomousComplete, BacklogStatusIdea, BacklogStatusReady, DeniedOperatorGate)

	NewBacklogLifecycleListener(h.storage).reconcileOrphanedTriageRemediation(ctx, h.repo)

	rows := denOpenRows(t, h, item.ID, domain.StuckReasonOrphanedTriage)
	require.Len(t, rows, 1)
	assert.Zero(t, rows[0].RemediationAttempts)
}

func TestReconcileOrphanedAgentPRs_should_SkipTheGitHubLookup_When_ReviewHasAnOpenDenial(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		denied    bool
		wantCalls int
	}{{"open denial", true, 0}, {"no denial", false, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newGateHarness(t, ModeEnforce)
			denBlock(t, h, BacklogStatusReview, BacklogStatusPRPending)
			item := newOrphanedAgentPRTestItem(t, h.storage, "backlog/den-orphan")
			if tc.denied {
				denOpenDenial(t, h, item.ID, CallerOrphanedAgentPR, BacklogStatusReview, BacklogStatusPRPending, DeniedOperatorGate)
			}
			lookups := 0
			l := NewBacklogLifecycleListener(h.storage)
			l.SetOrphanedPRFinder(func(context.Context, string, string) (*github.PRInfo, error) {
				lookups++
				return nil, github.ErrNoPR
			})

			l.reconcileOrphanedAgentPRs(context.Background(), h.repo)

			assert.Equal(t, tc.wantCalls, lookups)
		})
	}
}

func TestReconcilePushFailedItems_should_SpendNoAttempt_When_ReviewHasAnOpenDenial(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	denBlock(t, h, BacklogStatusReview, BacklogStatusPRPending)
	item := h.newItem(t, BacklogStatusReview)
	_, err := h.storage.MarkStuck(ctx, item.ID, domain.StuckReasonPushFailed, BacklogStatusReview, "push rejected")
	require.NoError(t, err)
	denOpenDenial(t, h, item.ID, CallerPushCreatePRFallback, BacklogStatusReview, BacklogStatusPRPending, DeniedOperatorGate)

	NewBacklogLifecycleListener(h.storage).reconcilePushFailedItems(ctx, h.repo)

	rows := denOpenRows(t, h, item.ID, domain.StuckReasonPushFailed)
	require.Len(t, rows, 1)
	assert.Zero(t, rows[0].RemediationAttempts, "a held item spends no backoff attempt")
}

func TestReconcileUnprocessedReviewVerdicts_should_NotApplyTheVerdict_When_ReviewHasAnOpenDenial(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		denied     bool
		wantStatus BacklogStatus
	}{{"open denial", true, BacklogStatusReview}, {"no denial", false, BacklogStatusDone}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newGateHarness(t, ModeEnforce)
			ctx := context.Background()
			item := newStuckReviewTestItem(t, h.storage, ReviewVerdictPass, false, true)
			if tc.denied {
				denBlock(t, h, BacklogStatusReview, BacklogStatusDone)
				denOpenDenial(t, h, item.ID, CallerPushCreatePRFallback, BacklogStatusReview, BacklogStatusDone, DeniedOperatorGate)
			}
			l := NewBacklogLifecycleListener(h.storage)
			l.SetSessionLivenessChecker(func(sessionUUID string) bool { return !strings.HasPrefix(sessionUUID, "headless-review-") })

			l.reconcileUnprocessedReviewVerdicts(ctx, h.repo)

			assert.Equal(t, string(tc.wantStatus), h.status(t, item.ID))
		})
	}
}

// --- completeness -----------------------------------------------------------------

// denKind classifies a runStuckDetector name against an open denial (plan table).
type denKind string

const (
	denSpawn   denKind = "S"  // acts through a BacklogService spawn point that holds the guard
	denLocal   denKind = "D"  // detector-local guard
	denWriter  denKind = "W"  // is itself the status writer; the gate reports the denial, the sweep retries quietly
	denResolve denKind = "R"  // resolve-only, writes no status and spawns nothing
	denDetect  denKind = "DO" // detection only: one row, one notification, idempotent
	denNA      denKind = "N"  // not applicable, with the reason in the table
	denSweep   denKind = "RD" // the re-drive sweep itself
)

// denDetectorKinds has one entry per runStuckDetector name; adding a detector
// without classifying it fails TestEveryStuckDetectorIsClassified.
var denDetectorKinds = map[string]denKind{
	"transition_denied_redrive":   denSweep,
	"work_commit_refresh":         denNA, // refreshes git activity on live sessions; no status write
	"stale_work":                  denSpawn,
	"rework_blocked_stale":        denResolve,
	"respawn_blocked_active":      denResolve,
	"abandoned_review":            denSpawn,  // AutoRespawnReview
	"orphaned_agent_pr":           denLocal,  // also the writer: skips the per-tick GitHub lookup
	"unprocessed_review_verdict":  denLocal,  // would reopen (AutoReopenAfterFailedReview) or ship under the denial
	"bouncing":                    denSpawn,  // AutoReopenAfterFailedReview
	"repeated_noop_dispatch":      denDetect, // flags so the dispatcher gate stops respawning
	"push_failed":                 denLocal,  // a retried push spends attempts and ends in the denied write
	"orphaned_triage":             denLocal,  // shape 2 only; shapes 1 and 3 are detection only
	"orphaned_triage_remediation": denSpawn,  // AutoRespawnTriage, plus a local guard ahead of RemediationDue
	"gate_timeout":                denNA,     // custom gate invocations, not a status write
	"plan_not_approved":           denDetect,
	"self_heal":                   denNA, // resolves rows; never writes status
	"multi_reason_escalation":     denNA, // a lone transition_denied row is non-escalating (tested below)
	"auto_archive_done":           denWriter,
	"archive_terminal_sessions":   denNA, // archives sessions of done/archived items
	"cleanup_terminal_worktrees":  denNA, // cleans worktrees of done/archived items
	"pr_drift_recovery":           denWriter,
	"pr_pending_no_pr":            denSpawn, // AutoReopenForPRFix
	"pr_ready+merge_detection":    denWriter,
	"blocked_by_dependency":       denResolve,
	"dequeue_backlog_items":       denSpawn, // DequeueNextQueuedItems skips a held claim before attempting it
	"trigger_chain_reconcile":     denNA,    // fires a chained session; writes no status (chain_firer.go)
}

func denSourceFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(rel)
	require.NoError(t, err)
	return string(b)
}

func TestEveryStuckDetectorIsClassified(t *testing.T) {
	t.Parallel()
	re := regexp.MustCompile(`l\.runStuckDetector\("([^"]+)"`)
	matches := re.FindAllStringSubmatch(denSourceFile(t, "backlog_lifecycle.go"), -1)
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m[1])
	}
	require.NotEmpty(t, names)
	sort.Strings(names)

	classified := make([]string, 0, len(denDetectorKinds))
	for name := range denDetectorKinds {
		classified = append(classified, name)
	}
	sort.Strings(classified)
	assert.Equal(t, classified, names, "every runStuckDetector name needs a kind in denDetectorKinds, and no entry may be stale")
}

// denCallsTo lists the functions (as Name or Recv.Name) in dir's non-test files
// that call a method named one of calls.
func denCallsTo(t *testing.T, dir string, calls ...string) map[string]bool {
	t.Helper()
	want := map[string]bool{}
	for _, c := range calls {
		want[c] = true
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	require.NoError(t, err)
	found := map[string]bool{}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, f, nil, 0)
		require.NoError(t, err)
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok && want[sel.Sel.Name] {
						found[denFuncName(fn)+"->"+sel.Sel.Name] = true
					}
				}
				return true
			})
		}
	}
	return found
}

func denFuncName(fn *ast.FuncDecl) string {
	if fn.Recv != nil && len(fn.Recv.List) == 1 {
		t := fn.Recv.List[0].Type
		if star, ok := t.(*ast.StarExpr); ok {
			t = star.X
		}
		if id, ok := t.(*ast.Ident); ok {
			return id.Name + "." + fn.Name.Name
		}
	}
	return fn.Name.Name
}

// TestDeniedGuards_should_AllShareOnePredicate pins the collapse: the open-denial
// predicate is reached through exactly the guard helpers, never copied per detector.
func TestDeniedGuards_should_AllShareOnePredicate(t *testing.T) {
	t.Parallel()
	got := denCallsTo(t, ".", "HasOpenTransitionDenied", "OpenTransitionDenial", "TransitionDenialHolds")
	assert.Equal(t, map[string]bool{
		"Storage.HasOpenTransitionDenied->OpenTransitionDenial":                  true,
		"Storage.TransitionDenialHolds->OpenTransitionDenial":                    true,
		"BacklogLifecycleListener.heldByTransitionDenial->TransitionDenialHolds": true,
	}, got)
}

func TestDetectorLocalGuards_should_CallTheSharedHelper(t *testing.T) {
	t.Parallel()
	got := denCallsTo(t, ".", "heldByTransitionDenial")
	for _, fn := range []string{
		"BacklogLifecycleListener.respawnReviewGatesWithoutGate",
		"BacklogLifecycleListener.reconcileUnprocessedReviewVerdicts",
		"BacklogLifecycleListener.reconcileOrphanedTriageItems",
		"BacklogLifecycleListener.reconcileOrphanedTriageRemediation",
		"BacklogLifecycleListener.reconcileOrphanedAgentPRs",
		"BacklogLifecycleListener.reconcilePushFailedItems",
	} {
		assert.Truef(t, got[fn+"->heldByTransitionDenial"], "%s must call the open-denial guard", fn)
	}
}

func TestReconcileMultiReasonEscalation_should_TreatALoneTransitionDeniedRowAsNonEscalating(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	denBlock(t, h, BacklogStatusIdea, BacklogStatusReady)
	item := h.newItem(t, BacklogStatusIdea)
	denOpenDenial(t, h, item.ID, CallerAutonomousComplete, BacklogStatusIdea, BacklogStatusReady, DeniedOperatorGate)

	NewBacklogLifecycleListener(h.storage).reconcileMultiReasonEscalation(ctx, h.repo)

	assert.Empty(t, denOpenRows(t, h, item.ID, domain.StuckReasonMultipleReasons))
}
