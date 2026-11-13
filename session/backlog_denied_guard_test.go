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

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// --- fixtures ----------------------------------------------------------------

// denLiftGates removes every operator gate, as an approval or a config edit would.
func denLiftGates(t *testing.T, h *gateHarness) {
	t.Helper()
	ctx := context.Background()
	_, err := h.repo.Client.TransitionGate.Delete().Exec(ctx)
	require.NoError(t, err)
	require.NoError(t, h.cfg.InvalidateCache(ctx))
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

	denOpenDenial(t, h, item.ID, domain.CallerAutonomousComplete, BacklogStatusIdea, BacklogStatusReady, backlogdto.DeniedOperatorGate)
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
	denOpenDenial(t, h, item.ID, domain.CallerAttachSessionToItem, BacklogStatusReady, BacklogStatusInProgress, backlogdto.DeniedDefaultGuard)

	held, err := h.storage.HasOpenTransitionDenied(context.Background(), item.ID, BacklogStatusReady)

	require.NoError(t, err)
	assert.False(t, held, "a default_guard denial is the caller's own input and is retried as before")
}

func TestHasOpenTransitionDenied_should_IgnoreASnoozedRow(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	item := h.newItem(t, BacklogStatusIdea)
	denOpenDenial(t, h, item.ID, domain.CallerAutonomousComplete, BacklogStatusIdea, BacklogStatusReady, backlogdto.DeniedOperatorGate)
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
	denOpenDenial(t, h, item.ID, domain.CallerAutonomousComplete, BacklogStatusIdea, BacklogStatusReady, backlogdto.DeniedOperatorGate)

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
	denOpenDenial(t, h, item.ID, domain.CallerAutonomousComplete, BacklogStatusIdea, BacklogStatusReady, backlogdto.DeniedOperatorGate)
	require.True(t, h.storage.TransitionDenialHolds(ctx, item))

	h.sel.Modes = allClasses(ModeShadow)

	assert.False(t, h.storage.TransitionDenialHolds(ctx, item), "rolling the flag back to Shadow frees the item")
}

func TestRedriveTransitionDenials_should_MoveTheItemThroughTheGate_When_TheGateIsSatisfied(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	denBlock(t, h, BacklogStatusIdea, BacklogStatusReady)
	item := h.newItem(t, BacklogStatusIdea)
	denOpenDenial(t, h, item.ID, domain.CallerAutonomousComplete, BacklogStatusIdea, BacklogStatusReady, backlogdto.DeniedOperatorGate)

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
	denOpenDenial(t, h, item.ID, domain.CallerClaimAndSpawn, BacklogStatusQueued, BacklogStatusInProgress, backlogdto.DeniedOperatorGate)

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
	denOpenDenial(t, h, item.ID, domain.CallerAutonomousComplete, BacklogStatus("gone-from"), to, backlogdto.DeniedOperatorGate)

	assert.Zero(t, h.storage.RedriveTransitionDenials(ctx))

	calls := h.rep.snapshot()
	require.Len(t, calls, 1, "the changed reason is reported, not dropped")
	assert.Equal(t, backlogdto.DeniedTargetStageRemoved, calls[0].reason)
	assert.Equal(t, to, calls[0].to)
}

// --- detector-local guards (kind D) -----------------------------------------------

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

// denDetectorKinds has one entry per detector in backlog/stuck_detectors.go; adding a detector
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
	"trigger_chain_reconcile":     denNA,    // fires a chained session; writes no status (backlog/chains/firer.go)
}

// denScanDirs are the directories the guard scans: the session core and the backlog lifecycle package.
var denScanDirs = []string{".", "backlog"}

func denSourceFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(rel)
	require.NoError(t, err)
	return string(b)
}

func TestEveryStuckDetectorIsClassified(t *testing.T) {
	t.Parallel()
	re := regexp.MustCompile(`funcStuckDetector\{"([^"]+)"`)
	matches := re.FindAllStringSubmatch(denSourceFile(t, "backlog/stuck_detectors.go"), -1)
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
	assert.Equal(t, classified, names, "every stuck detector name needs a kind in denDetectorKinds, and no entry may be stale")
}

// denCallsTo lists the functions (as Name or Recv.Name) in the dirs' non-test files
// that call a method named one of calls.
func denCallsTo(t *testing.T, dirs []string, calls ...string) map[string]bool {
	t.Helper()
	want := map[string]bool{}
	for _, c := range calls {
		want[c] = true
	}
	var files []string
	for _, dir := range dirs {
		matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
		require.NoError(t, err)
		files = append(files, matches...)
	}
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
	got := denCallsTo(t, denScanDirs, "HasOpenTransitionDenied", "OpenTransitionDenial", "TransitionDenialHolds")
	assert.Equal(t, map[string]bool{
		"Storage.HasOpenTransitionDenied->OpenTransitionDenial":                  true,
		"Storage.TransitionDenialHolds->OpenTransitionDenial":                    true,
		"BacklogLifecycleListener.heldByTransitionDenial->TransitionDenialHolds": true,
	}, got)
}

func TestDetectorLocalGuards_should_CallTheSharedHelper(t *testing.T) {
	t.Parallel()
	got := denCallsTo(t, denScanDirs, "heldByTransitionDenial")
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
