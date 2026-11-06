package services

import (
	"errors"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// Characterization (not specification) of TransitionBacklogItemStatus under
// the Default workflow engine, pinned before the engine-gated choke point
// (ADR-007) changes anything. Configured-engine gates are out of scope.
//
// Observed fact worth keeping: override_reason bypasses only the review/
// pr_pending -> ready verdict-clear guard and the ->done verdict/shipped-code
// guards (session/domain/backlog.go TransitionGuard). It does NOT bypass the
// AC, plan, plan-artifact or blocker guards, and never bypasses CanTransition.

type transitionCase struct {
	name string
	// seed creates the item (and any verdict/dependency) and returns its ID.
	seed     func(t *testing.T, svc *BacklogService) string
	to       session.BacklogStatus
	override string
	// wantCode is the connect code, or zero value with wantOK for success.
	wantOK       bool
	wantCode     connect.Code
	wantSentinel error
}

func seedItem(t *testing.T, svc *BacklogService, data session.BacklogItemData) string {
	t.Helper()
	item, err := svc.storage.CreateBacklogItem(t.Context(), data)
	require.NoError(t, err)
	return item.ID
}

func seedReviewItemWithVerdict(t *testing.T, svc *BacklogService, status session.BacklogStatus, outcome session.ReviewOutcome) string {
	t.Helper()
	id := seedItem(t, svc, session.BacklogItemData{Title: "characterization item", Status: string(status)})
	if outcome != "" {
		_, err := svc.storage.CreateItemSessionWithVerdict(t.Context(), session.ItemSessionData{
			ItemID:      id,
			SessionUUID: "char-review-session-" + id,
			SessionRole: session.SessionRoleReview,
		}, session.ReviewVerdictData{OverallOutcome: outcome})
		require.NoError(t, err)
	}
	return id
}

func TestTransitionBacklogItemStatus_CharacterizesOverrideAndErrorCodes_UnderDefaultEngine(t *testing.T) {
	t.Parallel()

	oneAC, err := session.SerializeAcCriteria([]session.AcCriterion{{Index: 0, Text: "works", Status: session.AcStatusPending}})
	require.NoError(t, err)

	cases := []transitionCase{
		{
			name: "review_to_done_without_verdict_and_no_override_is_FailedPrecondition_ErrVerdictRequired",
			seed: func(t *testing.T, svc *BacklogService) string {
				return seedReviewItemWithVerdict(t, svc, session.BacklogStatusReview, "")
			},
			to: session.BacklogStatusDone, wantCode: connect.CodeFailedPrecondition, wantSentinel: session.ErrVerdictRequired,
		},
		{
			name: "review_to_done_without_verdict_but_with_override_succeeds",
			seed: func(t *testing.T, svc *BacklogService) string {
				return seedReviewItemWithVerdict(t, svc, session.BacklogStatusReview, "")
			},
			to: session.BacklogStatusDone, override: "ship per owner", wantOK: true,
		},
		{
			name: "review_to_ready_with_PASS_verdict_and_no_override_is_FailedPrecondition_ErrVerdictClearRequiredForReady",
			seed: func(t *testing.T, svc *BacklogService) string {
				return seedReviewItemWithVerdict(t, svc, session.BacklogStatusReview, session.ReviewVerdictPass)
			},
			to: session.BacklogStatusReady, wantCode: connect.CodeFailedPrecondition, wantSentinel: session.ErrVerdictClearRequiredForReady,
		},
		{
			name: "review_to_ready_with_PASS_verdict_and_override_succeeds",
			seed: func(t *testing.T, svc *BacklogService) string {
				return seedReviewItemWithVerdict(t, svc, session.BacklogStatusReview, session.ReviewVerdictPass)
			},
			to: session.BacklogStatusReady, override: "re-spawn", wantOK: true,
		},
		{
			name: "idea_to_ready_without_AC_is_FailedPrecondition_ErrACRequired",
			seed: func(t *testing.T, svc *BacklogService) string {
				return seedItem(t, svc, session.BacklogItemData{Title: "no ac", Status: string(session.BacklogStatusIdea)})
			},
			to: session.BacklogStatusReady, wantCode: connect.CodeFailedPrecondition, wantSentinel: session.ErrACRequired,
		},
		{
			name: "idea_to_ready_without_AC_is_NOT_bypassed_by_override",
			seed: func(t *testing.T, svc *BacklogService) string {
				return seedItem(t, svc, session.BacklogItemData{Title: "no ac", Status: string(session.BacklogStatusIdea)})
			},
			to: session.BacklogStatusReady, override: "please", wantCode: connect.CodeFailedPrecondition, wantSentinel: session.ErrACRequired,
		},
		{
			name: "idea_to_ready_with_AC_succeeds",
			seed: func(t *testing.T, svc *BacklogService) string {
				return seedItem(t, svc, session.BacklogItemData{Title: "has ac", Status: string(session.BacklogStatusIdea), AcceptanceCriteria: oneAC})
			},
			to: session.BacklogStatusReady, wantOK: true,
		},
		{
			name: "ready_to_in_progress_without_plan_is_FailedPrecondition_ErrPlanRequired",
			seed: func(t *testing.T, svc *BacklogService) string {
				return seedItem(t, svc, session.BacklogItemData{Title: "no plan", Status: string(session.BacklogStatusReady)})
			},
			to: session.BacklogStatusInProgress, wantCode: connect.CodeFailedPrecondition, wantSentinel: session.ErrPlanRequired,
		},
		{
			name: "ready_to_in_progress_without_plan_is_NOT_bypassed_by_override",
			seed: func(t *testing.T, svc *BacklogService) string {
				return seedItem(t, svc, session.BacklogItemData{Title: "no plan", Status: string(session.BacklogStatusReady)})
			},
			to: session.BacklogStatusInProgress, override: "please", wantCode: connect.CodeFailedPrecondition, wantSentinel: session.ErrPlanRequired,
		},
		{
			name: "ready_to_in_progress_with_approved_plan_but_no_artifacts_path_is_FailedPrecondition_ErrPlanArtifactsRequired",
			seed: func(t *testing.T, svc *BacklogService) string {
				id := seedItem(t, svc, session.BacklogItemData{Title: "plan no path", Status: string(session.BacklogStatusReady)})
				approved := true
				_, err := svc.storage.UpdateBacklogItem(t.Context(), id, session.BacklogItemUpdate{PlanApproved: &approved}, nil)
				require.NoError(t, err)
				return id
			},
			to: session.BacklogStatusInProgress, wantCode: connect.CodeFailedPrecondition, wantSentinel: session.ErrPlanArtifactsRequired,
		},
		{
			name: "ready_to_in_progress_with_artifacts_path_missing_on_disk_is_FailedPrecondition_ErrPlanArtifactsMissing",
			seed: func(t *testing.T, svc *BacklogService) string {
				id := seedItem(t, svc, session.BacklogItemData{Title: "plan gone", Status: string(session.BacklogStatusReady)})
				approved := true
				gone := filepath.Join(t.TempDir(), "deleted-plan-dir")
				_, err := svc.storage.UpdateBacklogItem(t.Context(), id, session.BacklogItemUpdate{PlanApproved: &approved, PlanArtifactsPath: &gone}, nil)
				require.NoError(t, err)
				return id
			},
			to: session.BacklogStatusInProgress, wantCode: connect.CodeFailedPrecondition, wantSentinel: session.ErrPlanArtifactsMissing,
		},
		{
			name: "ready_to_in_progress_with_unresolved_blocker_is_FailedPrecondition_ErrUnresolvedBlockers",
			seed: func(t *testing.T, svc *BacklogService) string {
				blocker := seedItem(t, svc, session.BacklogItemData{Title: "blocker", Status: string(session.BacklogStatusIdea)})
				id := seedItem(t, svc, session.BacklogItemData{Title: "blocked", Status: string(session.BacklogStatusReady), SkipPlanning: true})
				require.NoError(t, svc.storage.AddBacklogItemDependency(t.Context(), session.BacklogItemDependencyEdge{BlockerID: blocker, BlockedID: id}))
				return id
			},
			to: session.BacklogStatusInProgress, override: "please", wantCode: connect.CodeFailedPrecondition, wantSentinel: session.ErrUnresolvedBlockers,
		},
		{
			name: "idea_to_done_is_InvalidArgument_without_override",
			seed: func(t *testing.T, svc *BacklogService) string {
				return seedItem(t, svc, session.BacklogItemData{Title: "skip ahead", Status: string(session.BacklogStatusIdea)})
			},
			to: session.BacklogStatusDone, wantCode: connect.CodeInvalidArgument,
		},
		{
			name: "idea_to_done_is_InvalidArgument_even_with_override_because_CanTransition_runs_first",
			seed: func(t *testing.T, svc *BacklogService) string {
				return seedItem(t, svc, session.BacklogItemData{Title: "skip ahead", Status: string(session.BacklogStatusIdea)})
			},
			to: session.BacklogStatusDone, override: "ship per owner", wantCode: connect.CodeInvalidArgument,
		},
		{
			name: "unknown_item_is_NotFound",
			seed: func(t *testing.T, svc *BacklogService) string { return "does-not-exist" },
			to:   session.BacklogStatusDone, wantCode: connect.CodeNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newBacklogService(t)
			bus := events.NewEventBus(100)
			svc.SetEventBus(bus)
			ch, _ := bus.Subscribe(t.Context())

			id := tc.seed(t, svc)
			var fromStatus string
			if item, getErr := svc.storage.GetBacklogItem(t.Context(), id); getErr == nil {
				fromStatus = item.Status
			}

			_, err := svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
				ItemId:         id,
				TargetStatus:   string(tc.to),
				OverrideReason: tc.override,
			}))

			if !tc.wantOK {
				require.Error(t, err)
				assert.Equal(t, tc.wantCode, connect.CodeOf(err))
				if tc.wantSentinel != nil {
					assert.True(t, errors.Is(err, tc.wantSentinel), "want %v, got %v", tc.wantSentinel, err)
				}
				if fromStatus != "" {
					after, getErr := svc.storage.GetBacklogItem(t.Context(), id)
					require.NoError(t, getErr)
					assert.Equal(t, fromStatus, after.Status, "a rejected transition must not change status")
				}
				assertNoManualOverrideRecorded(t, svc, id, ch)
				return
			}

			require.NoError(t, err)
			after, getErr := svc.storage.GetBacklogItem(t.Context(), id)
			require.NoError(t, getErr)
			assert.Equal(t, string(tc.to), after.Status)
			if tc.override == "" {
				assertNoManualOverrideRecorded(t, svc, id, ch)
				return
			}
			notes, notesErr := svc.storage.ListProgressNotesForItem(t.Context(), id)
			require.NoError(t, notesErr)
			require.Len(t, notes, 1, "an override records exactly one progress note")
			assert.Contains(t, notes[0].Note, "Manually overridden by operator")
			assert.Contains(t, notes[0].Note, tc.override)
			select {
			case ev := <-ch:
				require.NotNil(t, ev)
			default:
				t.Fatal("notifyManualOverride must publish a notification on the override success path")
			}
		})
	}
}

