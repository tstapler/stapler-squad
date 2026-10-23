package services

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func transitionDeniedRows(t *testing.T, svc *BacklogService, itemID string) []*sessionv1.StuckBacklogItem {
	t.Helper()
	resp, err := svc.ListStuckBacklogItems(context.Background(), connect.NewRequest(&sessionv1.ListStuckBacklogItemsRequest{}))
	require.NoError(t, err)
	var rows []*sessionv1.StuckBacklogItem
	for _, it := range resp.Msg.Items {
		if it.ItemId == itemID && it.Reason == sessionv1.StuckReason_STUCK_REASON_TRANSITION_DENIED {
			rows = append(rows, it)
		}
	}
	return rows
}

func countDeniedNotes(t *testing.T, svc *BacklogService, itemID, want string) int {
	t.Helper()
	notes, err := svc.storage.ListProgressNotesForItem(context.Background(), itemID)
	require.NoError(t, err)
	n := 0
	for _, note := range notes {
		if note.Note == want {
			n++
		}
	}
	return n
}

func countDeniedNotifications(ch <-chan *events.Event) int {
	n := 0
	deadline := time.After(300 * time.Millisecond)
	for {
		select {
		case ev := <-ch:
			if ev != nil && ev.Type == events.EventNotification {
				n++
			}
		case <-deadline:
			return n
		}
	}
}

func TestDequeueClaim_should_SurfaceTransitionDeniedOnceAndRecover_When_EngineDeniesThenAllows(t *testing.T) {
	// A graph denial cannot reach the claim: queued->in_progress is a
	// system-required edge, which a stage-config edit cannot disable.
	cases := []struct {
		name       string
		deny       func(*systemGateHarness, *testing.T) (undo func())
		wantReason string
	}{
		{"operator_gate", func(h *systemGateHarness, t *testing.T) func() {
			return h.blockWithGate(t, session.BacklogStatusQueued, session.BacklogStatusInProgress)
		}, "operator_gate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureClaimLogs(t)
			svc, creator, repoPath := newDequeueClaimService(t, &fakeClaimChecker{verdict: NewUnclaimedVerdict()})
			svc.claimDedupFlag = func() bool { return false }
			itemID := createQueuedItemWithURL(t, svc, repoPath, claimIssueURL)

			h := wireEnforcedGate(t, svc)
			undo := tc.deny(h, t)
			bus := events.NewEventBus(16)
			svc.SetEventBus(bus)
			ch, _ := bus.Subscribe(t.Context())
			ctx := context.Background()

			require.NoError(t, svc.DequeueNextQueuedItems(ctx))

			assert.Equal(t, "queued", itemStatus(t, svc, itemID))
			rows := transitionDeniedRows(t, svc, itemID)
			require.Len(t, rows, 1)
			assert.Equal(t, "claim_and_spawn", rows[0].DeniedCaller)
			assert.Equal(t, "queued", rows[0].DeniedFrom)
			assert.Equal(t, "in_progress", rows[0].DeniedTo)
			assert.Equal(t, tc.wantReason, rows[0].DeniedReason)
			note := fmt.Sprintf("automatic claim_and_spawn denied: from=queued to=in_progress reason=%s", tc.wantReason)
			assert.Equal(t, 1, countDeniedNotes(t, svc, itemID, note))
			assert.Equal(t, 1, countDeniedNotifications(ch))
			assert.NotContains(t, logs.String(), "claim failed")

			require.NoError(t, svc.DequeueNextQueuedItems(ctx))
			assert.Equal(t, 1, countDeniedNotes(t, svc, itemID, note), "second sweep must not add a comment")
			assert.Equal(t, 0, countDeniedNotifications(ch), "second sweep must not notify")
			assert.Len(t, transitionDeniedRows(t, svc, itemID), 1)
			assert.NotContains(t, logs.String(), "claim failed")

			undo()
			require.NoError(t, svc.DequeueNextQueuedItems(ctx))
			assert.Equal(t, "in_progress", itemStatus(t, svc, itemID))
			assert.Empty(t, transitionDeniedRows(t, svc, itemID), "row must clear once the claim succeeds")
			assert.NotEmpty(t, creator.calls)
		})
	}
}

