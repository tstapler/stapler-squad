package svctest

import (
	"context"
	"sync"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/tokens"
)

type FakeTokenStore struct {
	Results []*tokens.ParseResult
	Loading bool
	// SubscribeCh, when set, is returned as-is by Subscribe() so a test can
	// push values directly onto it (deterministic control over watchInsights's
	// receive loop, vs. racing a real TokenStore's background walk timing).
	// Subscribe() falls back to a fresh unconnected channel when unset,
	// preserving the original behavior for tests that don't care.
	SubscribeCh chan *tokens.ParseResult
}

func (f *FakeTokenStore) GetAll() []*tokens.ParseResult { return f.Results }

func (f *FakeTokenStore) GetByUUID(uuid string) *tokens.ParseResult {
	for _, r := range f.Results {
		if r.SessionUUID == uuid {
			return r
		}
	}
	return nil
}

func (f *FakeTokenStore) IsLoading() bool { return f.Loading }

func (f *FakeTokenStore) Subscribe() <-chan *tokens.ParseResult {
	if f.SubscribeCh != nil {
		return f.SubscribeCh
	}
	return make(chan *tokens.ParseResult, 1)
}

func (f *FakeTokenStore) Unsubscribe(_ <-chan *tokens.ParseResult) {}

type FakeBacklogReader struct {
	Entries []backlogdto.ItemSessionBacklogEntry
	err     error
	// Calls counts GetAllItemSessionsWithBacklogInfo invocations — see
	// TestGetInsightsSummary_should_FetchBacklogEntriesAndLedgerExactlyOnce.
	Calls int
}

func (f *FakeBacklogReader) GetAllItemSessionsWithBacklogInfo(_ context.Context) ([]backlogdto.ItemSessionBacklogEntry, error) {
	f.Calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.Entries, nil
}

// newResult builds a minimal ParseResult for tests.
func NewResult(uuid, model, projectPath string, input, output, cacheRead int64, modTime time.Time) *tokens.ParseResult {
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
type FakeInsightsEventSender struct {
	mu   sync.Mutex
	sent []*sessionv1.InsightsEvent
}

func (f *FakeInsightsEventSender) Send(e *sessionv1.InsightsEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, e)
	return nil
}

// Sent returns a snapshot copy of the messages sent so far, safe to read
// concurrently with in-flight Send calls.
func (f *FakeInsightsEventSender) Sent() []*sessionv1.InsightsEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*sessionv1.InsightsEvent, len(f.sent))
	copy(out, f.sent)
	return out
}