func assertNoManualOverrideRecorded(t *testing.T, svc *BacklogService, id string, ch <-chan *events.Event) {
	t.Helper()
	if notes, err := svc.storage.ListProgressNotesForItem(t.Context(), id); err == nil {
		assert.Empty(t, notes, "no override progress note without a successful override")
	}
	select {
	case ev := <-ch:
		t.Fatalf("unexpected notification published: %+v", ev)
	default:
	}
}

func TestTransitionBacklogItemStatus_CharacterizesNilStorage_AsUnavailable(t *testing.T) {
	t.Parallel()
	_, err := newBacklogServiceNilStorage().TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId: "x", TargetStatus: string(session.BacklogStatusReady),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
}

// The sentinels that map to FailedPrecondition; every other guard error maps
// to InvalidArgument. ErrCodeNotOnMain needs a git fixture at the handler
// level and is pinned by TestTransitionBacklogItemStatus_should_BlockDone_*.
func TestIsFailedPreconditionGuardError_CharacterizesSentinelSet(t *testing.T) {
	t.Parallel()
	for _, sentinel := range []error{
		session.ErrACRequired, session.ErrPlanRequired, session.ErrPlanArtifactsRequired,
		session.ErrPlanArtifactsMissing, session.ErrVerdictRequired, session.ErrCodeNotOnMain,
		session.ErrVerdictClearRequiredForReady, session.ErrUnresolvedBlockers,
	} {
		assert.True(t, isFailedPreconditionGuardError(sentinel), "%v", sentinel)
	}
	assert.False(t, isFailedPreconditionGuardError(errors.New("some other guard failure")))
}
