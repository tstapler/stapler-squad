package backlogsvc

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/headless"
)

const (
	noAcTriageJSON  = `{"summary":"plan summary","suggestions":[],"tasks":[]}`
	derivedACJSON   = `{"acceptance_criteria":[{"index":0,"text":"go test ./foo passes","status":"pending"}]}`
	emptyDerivation = `{"acceptance_criteria":[]}`
)

func newNoAcItem(t *testing.T, storage interface {
	CreateBacklogItem(context.Context, backlogdto.BacklogItemData) (*backlogdto.BacklogItemData, error)
}) *backlogdto.BacklogItemData {
	t.Helper()
	item, err := storage.CreateBacklogItem(t.Context(), backlogdto.BacklogItemData{
		Title: "no ac item", Status: string(domain.BacklogStatusIdea), Priority: 3, RepoPath: t.TempDir(),
	})
	require.NoError(t, err)
	return item
}

func TestDeriveAcFromDescription_Success(t *testing.T) {
	t.Parallel()
	pool := &fakeHeadlessPool{response: derivedACJSON}
	item := &backlogdto.BacklogItemData{ID: "i1", Title: "t", Description: "make foo testable"}

	got, err := deriveAcFromDescription(t.Context(), pool, "", t.TempDir(), item, "summary")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "go test ./foo passes", got[0].Text)
	assert.Contains(t, pool.firstCall().userPrompt, "make foo testable")
	assert.Equal(t, headless.FeatureKeyTriage, pool.firstCall().key)
}

func TestDeriveAcFromDescription_EmptyResponse_ReturnsNoCriteria(t *testing.T) {
	t.Parallel()
	pool := &fakeHeadlessPool{response: emptyDerivation}
	got, err := deriveAcFromDescription(t.Context(), pool, "", t.TempDir(), &backlogdto.BacklogItemData{ID: "i1"}, "")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestTriggerTriage_GeneratesAC_Integration(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	pool := &fakeHeadlessPool{responses: []string{noAcTriageJSON, derivedACJSON}}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)
	bus := events.NewEventBus(64)
	svc.SetEventBus(bus)
	subCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)
	item := newNoAcItem(t, storage)

	_, err := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: item.ID}))
	require.NoError(t, err)

	var updated *backlogdto.BacklogItemData
	require.Eventually(t, func() bool {
		u, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		updated = u
		return loadErr == nil && u.Status == string(domain.BacklogStatusReady)
	}, 30*time.Second, 50*time.Millisecond)

	criteria, parseErr := domain.ParseAcCriteria(updated.AcceptanceCriteria)
	require.NoError(t, parseErr)
	require.Len(t, criteria, 1)
	assert.Equal(t, "go test ./foo passes", criteria[0].Text)
	assert.Equal(t, 2, pool.callCount(), "triage call plus one derivation call")
	select {
	case ev := <-ch:
		assert.NotEqual(t, "Triage needs acceptance criteria", ev.NotificationTitle)
	default:
	}
}

func TestTriggerTriage_EscalatesOnUnderivableAC(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	pool := &fakeHeadlessPool{responses: []string{noAcTriageJSON, emptyDerivation}}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)
	bus := events.NewEventBus(64)
	svc.SetEventBus(bus)
	subCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)
	item := newNoAcItem(t, storage)

	_, err := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: item.ID}))
	require.NoError(t, err)

	var notif *events.Event
	deadline := time.After(30 * time.Second)
	for notif == nil {
		select {
		case ev := <-ch:
			if ev.Type == events.EventNotification {
				notif = ev
			}
		case <-deadline:
			require.FailNow(t, "no owner escalation published")
		}
	}
	assert.Equal(t, "Triage needs acceptance criteria", notif.NotificationTitle)
	assert.Equal(t, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING), notif.NotificationType)

	updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
	require.NoError(t, loadErr)
	assert.Equal(t, string(domain.BacklogStatusIdea), updated.Status, "item must stay at idea")
	assert.Empty(t, updated.AcceptanceCriteria)
}

// TestNoACVerdictReasonAbsentInWindow: an item can only leave triage with criteria, so the
// reviewer never sees an empty list; the only place reason=no_acceptance_criteria is emitted
// is the triage escalation above, never a review verdict.
func TestNoACVerdictReasonAbsentInWindow(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	pool := &fakeHeadlessPool{responses: []string{noAcTriageJSON, derivedACJSON}}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)
	item := newNoAcItem(t, storage)
	_, err := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: item.ID}))
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		u, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && u.Status == string(domain.BacklogStatusReady)
	}, 30*time.Second, 50*time.Millisecond)

	u, _ := storage.GetBacklogItem(t.Context(), item.ID)
	assert.True(t, itemHasAcceptanceCriteria(u), "a ready item must carry criteria, so no verdict can cite %q", reasonNoAcceptanceCriteria)
}
