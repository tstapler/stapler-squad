package services

// jules_usage_counter_test.go — Epic 4.1, Task 4.1.1b. Two tests:
//
//   - TestJulesFullCycle_should_LogNoSecretMaterial_When_DispatchPollCompleteCycleRunsAgainstFakes
//     runs a real dispatch -> poll(queued) -> poll(completed) cycle against a
//     real *session.Storage (createTestStorage, jules_dispatch_service_test.go)
//     and fakes for the Jules API, with a capturing slog.Handler installed as
//     the process default, then asserts every log line the Observability Plan
//     names for this path was emitted and none of them carry the test API key
//     or the auth header name.
//   - TestJulesUsageCounter_Snapshot_should_IncrementDispatchedAndAPIErrorExactlyOnce_When_OneDispatchAndOnePollFailure
//     is the narrower unit test: one successful dispatch, one failed poll,
//     assert the two counters that should move each land on exactly 1.
//
// Neither test is marked t.Parallel(): the first mutates the process-wide
// slog default for its duration (restored via t.Cleanup) and must not race
// another test's log calls.

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/jules"
	"github.com/tstapler/stapler-squad/session"
)

// capturingSlogHandler is a minimal slog.Handler that records every handled
// Record for later inspection, letting a test assert on exact message text
// and attribute content without depending on log.Initialize's async
// file/console pipeline (which writes JSON to disk, not to memory).
type capturingSlogHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingSlogHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *capturingSlogHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *capturingSlogHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *capturingSlogHandler) WithGroup(_ string) slog.Handler      { return h }

// snapshot returns a defensive copy of every record captured so far, safe to
// range over after concurrent Handle calls from the poller's goroutine.
func (h *capturingSlogHandler) snapshot() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]slog.Record, len(h.records))
	copy(out, h.records)
	return out
}

// messages returns every captured record's Message, in capture order.
func (h *capturingSlogHandler) messages() []string {
	records := h.snapshot()
	out := make([]string, len(records))
	for i, r := range records {
		out[i] = r.Message
	}
	return out
}

