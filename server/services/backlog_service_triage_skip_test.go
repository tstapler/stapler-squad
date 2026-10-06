package services

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/workflows"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

func acJSON(t *testing.T, texts ...string) session.AcCriteriaJSON {
	t.Helper()
	cs := make([]session.AcCriterion, 0, len(texts))
	for i, tx := range texts {
		cs = append(cs, session.AcCriterion{Index: i, Text: tx, Status: "pending"})
	}
	j, err := session.SerializeAcCriteria(cs)
	require.NoError(t, err)
	return j
}

func activityMessages(t *testing.T, storage *session.Storage, itemID string) []string {
	t.Helper()
	notes, err := storage.ListActivityNotesForItem(t.Context(), itemID)
	require.NoError(t, err)
	out := make([]string, 0, len(notes))
	for _, n := range notes {
		out = append(out, n.Message)
	}
	return out
}

func TestResolveTriageModel(t *testing.T) {
	t.Parallel()
	fam := workflows.DefaultModelFamilies()
	cases := []struct {
		name            string
		cfg             *config.Config
		program, pinned string
		want            string
	}{
		{"default applies when unpinned", nil, "", "", fam["sonnet"]},
		{"pipeline pin wins", nil, "", "family:opus", fam["opus"]},
		{"configured family", &config.Config{HeadlessTriageModel: "family:haiku"}, "claude", "", fam["haiku"]},
		{"concrete configured id passes through", &config.Config{HeadlessTriageModel: "claude-x"}, "", "", "claude-x"},
		{"none opts out", &config.Config{HeadlessTriageModel: "none"}, "", "", ""},
		{"non-claude program gets no claude model", nil, "gemini", "", ""},
		{"bad configured family falls back to built-in default, not account default", &config.Config{HeadlessTriageModel: "family:nope"}, "", "", fam["sonnet"]},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, resolveTriageModel(tc.cfg, fam, tc.program, tc.pinned))
		})
	}
}

func TestTriageInputHash_IgnoresACStatusButNotText(t *testing.T) {
	t.Parallel()
	a := triageInputHash("t", "d", []session.AcCriterion{{Index: 0, Text: "x", Status: "pending"}})
	b := triageInputHash("t", "d", []session.AcCriterion{{Index: 3, Text: "x", Status: "done", Note: "n"}})
	assert.Equal(t, a, b)
	assert.NotEqual(t, a, triageInputHash("t", "d", []session.AcCriterion{{Text: "y"}}))
	assert.NotEqual(t, a, triageInputHash("t2", "d", []session.AcCriterion{{Text: "x"}}))
}

func TestMaybeTriggerTriage_SkipsItemWithACAndRecordsReason(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title: "has ac", Status: string(session.BacklogStatusIdea), Priority: 3, RepoPath: t.TempDir(),
		AcceptanceCriteria: acJSON(t, "do it"),
	})
	require.NoError(t, err)

	assert.False(t, svc.MaybeTriggerTriage(t.Context(), item.ID, false, item.RepoPath))
	assert.Empty(t, pool.calls)
	msgs := activityMessages(t, storage, item.ID)
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "Triage skipped: item already has acceptance criteria")
}

func TestMaybeTriggerTriage_SkipsApprovedPlan_RunsPlainItem(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)
	approved, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title: "approved", Status: string(session.BacklogStatusIdea), Priority: 3, RepoPath: t.TempDir(), PlanApproved: true,
	})
	require.NoError(t, err)
	assert.False(t, svc.MaybeTriggerTriage(t.Context(), approved.ID, false, approved.RepoPath))
	assert.Empty(t, pool.calls)

	plain, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title: "plain", Status: string(session.BacklogStatusIdea), Priority: 3, RepoPath: t.TempDir(),
	})
	require.NoError(t, err)
	assert.True(t, svc.MaybeTriggerTriage(t.Context(), plain.ID, false, plain.RepoPath))
	wait.RequireEventually(t, func() bool { return pool.callCount() == 1 }, 5*time.Second, 20*time.Millisecond, "plain item still triaged")
}