func TestDequeueClaim_should_KeepExistingArmsAndRaiseNoDenial_When_BlockerUnresolved(t *testing.T) {
	svc, _, _, dependentID := blockedDequeueFixture(t)

	require.NoError(t, svc.DequeueNextQueuedItems(context.Background()))

	assert.Len(t, blockedByDependencyRows(t, svc, dependentID), 1)
	assert.Empty(t, transitionDeniedRows(t, svc, dependentID))
}

func TestTransitionDeniedContext_should_RoundTrip(t *testing.T) {
	d := domain.TransitionDenial{Caller: "dequeue_claim", From: "queued", To: "in_progress", Reason: "config_graph"}
	got, ok := domain.ParseTransitionDeniedContext(domain.FormatTransitionDeniedContext(d, "x=y; z"))
	require.True(t, ok)
	assert.Equal(t, d, got)
	_, ok = domain.ParseTransitionDeniedContext("something else")
	assert.False(t, ok)
	assert.Equal(t, sessionv1.StuckReason_STUCK_REASON_TRANSITION_DENIED, toProtoStuckReason(domain.StuckReasonTransitionDenied))
	assert.Equal(t, domain.StuckReasonTransitionDenied, fromProtoStuckReason(sessionv1.StuckReason_STUCK_REASON_TRANSITION_DENIED))
}

// --- Story 1.5.8: spawn points park behind an open denial --------------------------

const denHeldLog = "item waits on a denied transition, skipping"

type denSpawnCase struct {
	name   string
	status session.BacklogStatus
	to     session.BacklogStatus // the denied edge out of status
	caller session.TransitionCaller
	setup  func(t *testing.T) (*BacklogService, string)
	call   func(svc *BacklogService, itemID string)
}

func denDefaultSetup(status session.BacklogStatus) func(t *testing.T) (*BacklogService, string) {
	return func(t *testing.T) (*BacklogService, string) {
		svc := NewBacklogService(createTestStorage(t), &mockSessionCreator{}, nil, nil, nil, nil)
		svc.SetHeadlessPool(&fakeHeadlessPool{})
		return svc, systemItemAt(t, svc, status).ID
	}
}

func denSpawnCases() []denSpawnCase {
	ctx := context.Background()
	return []denSpawnCase{
		{"AutoRespawnTriage", session.BacklogStatusIdea, session.BacklogStatusReady, session.CallerAutonomousComplete,
			denDefaultSetup(session.BacklogStatusIdea), func(s *BacklogService, id string) { _ = s.AutoRespawnTriage(ctx, id) }},
		{"MaybeTriggerTriage", session.BacklogStatusIdea, session.BacklogStatusReady, session.CallerMaybeTriggerTriage,
			func(t *testing.T) (*BacklogService, string) {
				svc := NewBacklogService(createTestStorage(t), &mockSessionCreator{}, nil, nil, nil, nil)
				svc.SetHeadlessPool(&fakeHeadlessPool{})
				item, err := svc.storage.CreateBacklogItem(ctx, session.BacklogItemData{
					Title: "has a plan", Status: string(session.BacklogStatusIdea), RepoPath: t.TempDir(), PlanApproved: true,
				})
				require.NoError(t, err)
				return svc, item.ID
			},
			func(s *BacklogService, id string) {
				item, _ := s.storage.GetBacklogItem(ctx, id)
				s.MaybeTriggerTriage(ctx, id, false, item.RepoPath)
			}},
		{"DequeueNextQueuedItems", session.BacklogStatusQueued, session.BacklogStatusInProgress, session.CallerClaimAndSpawn,
			func(t *testing.T) (*BacklogService, string) {
				svc, _, repoPath := newDequeueClaimService(t, &fakeClaimChecker{verdict: NewUnclaimedVerdict()})
				svc.claimDedupFlag = func() bool { return false }
				return svc, createQueuedItemWithURL(t, svc, repoPath, claimIssueURL)
			},
			func(s *BacklogService, _ string) { _ = s.DequeueNextQueuedItems(ctx) }},
		{"AutoReopenAfterFailedReview", session.BacklogStatusReview, session.BacklogStatusPRPending, session.CallerOrphanedAgentPR,
			denDefaultSetup(session.BacklogStatusReview), func(s *BacklogService, id string) { _ = s.AutoReopenAfterFailedReview(ctx, id) }},
		{"AutoRespawnAutonomousWork", session.BacklogStatusInProgress, session.BacklogStatusReview, session.CallerReconcileStuck,
			denDefaultSetup(session.BacklogStatusInProgress), func(s *BacklogService, id string) { _ = s.AutoRespawnAutonomousWork(ctx, id) }},
		{"RemediateStaleWorkSession", session.BacklogStatusInProgress, session.BacklogStatusReview, session.CallerReconcileStuck,
			denDefaultSetup(session.BacklogStatusInProgress), func(s *BacklogService, id string) { _ = s.RemediateStaleWorkSession(ctx, id) }},
		{"AutoReopenForPRFix", session.BacklogStatusPRPending, session.BacklogStatusDone, session.CallerReconcilePRPendingMerge,
			denDefaultSetup(session.BacklogStatusPRPending), func(s *BacklogService, id string) { _ = s.AutoReopenForPRFix(ctx, id, "ci failed") }},
		{"AutoRespawnReview", session.BacklogStatusReview, session.BacklogStatusPRPending, session.CallerOrphanedAgentPR,
			denDefaultSetup(session.BacklogStatusReview), func(s *BacklogService, id string) { _ = s.AutoRespawnReview(ctx, id) }},
	}
}

