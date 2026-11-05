package services

import (
	"context"
	"sync"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tokens"
)

type fakeTokenStore struct {
	results   []*tokens.ParseResult
	isLoading bool
	// subscribeCh, when set, is returned as-is by Subscribe() so a test can
	// push values directly onto it (deterministic control over watchInsights's
	// receive loop, vs. racing a real TokenStore's background walk timing).
	// Subscribe() falls back to a fresh unconnected channel when unset,
	// preserving the original behavior for tests that don't care.
	subscribeCh chan *tokens.ParseResult
}

func (f *fakeTokenStore) GetAll() []*tokens.ParseResult { return f.results }

func (f *fakeTokenStore) GetByUUID(uuid string) *tokens.ParseResult {
	for _, r := range f.results {
		if r.SessionUUID == uuid {
			return r
		}
	}
	return nil
}

func (f *fakeTokenStore) IsLoading() bool { return f.isLoading }

func (f *fakeTokenStore) Subscribe() <-chan *tokens.ParseResult {
	if f.subscribeCh != nil {
		return f.subscribeCh
	}
	return make(chan *tokens.ParseResult, 1)
}

func (f *fakeTokenStore) Unsubscribe(_ <-chan *tokens.ParseResult) {}

type fakeBacklogReader struct {
	entries []session.ItemSessionBacklogEntry
	err     error
	// calls counts GetAllItemSessionsWithBacklogInfo invocations — see
	// TestGetInsightsSummary_should_FetchBacklogEntriesAndLedgerExactlyOnce.
	calls int
}

func (f *fakeBacklogReader) GetAllItemSessionsWithBacklogInfo(_ context.Context) ([]session.ItemSessionBacklogEntry, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.entries, nil
}

// newResult builds a minimal ParseResult for tests.
func newResult(uuid, model, projectPath string, input, output, cacheRead int64, modTime time.Time) *tokens.ParseResult {
	return &tokens.ParseResult{
		SessionUUID:  uuid,
		PrimaryModel: model,
		ProjectPath:  projectPath,
		TotalInput:   input,
		TotalOutput:  output,
		CacheRead:    cacheRead,
		FileModTime:  modTime,
		TurnTimeline: []tokens.TurnStats{
			{
				Model:     model,
				Input:     input,
				Output:    output,
				CacheRead: cacheRead,
				Timestamp: modTime,
			},
		},
		ToolUsage: map[string]tokens.ToolTokenStats{},
	}
}

// fakeInsightsEventSender is a hand-rolled fake implementing
// insightsEventSender, capturing every sent message in a mutex-guarded slice
// — watchInsights runs in a goroutine below while the test triggers a file
// change concurrently, so Send is called concurrently with Sent().
type fakeInsightsEventSender struct {
	mu   sync.Mutex
	sent []*sessionv1.InsightsEvent
}

func (f *fakeInsightsEventSender) Send(e *sessionv1.InsightsEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, e)
	return nil
}

// Sent returns a snapshot copy of the messages sent so far, safe to read
// concurrently with in-flight Send calls.
func (f *fakeInsightsEventSender) Sent() []*sessionv1.InsightsEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*sessionv1.InsightsEvent, len(f.sent))
	copy(out, f.sent)
	return out
}
