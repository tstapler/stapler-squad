package services

import (
	"context"
	"fmt"
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