// denOpenRowFrom opens a transition_denied row anchored at from. The item must
// be at from when called.
func denOpenRowFrom(t *testing.T, svc *BacklogService, itemID string, caller session.TransitionCaller, from, to session.BacklogStatus) {
	t.Helper()
	d := domain.TransitionDenial{Caller: string(caller), From: string(from), To: string(to), Reason: session.DeniedOperatorGate}
	applied, err := svc.storage.MarkStuck(context.Background(), itemID, domain.StuckReasonTransitionDenied, from, domain.FormatTransitionDeniedContext(d, "test"))
	require.NoError(t, err)
	require.True(t, applied)
}

// TestSpawnPoints_should_SkipOnAnOpenDenialFromTheirStatusOnly runs each spawn
// point with an open denial, with none (today's behavior) and with a stale one
// (its from differs from the item's status).
func TestSpawnPoints_should_SkipOnAnOpenDenialFromTheirStatusOnly(t *testing.T) {
	for _, tc := range denSpawnCases() {
		for _, mode := range []string{"open denial", "no row", "stale row"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				svc, itemID := tc.setup(t)
				h := wireEnforcedGate(t, svc)
				h.blockWithGate(t, tc.status, tc.to)
				switch mode {
				case "open denial":
					denOpenRowFrom(t, svc, itemID, tc.caller, tc.status, tc.to)
				case "stale row":
					_, err := svc.storage.RawTransitionBacklogItemStatus(t.Context(), itemID, session.BacklogStatusRefining, nil, session.TriggeredBySystem)
					require.NoError(t, err)
					denOpenRowFrom(t, svc, itemID, tc.caller, session.BacklogStatusRefining, tc.to)
					_, err = svc.storage.RawTransitionBacklogItemStatus(t.Context(), itemID, tc.status, nil, session.TriggeredBySystem)
					require.NoError(t, err)
				}
				logs := captureLogs(t)

				tc.call(svc, itemID)

				if mode == "open denial" {
					assert.Contains(t, logs.String(), denHeldLog)
					assert.Equal(t, string(tc.status), itemStatus(t, svc, itemID), "a held item is not moved")
				} else {
					assert.NotContains(t, logs.String(), denHeldLog)
				}
			})
		}
	}
}

func TestSpawnPoints_should_ProceedAndClearTheRow_When_TheDenialLifted(t *testing.T) {
	svc, itemID := denDefaultSetup(session.BacklogStatusInProgress)(t)
	h := wireEnforcedGate(t, svc)
	remove := h.blockWithGate(t, session.BacklogStatusInProgress, session.BacklogStatusReview)
	denOpenRowFrom(t, svc, itemID, session.CallerReconcileStuck, session.BacklogStatusInProgress, session.BacklogStatusReview)
	remove()
	logs := captureLogs(t)

	_ = svc.AutoRespawnAutonomousWork(context.Background(), itemID) // proceeds past the guard, then fails on the fixture's missing repo_path

	assert.NotContains(t, logs.String(), denHeldLog)
	assert.Empty(t, transitionDeniedRows(t, svc, itemID), "the lifted denial is resolved by the first spawn point to see it")
}