// containsText reports whether needle appears in any captured record's
// message, attribute key, or attribute value (recursively through groups) —
// the secret-absence check covers the whole record, not just Message, per
// validation.md's Story 4.1.1 test case.
func (h *capturingSlogHandler) containsText(needle string) bool {
	for _, r := range h.snapshot() {
		if strings.Contains(r.Message, needle) {
			return true
		}
		found := false
		r.Attrs(func(a slog.Attr) bool {
			if attrContainsText(a, needle) {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func attrContainsText(a slog.Attr, needle string) bool {
	if strings.Contains(a.Key, needle) {
		return true
	}
	if a.Value.Kind() == slog.KindGroup {
		for _, sub := range a.Value.Group() {
			if attrContainsText(sub, needle) {
				return true
			}
		}
		return false
	}
	return strings.Contains(a.Value.String(), needle)
}

// fakeUsageCycleClient is a fake session.julesStatusClient (unexported,
// satisfied structurally) whose GetSession returns QUEUED on the first call
// for a given session and COMPLETED-with-a-PR on every call after — enough
// to drive JulesSessionPoller through a non-terminal state (so
// noteStateChangeIfNeeded logs "jules session state changed") and then to
// completion (session.SetBacklogItemPRAndTransition), without needing a
// second fixture per session.
type fakeUsageCycleClient struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeUsageCycleClient) IsLimited() bool { return false }

func (f *fakeUsageCycleClient) GetSession(_ context.Context, name jules.JulesSessionName) (*jules.JulesSession, error) {
	f.mu.Lock()
	f.calls++
	n := f.calls
	f.mu.Unlock()

	if n == 1 {
		return &jules.JulesSession{Name: name, State: jules.JulesStateQueued}, nil
	}
	return &jules.JulesSession{
		Name:  name,
		State: jules.JulesStateCompleted,
		Outputs: []jules.JulesSessionOutput{{
			PullRequest: &jules.JulesPullRequestOutput{URL: "https://github.com/tstapler/stapler-squad/pull/42"},
		}},
	}, nil
}

// fakeUsagePollErrorClient's first GetSession call returns a transient
// error (Task 4.1.1b's "one failed poll" scenario). Every call after that
// blocks on ctx.Done() instead of returning a fixture — since the item
// session this drives is already ended after the first failure classifies
// as jules_session_missing... no: a transient error does NOT end the
// session (only ErrJulesSessionNotFound does), so the session stays open
// and the poller would keep calling GetSession on every subsequent tick.
// Blocking on ctx.Done() (which fires when the test's poller.Stop() cancels
// the shared context) guarantees no test, however slow its scheduler, can
// ever observe a second increment before asserting on the first.
type fakeUsagePollErrorClient struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeUsagePollErrorClient) IsLimited() bool { return false }

func (f *fakeUsagePollErrorClient) GetSession(ctx context.Context, _ jules.JulesSessionName) (*jules.JulesSession, error) {
	f.mu.Lock()
	f.calls++
	first := f.calls == 1
	f.mu.Unlock()

	if first {
		return nil, jules.ErrJulesTransient
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestJulesFullCycle_should_LogNoSecretMaterial_When_DispatchPollCompleteCycleRunsAgainstFakes(t *testing.T) {
	const testAPIKey = "AIzaSyD-EXAMPLE"

	handler := &capturingSlogHandler{}
	origDefault := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(origDefault) })

	storage := createTestStorage(t)
	ctx := t.Context()

	item := createTestJulesReadyItem(t, ctx, storage, "full cycle item")
	cfg := newTestJulesConfig(item.RepoPath)
	guard := &fakeJulesTransitionGuard{}
	creator := &fakeJulesSessionCreator{resultName: "sessions/full-cycle"}

	usage := NewJulesUsageCounter()
	dispatchSvc := NewJulesDispatchService(storage, guard, creator, newTestJulesSourceRegistry(), cfg)
	dispatchSvc.SetUsageCounter(usage)

	// The prompt deliberately embeds the test API key: DispatchToJules must
	// never log req.Prompt (the Observability Plan's "jules dispatch
	// requested" line only carries item_id/repo/branch/source_name), so this
	// doubles as a regression guard against a future change that logs it.
	req, err := NewJulesDispatchRequest(item.ID, "backlog/e2e-1", "Investigate using key "+testAPIKey)
	require.NoError(t, err)

	name, err := dispatchSvc.DispatchToJules(ctx, item, req)
	require.NoError(t, err)
	assert.Equal(t, jules.JulesSessionName("sessions/full-cycle"), name)

	// guard is a fake — its transitionWithGuard call above only mutated the
	// in-memory item struct, never the real storage row. The poller's own
	// completion path (applyCompletedState) transitions in_progress -> review
	// against a real precondition check on that row, so this test must apply
	// the real transition itself first, mirroring what *BacklogService's real
	// transitionWithGuard would have done in production.
	_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, session.BacklogStatusInProgress,
		&session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusReady)}, session.TriggeredByUser)
	require.NoError(t, err)

	pollClient := &fakeUsageCycleClient{}
	poller := session.NewJulesSessionPoller(pollClient, storage, session.JulesSessionPollerConfig{
		PollInterval:  2 * time.Millisecond,
		CallTimeout:   time.Second,
		MaxSessionAge: time.Hour,
	})
	poller.SetUsageCounter(usage)
	poller.Start(ctx)
	t.Cleanup(poller.Stop)

	require.Eventually(t, func() bool {
		updated, err := storage.GetBacklogItem(ctx, item.ID)
		return err == nil && updated != nil && updated.Status == string(session.BacklogStatusReview)
	}, 2*time.Second, 5*time.Millisecond, "item must reach review once the poller observes Jules' COMPLETED state")

	messages := handler.messages()
	assert.Contains(t, messages, "jules dispatch requested")
	assert.Contains(t, messages, "jules session created")
	assert.Contains(t, messages, "jules session state changed")

	assert.False(t, handler.containsText(testAPIKey), "no captured record may contain the api key")
	assert.False(t, handler.containsText("x-goog-api-key"), "no captured record may contain the auth header name")

	snap := usage.Snapshot()
	assert.Equal(t, int64(1), snap.SessionDispatched)
	assert.Equal(t, int64(1), snap.SessionCompleted)
}

func TestJulesUsageCounter_Snapshot_should_IncrementDispatchedAndAPIErrorExactlyOnce_When_OneDispatchAndOnePollFailure(t *testing.T) {
	storage := createTestStorage(t)
	ctx := t.Context()

	item := createTestJulesReadyItem(t, ctx, storage, "dispatch plus failed poll item")
	cfg := newTestJulesConfig(item.RepoPath)
	guard := &fakeJulesTransitionGuard{}
	creator := &fakeJulesSessionCreator{}

	usage := NewJulesUsageCounter()
	dispatchSvc := NewJulesDispatchService(storage, guard, creator, newTestJulesSourceRegistry(), cfg)
	dispatchSvc.SetUsageCounter(usage)

	req, err := NewJulesDispatchRequest(item.ID, "backlog/e2e-1", "Fix the flaky poller test")
	require.NoError(t, err)
	_, err = dispatchSvc.DispatchToJules(ctx, item, req)
	require.NoError(t, err)

	pollClient := &fakeUsagePollErrorClient{}
	poller := session.NewJulesSessionPoller(pollClient, storage, session.JulesSessionPollerConfig{
		PollInterval:  2 * time.Millisecond,
		CallTimeout:   time.Second,
		MaxSessionAge: time.Hour,
	})
	poller.SetUsageCounter(usage)
	poller.Start(ctx)
	t.Cleanup(poller.Stop)

	require.Eventually(t, func() bool {
		return usage.Snapshot().APIError == 1
	}, time.Second, 2*time.Millisecond, "the one failed poll must increment jules.api.error")

	snap := usage.Snapshot()
	assert.Equal(t, int64(1), snap.SessionDispatched)
	assert.Equal(t, int64(1), snap.APIError)
}