// A completed triage stamps InputHash; automatic retriage of unchanged content is then
// skipped (not demoted), a changed description runs again, and ResumeTriage bypasses the gate.
func TestAutoRespawnTriage_UnchangedContentSkipped_ChangedRuns_ResumeBypasses(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title: "retriage", Description: "v1", Status: string(session.BacklogStatusIdea), Priority: 3, RepoPath: t.TempDir(),
	})
	require.NoError(t, err)

	require.NoError(t, svc.AutoRespawnTriage(t.Context(), item.ID))
	wait.RequireEventually(t, func() bool {
		got, e := storage.GetBacklogItem(t.Context(), item.ID)
		return e == nil && got.Status == string(session.BacklogStatusReady) && !svc.IsTriageLive(item.ID)
	}, 5*time.Second, 20*time.Millisecond, "first triage completes")
	require.Equal(t, 1, pool.callCount())

	// Back to queued (e.g. plan never approved): unchanged content -> skipped, status preserved.
	_, err = storage.TransitionBacklogItemStatus(t.Context(), item.ID, session.BacklogStatusQueued, nil, session.TriggeredBySystem)
	require.NoError(t, err)
	require.NoError(t, svc.AutoRespawnTriage(t.Context(), item.ID))
	assert.Equal(t, 1, pool.callCount())
	got, err := storage.GetBacklogItem(t.Context(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusQueued), got.Status, "skipped item must not be demoted to idea")
	assert.Contains(t, activityMessages(t, storage, item.ID)[0], "unchanged since the last completed triage")

	// Guidance-answer resume bypasses the gate.
	require.NoError(t, svc.ResumeTriage(t.Context(), item.ID))
	wait.RequireEventually(t, func() bool { return pool.callCount() == 2 }, 5*time.Second, 20*time.Millisecond, "ResumeTriage must run despite unchanged content")
}

func TestTriageConcurrencyCapQueuesExcessRuns(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	var active, maxActive int32
	var mu sync.Mutex
	pool := &fakeHeadlessPool{response: validTriageJSON(), delay: 80 * time.Millisecond}
	pool.onEnter = func() {
		n := atomic.AddInt32(&active, 1)
		mu.Lock()
		if n > maxActive {
			maxActive = n
		}
		mu.Unlock()
		time.AfterFunc(70*time.Millisecond, func() { atomic.AddInt32(&active, -1) })
	}
	svc := NewBacklogService(storage, nil, &config.Config{MaxConcurrentTriage: 1}, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	for i := 0; i < 3; i++ {
		it, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
			Title: "bulk", Status: string(session.BacklogStatusIdea), Priority: 3, RepoPath: t.TempDir(),
		})
		require.NoError(t, err)
		require.True(t, svc.MaybeTriggerTriage(t.Context(), it.ID, false, it.RepoPath))
	}
	wait.RequireEventually(t, func() bool { return pool.callCount() == 3 }, 10*time.Second, 20*time.Millisecond, "queued runs all eventually execute")
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, int32(1), maxActive, "cap=1 must never run two triage calls at once")
}

func TestTriageCallUsesConfiguredModel(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	svc := NewBacklogService(storage, nil, &config.Config{HeadlessTriageModel: "family:haiku"}, nil, nil, nil)
	svc.SetHeadlessPool(pool)
	it, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title: "m", Status: string(session.BacklogStatusIdea), Priority: 3, RepoPath: t.TempDir(),
	})
	require.NoError(t, err)
	require.True(t, svc.MaybeTriggerTriage(t.Context(), it.ID, false, it.RepoPath))
	wait.RequireEventually(t, func() bool { return pool.callCount() == 1 }, 5*time.Second, 20*time.Millisecond, "call made")
	pool.mu.Lock()
	defer pool.mu.Unlock()
	assert.Equal(t, workflows.DefaultModelFamilies()["haiku"], pool.calls[0].model)
}