func TestTriggerRemediationNow_should_ReturnFailedPreconditionAndSpendNoAttempt_When_TheItemIsHeldByADenial(t *testing.T) {
	svc, itemID := denDefaultSetup(session.BacklogStatusIdea)(t)
	h := wireEnforcedGate(t, svc)
	h.blockWithGate(t, session.BacklogStatusIdea, session.BacklogStatusReady)
	_, err := svc.storage.MarkStuck(context.Background(), itemID, domain.StuckReasonOrphanedTriage, session.BacklogStatusIdea, "orphan")
	require.NoError(t, err)
	denOpenRowFrom(t, svc, itemID, session.CallerAutonomousComplete, session.BacklogStatusIdea, session.BacklogStatusReady)

	_, err = svc.TriggerRemediationNow(context.Background(), connect.NewRequest(&sessionv1.TriggerRemediationNowRequest{
		ItemId: itemID, Reason: sessionv1.StuckReason_STUCK_REASON_ORPHANED_TRIAGE,
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.ErrorIs(t, err, session.ErrTransitionDeniedHold)
	assert.Contains(t, err.Error(), "autonomous_complete")
	rows, findErr := svc.storage.FindOpenStuckStates(context.Background())
	require.NoError(t, findErr)
	for _, row := range rows {
		if row.Reason == domain.StuckReasonOrphanedTriage {
			assert.Zero(t, row.RemediationAttempts, "a held item spends no attempt")
		}
	}
}

// --- completeness: remediation reasons and the shared guard ----------------------

// denRemediationReasons is every reason remediationActionByReason maps, with the
// spawn point it reaches; a new mapping fails the test until it is classified.
var denRemediationReasons = map[string]string{
	"StuckReasonBouncing":        "AutoReopenAfterFailedReview",
	"StuckReasonAbandonedReview": "AutoRespawnReview",
	"StuckReasonAutonomousStuck": "AutoRespawnAutonomousWork",
	"StuckReasonStaleWork":       "RemediateStaleWorkSession",
	"StuckReasonOrphanedTriage":  "AutoRespawnTriage",
	"StuckReasonPRPendingNoPR":   "AutoReopenForPRFix",
	"StuckReasonPRNeedsFix":      "AutoReopenForPRFix",
}

func denParse(t *testing.T, file string) *ast.File {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	require.NoError(t, err)
	return parsed
}

func TestEveryRemediationReasonIsClassified(t *testing.T) {
	var got []string
	for _, decl := range denParse(t, "backlog_service_stuck.go").Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "remediationActionByReason" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if cc, ok := n.(*ast.CaseClause); ok {
				for _, e := range cc.List {
					if sel, ok := e.(*ast.SelectorExpr); ok {
						got = append(got, sel.Sel.Name)
					}
				}
			}
			return true
		})
	}
	want := make([]string, 0, len(denRemediationReasons))
	for reason := range denRemediationReasons {
		want = append(want, reason)
	}
	sort.Strings(got)
	sort.Strings(want)
	assert.Equal(t, want, got)
}

// denFuncsCalling lists the BacklogService methods (and plain functions) in this
// package's non-test files whose body calls a method named one of names.
func denFuncsCalling(t *testing.T, names ...string) map[string]bool {
	t.Helper()
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	found := map[string]bool{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		for _, decl := range denParse(t, f).Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok && want[sel.Sel.Name] {
						found[fn.Name.Name+"->"+sel.Sel.Name] = true
					}
				}
				return true
			})
		}
	}
	return found
}

func TestSpawnPointGuards_should_ShareOnePredicateAndCoverEveryRemediationAction(t *testing.T) {
	predicate := denFuncsCalling(t, "HasOpenTransitionDenied", "OpenTransitionDenial", "TransitionDenialHolds")
	assert.Equal(t, map[string]bool{
		"heldByTransitionDenial->TransitionDenialHolds":    true,
		"transitionDenialHoldError->OpenTransitionDenial":  true,
		"transitionDenialHoldError->TransitionDenialHolds": true,
	}, predicate, "the predicate is reached only through the two guard helpers")

	guarded := denFuncsCalling(t, "heldByTransitionDenial", "triageHeldByDenial", "transitionDenialHoldError")
	spawnPoints := map[string]string{ // function -> the helper it must call
		"AutoReopenAfterFailedReview": "heldByTransitionDenial",
		"AutoRespawnReview":           "heldByTransitionDenial",
		"AutoRespawnAutonomousWork":   "heldByTransitionDenial",
		"RemediateStaleWorkSession":   "heldByTransitionDenial",
		"respawnTriage":               "heldByTransitionDenial",
		"autoReopenForPRFix":          "heldByTransitionDenial",
		"DequeueNextQueuedItems":      "heldByTransitionDenial",
		"triageHeldByDenial":          "heldByTransitionDenial",
		"MaybeTriggerTriage":          "triageHeldByDenial",
		"TriggerRemediationNow":       "transitionDenialHoldError",
	}
	for fn, helper := range spawnPoints {
		assert.Truef(t, guarded[fn+"->"+helper], "%s must call %s", fn, helper)
	}
}

// --- end to end: gate + real reporter + sweep --------------------------------------

// denTriageSpy counts the respawns the lifecycle listener asks for and
// delegates to the real service, so the spawn-point guard is exercised too.
type denTriageSpy struct {
	*BacklogService
	respawns int
}

func (s *denTriageSpy) AutoRespawnTriage(ctx context.Context, itemID string) error {
	s.respawns++
	return s.BacklogService.AutoRespawnTriage(ctx, itemID)
}

func denTriageSessionCount(t *testing.T, svc *BacklogService, itemID string) int {
	t.Helper()
	sessions, err := svc.storage.ListItemSessions(context.Background(), itemID)
	require.NoError(t, err)
	n := 0
	for _, s := range sessions {
		if s.Role == string(session.SessionRoleTriage) {
			n++
		}
	}
	return n
}

func TestAutonomousTriageDenied_should_StayQuietAndFinishByItself_When_TheOperatorApproves(t *testing.T) {
	f := newCharFixture(t, "denied-redrive", session.SessionRoleTriage, session.BacklogStatusIdea)
	svc := NewBacklogService(f.storage, &mockSessionCreator{}, nil, nil, nil, nil)
	svc.SetHeadlessPool(&fakeHeadlessPool{})
	bus := events.NewEventBus(64)
	svc.SetEventBus(bus)
	notifications, _ := bus.Subscribe(t.Context())
	h := wireEnforcedGate(t, svc)
	remove := h.blockWithGate(t, session.BacklogStatusIdea, session.BacklogStatusReady)
	spy := &denTriageSpy{BacklogService: svc}
	listener := session.NewBacklogLifecycleListener(f.storage)
	listener.SetEnabled(true)
	listener.SetTriageRespawner(spy)
	ctx := context.Background()

	f.complete(session.AutonomousDriverOutcome{Done: true, Reason: "done", Turns: 3})
	triageSessions := denTriageSessionCount(t, svc, f.itemID)
	for range 5 {
		listener.ReconcileStuck(ctx)
	}

	assert.Equal(t, "idea", itemStatus(t, svc, f.itemID))
	assert.Zero(t, spy.respawns, "no triage respawn while the gate holds the item")
	assert.Equal(t, triageSessions, denTriageSessionCount(t, svc, f.itemID), "no new triage session")
	assert.Len(t, transitionDeniedRows(t, svc, f.itemID), 1)
	note := "automatic autonomous_complete denied: from=idea to=ready reason=operator_gate"
	assert.Equal(t, 1, countDeniedNotes(t, svc, f.itemID, note), "exactly one comment however many ticks")
	assert.Equal(t, 1, countDeniedNotifications(notifications), "exactly one denial notification (the status-update-failed one goes out on the session service's own bus)")
	open, err := f.storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	for _, row := range open {
		assert.NotEqual(t, domain.StuckReasonOrphanedTriage, row.Reason, "orphaned_triage is never marked for a gate-held item")
	}

	remove()
	listener.ReconcileStuck(ctx)

	assert.Equal(t, "ready", itemStatus(t, svc, f.itemID), "the sweep moved the item through the gate")
	assert.Empty(t, transitionDeniedRows(t, svc, f.itemID))
	assert.Zero(t, spy.respawns)
	assert.Equal(t, triageSessions, denTriageSessionCount(t, svc, f.itemID))
}

func TestReconcileStuckDenied_should_NotRespawnWork_And_MoveToReviewOnceApproved(t *testing.T) {
	creator := &mockSessionCreator{}
	svc := NewBacklogService(createTestStorage(t), creator, nil, nil, nil, nil)
	h := wireEnforcedGate(t, svc)
	remove := h.blockWithGate(t, session.BacklogStatusInProgress, session.BacklogStatusReview)
	item := systemItemAt(t, svc, session.BacklogStatusInProgress)
	ctx := context.Background()
	workIS, err := svc.storage.CreateItemSession(ctx, session.ItemSessionData{ItemID: item.ID, SessionUUID: "ended-work", SessionRole: session.SessionRoleWork})
	require.NoError(t, err)
	require.NoError(t, svc.storage.UpdateItemSessionEnded(ctx, workIS.ID, time.Now()))
	listener := session.NewBacklogLifecycleListener(svc.storage)
	listener.SetEnabled(true)

	for range 3 {
		moved, err := svc.storage.GatedReconcileStuck(ctx)
		require.NoError(t, err)
		require.Zero(t, moved)
	}
	require.Equal(t, []string{string(session.CallerReconcileStuck)}, deniedCallers(t, svc, item.ID), "one row for three denied ticks")

	require.NoError(t, svc.AutoRespawnAutonomousWork(ctx, item.ID))
	require.NoError(t, svc.RemediateStaleWorkSession(ctx, item.ID))
	require.NoError(t, svc.AutoReopenAfterFailedReview(ctx, item.ID))
	assert.Zero(t, creator.callCount(), "stale_work, autonomous_stuck and bouncing spawn nothing while the gate is the only blocker")
	assert.Equal(t, "in_progress", itemStatus(t, svc, item.ID))

	remove()
	listener.ReconcileStuck(ctx)

	assert.Equal(t, "review", itemStatus(t, svc, item.ID))
	assert.Empty(t, transitionDeniedRows(t, svc, item.ID))
}

// TestWriterCallersDenied_should_ReportOnceAcrossManyTicks covers the rows where
// the detector is itself the status writer: PR merge, archive, drift recovery
// and reconcile-stuck.
func TestWriterCallersDenied_should_ReportOnceAcrossManyTicks(t *testing.T) {
	cases := []struct {
		caller   session.TransitionCaller
		from, to session.BacklogStatus
	}{
		{session.CallerReconcilePRPendingMerge, session.BacklogStatusPRPending, session.BacklogStatusDone},
		{session.CallerArchiveStaleDone, session.BacklogStatusDone, session.BacklogStatusArchived},
		{session.CallerRecoverDriftedPR, session.BacklogStatusReview, session.BacklogStatusPRPending},
		{session.CallerReconcileStuck, session.BacklogStatusInProgress, session.BacklogStatusReview},
	}
	for _, tc := range cases {
		t.Run(string(tc.caller), func(t *testing.T) {
			svc := NewBacklogService(createTestStorage(t), &mockSessionCreator{}, nil, nil, nil, nil)
			bus := events.NewEventBus(16)
			svc.SetEventBus(bus)
			ch, _ := bus.Subscribe(t.Context())
			h := wireEnforcedGate(t, svc)
			h.blockWithGate(t, tc.from, tc.to)
			item := systemItemAt(t, svc, tc.from)

			for range 5 {
				_, err := svc.storage.GatedTransition(context.Background(), session.TransitionRequest{
					Caller: tc.caller, ItemID: item.ID, To: tc.to, TriggeredBy: session.TriggeredBySystem,
				})
				require.ErrorIs(t, err, session.ErrGateNotSatisfied)
			}

			assert.Len(t, transitionDeniedRows(t, svc, item.ID), 1)
			note := fmt.Sprintf("automatic %s denied: from=%s to=%s reason=operator_gate", tc.caller, tc.from, tc.to)
			assert.Equal(t, 1, countDeniedNotes(t, svc, item.ID, note))
			assert.Equal(t, 1, countDeniedNotifications(ch))
		})
	}
}
