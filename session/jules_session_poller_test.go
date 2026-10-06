package session

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/jules"
	ssqlog "github.com/tstapler/stapler-squad/log"
)

// --- fakes ---

// fakeJulesStatusClient is a minimal in-memory julesStatusClient. sessions
// maps a JulesSessionName to either a canned *jules.JulesSession or an error;
// calls records every GetSession invocation in order for assertion.
type fakeJulesStatusClient struct {
	mu       sync.Mutex
	limited  bool
	sessions map[jules.JulesSessionName]*jules.JulesSession
	errs     map[jules.JulesSessionName]error
	calls    []jules.JulesSessionName

	// blockUntilCtxDone, when set, makes GetSession ignore sessions/errs
	// entirely and instead block on <-ctx.Done(), returning ctx.Err(). This
	// lets a test observe whether the real GetSession call site's
	// context.WithTimeout(ctx, CallTimeout) (jules_session_poller.go's
	// processEntry) is actually enforced, and whether the resulting
	// context.DeadlineExceeded is routed through handleGetSessionError's
	// default log-and-swallow branch.
	blockUntilCtxDone bool
}

func newFakeJulesStatusClient() *fakeJulesStatusClient {
	return &fakeJulesStatusClient{
		sessions: make(map[jules.JulesSessionName]*jules.JulesSession),
		errs:     make(map[jules.JulesSessionName]error),
	}
}

func (f *fakeJulesStatusClient) IsLimited() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.limited
}

func (f *fakeJulesStatusClient) GetSession(ctx context.Context, name jules.JulesSessionName) (*jules.JulesSession, error) {
	f.mu.Lock()
	f.calls = append(f.calls, name)
	block := f.blockUntilCtxDone
	f.mu.Unlock()

	if block {
		// Must not hold f.mu while blocking, or callCount()/other assertions
		// on f would deadlock against this goroutine.
		<-ctx.Done()
		return nil, ctx.Err()
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.errs[name]; ok {
		return nil, err
	}
	if s, ok := f.sessions[name]; ok {
		return s, nil
	}
	return nil, fmt.Errorf("fakeJulesStatusClient: no fixture for %s", name)
}

func (f *fakeJulesStatusClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// fakeNote / fakeTransition / fakePRRecord record calls to fakeJulesPollerStorage
// for assertion.
type fakeNote struct {
	itemID         string
	criterionIndex int
	note           string
	status         string
}

type fakeTransition struct {
	itemID         string
	toStatus       BacklogStatus
	expectedStatus string
}

type fakePRRecord struct {
	itemID   string
	prURL    string
	prNumber int
}

// fakeJulesPollerStorage is a minimal in-memory julesPollerStorage.
type fakeJulesPollerStorage struct {
	mu sync.Mutex

	entries        []ItemSessionBacklogEntry
	sessionsByUUID map[string]ItemSessionSummary

	touched     []string
	ended       map[string]string
	notes       []fakeNote
	transitions []fakeTransition
	prRecorded  []fakePRRecord
}

func newFakeJulesPollerStorage() *fakeJulesPollerStorage {
	return &fakeJulesPollerStorage{
		sessionsByUUID: make(map[string]ItemSessionSummary),
		ended:          make(map[string]string),
	}
}

func (f *fakeJulesPollerStorage) addOpenSession(entry ItemSessionBacklogEntry, row ItemSessionSummary) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, entry)
	f.sessionsByUUID[entry.SessionUUID] = row
}

func (f *fakeJulesPollerStorage) ListOpenJulesItemSessions(_ context.Context) ([]ItemSessionBacklogEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ItemSessionBacklogEntry, len(f.entries))
	copy(out, f.entries)
	return out, nil
}

func (f *fakeJulesPollerStorage) GetItemSessionBySessionUUID(_ context.Context, sessionUUID string) (ItemSessionSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.sessionsByUUID[sessionUUID]
	if !ok {
		return ItemSessionSummary{}, fmt.Errorf("%w: %s", ErrNotFound, sessionUUID)
	}
	return row, nil
}

func (f *fakeJulesPollerStorage) TouchItemSessionProgress(_ context.Context, id string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.touched = append(f.touched, id)
	return nil
}

func (f *fakeJulesPollerStorage) UpdateItemSessionEndedWithReason(_ context.Context, id string, _ time.Time, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ended[id] = reason
	return nil
}

func (f *fakeJulesPollerStorage) AppendProgressNote(_ context.Context, itemID string, criterionIndex int, note, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notes = append(f.notes, fakeNote{itemID, criterionIndex, note, status})
	return nil
}

func (f *fakeJulesPollerStorage) SetBacklogItemPRAndTransition(_ context.Context, observed *BacklogItemData, prURL string, prNumber int, _ string, _ *PRReassignmentGuard) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prRecorded = append(f.prRecorded, fakePRRecord{observed.ID, prURL, prNumber})
	return nil
}

func (f *fakeJulesPollerStorage) TransitionBacklogItemStatus(_ context.Context, id string, toStatus BacklogStatus, precondition *BacklogItemPrecondition, _ string) (*BacklogItemData, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	expected := ""
	if precondition != nil {
		expected = precondition.ExpectedStatus
	}
	f.transitions = append(f.transitions, fakeTransition{id, toStatus, expected})
	return &BacklogItemData{ID: id, Status: string(toStatus), UpdatedAt: time.Now()}, nil
}

func (f *fakeJulesPollerStorage) notesFor(itemID string) []fakeNote {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []fakeNote
	for _, n := range f.notes {
		if n.itemID == itemID {
			out = append(out, n)
		}
	}
	return out
}

func (f *fakeJulesPollerStorage) noteCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.notes)
}

// --- test helpers ---

func testEntry(itemID, sessionUUID string) ItemSessionBacklogEntry {
	return ItemSessionBacklogEntry{
		SessionUUID: sessionUUID,
		SessionRole: SessionRoleJulesWork,
		ItemID:      itemID,
		ItemTitle:   "test item " + itemID,
		ItemStatus:  string(BacklogStatusInProgress),
	}
}

func testRow(id string, startedAt *time.Time, createdAt time.Time) ItemSessionSummary {
	return ItemSessionSummary{
		ID:          id,
		SessionUUID: "",
		StartedAt:   startedAt,
		CreatedAt:   createdAt,
	}
}

// captureSlog temporarily redirects the injectable slog seam
// (ssqlog.SetSlogDefaultForTest) log.Warn/log.Info actually read from — not
// slog.SetDefault, which log/log.go's logAt no longer consults — to a
// buffer, at Debug level so every one of this poller's log lines is
// captured. Not t.Parallel()-safe — mirrors session/repo_path_test.go's
// existing pattern.
func captureSlog(t *testing.T) func() string {
	t.Helper()
	var buf bytes.Buffer
	prev := ssqlog.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { ssqlog.SetSlogDefaultForTest(prev) })
	return buf.String
}

func newSession(name jules.JulesSessionName, state jules.JulesSessionState) *jules.JulesSession {
	return &jules.JulesSession{
		Name:  name,
		State: state,
		URL:   "https://jules.google.com/session/" + string(name),
	}
}

// --- Story 2.3.1: the poll loop ---

// TestJulesSessionPoller_tick_should_PollEachOpenSessionExactlyOnce_When_ThreeSessionsOpen
// guards Story 2.3.1: three open sessions -> exactly three GetSession calls, one
// per JulesSessionName.
func TestJulesSessionPoller_tick_should_PollEachOpenSessionExactlyOnce_When_ThreeSessionsOpen(t *testing.T) {
	t.Parallel()
	client := newFakeJulesStatusClient()
	storage := newFakeJulesPollerStorage()

	names := []jules.JulesSessionName{"sessions/a", "sessions/b", "sessions/c"}
	for i, name := range names {
		itemID := fmt.Sprintf("item-%d", i)
		uuid := julesSessionUUIDPrefix + string(name)
		storage.addOpenSession(testEntry(itemID, uuid), testRow(fmt.Sprintf("row-%d", i), nil, time.Now()))
		client.sessions[name] = newSession(name, jules.JulesStateInProgress)
	}

	p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())
	p.tick(context.Background())

	assert.Equal(t, 3, client.callCount(), "expected exactly one GetSession call per open session")
	assert.ElementsMatch(t, names, client.calls)
}

// TestJulesSessionPoller_tick_should_SkipTickEntirely_When_ClientIsRateLimited guards
// Story 2.3.1: a rate-limited client causes the whole tick to skip, not fire doomed calls.
func TestJulesSessionPoller_tick_should_SkipTickEntirely_When_ClientIsRateLimited(t *testing.T) {
	getLog := captureSlog(t)
	client := newFakeJulesStatusClient()
	client.limited = true
	storage := newFakeJulesPollerStorage()
	storage.addOpenSession(testEntry("item-1", julesSessionUUIDPrefix+"sessions/a"), testRow("row-1", nil, time.Now()))

	p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())
	p.tick(context.Background())

	assert.Equal(t, 0, client.callCount(), "rate-limited client must make zero GetSession calls")
	logOutput := getLog()
	assert.Contains(t, logOutput, "jules poll tick")
	assert.Contains(t, logOutput, "skipped_rate_limited=true")
}

// TestJulesSessionPoller_tick_should_ApplyRemainingSessions_When_OneSessionGetSessionFails
// guards Story 2.3.1: a failing session does not abort the tick — all three are
// attempted, the first and third apply normally.
func TestJulesSessionPoller_tick_should_ApplyRemainingSessions_When_OneSessionGetSessionFails(t *testing.T) {
	getLog := captureSlog(t)
	client := newFakeJulesStatusClient()
	storage := newFakeJulesPollerStorage()

	names := []jules.JulesSessionName{"sessions/a", "sessions/b", "sessions/c"}
	for i, name := range names {
		itemID := fmt.Sprintf("item-%d", i)
		uuid := julesSessionUUIDPrefix + string(name)
		storage.addOpenSession(testEntry(itemID, uuid), testRow(fmt.Sprintf("row-%d", i), nil, time.Now()))
	}
	client.sessions[names[0]] = newSession(names[0], jules.JulesStateInProgress)
	client.errs[names[1]] = jules.ErrJulesTransient
	client.sessions[names[2]] = newSession(names[2], jules.JulesStateInProgress)

	p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())
	p.tick(context.Background())

	assert.Equal(t, 3, client.callCount(), "all three sessions must be attempted")
	assert.Contains(t, storage.touched, "row-0", "first session must be applied normally")
	assert.Contains(t, storage.touched, "row-2", "third session must be applied normally")
	assert.NotContains(t, storage.touched, "row-1", "second (failing) session must not touch progress")
	assert.Contains(t, getLog(), "jules poll failed")
}

// TestJulesSessionPoller_tick_should_LogAndSwallow_When_GetSessionCallTimesOut guards
// that processEntry's context.WithTimeout(ctx, CallTimeout) is actually enforced around
// the real GetSession call, and that the resulting context.DeadlineExceeded is routed
// through handleGetSessionError's default log-and-swallow branch: the session is neither
// ended nor transitioned, matching how jules.ErrJulesTransient is already handled.
func TestJulesSessionPoller_tick_should_LogAndSwallow_When_GetSessionCallTimesOut(t *testing.T) {
	getLog := captureSlog(t)
	client := newFakeJulesStatusClient()
	client.blockUntilCtxDone = true
	storage := newFakeJulesPollerStorage()
	storage.addOpenSession(testEntry("item-1", julesSessionUUIDPrefix+"sessions/a"), testRow("row-1", nil, time.Now()))

	cfg := DefaultJulesSessionPollerConfig()
	cfg.CallTimeout = 10 * time.Millisecond
	p := NewJulesSessionPoller(client, storage, cfg)

	p.tick(context.Background())

	assert.Equal(t, 1, client.callCount())
	logOutput := getLog()
	assert.Contains(t, logOutput, "jules poll failed")
	assert.Contains(t, logOutput, context.DeadlineExceeded.Error())
	assert.Empty(t, storage.ended, "a GetSession timeout must not end the session")
	assert.Empty(t, storage.transitions, "a GetSession timeout must not transition the item")
}

// TestJulesSessionPoller_Start_should_ReturnWithinOneTickInterval_When_ContextCancelled
// guards Story 2.3.1: cancelling ctx returns the goroutine within one tick interval,
// and a second Start on the same instance is a no-op.
func TestJulesSessionPoller_Start_should_ReturnWithinOneTickInterval_When_ContextCancelled(t *testing.T) {
	t.Parallel()
	client := newFakeJulesStatusClient()
	storage := newFakeJulesPollerStorage()
	cfg := DefaultJulesSessionPollerConfig()
	cfg.PollInterval = 10 * time.Millisecond

	p := NewJulesSessionPoller(client, storage, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)
	p.Start(ctx) // second Start must be a no-op, not a second goroutine

	cancel()

	select {
	case <-p.done:
	case <-time.After(2 * time.Second):
		t.Fatal("poller goroutine did not exit within the timeout after context cancellation")
	}
}

// --- Story 2.3.2: applyJulesState ---

// TestApplyJulesState_should_TouchProgressOnly_When_StateIsNonTerminal guards
// Story 2.3.2: IN_PROGRESS touches progress only, no transition/PR call.
func TestApplyJulesState_should_TouchProgressOnly_When_StateIsNonTerminal(t *testing.T) {
	t.Parallel()
	client := newFakeJulesStatusClient()
	storage := newFakeJulesPollerStorage()
	p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())

	entry := testEntry("item-1", julesSessionUUIDPrefix+"sessions/a")
	row := testRow("row-1", nil, time.Now())
	s := newSession("sessions/a", jules.JulesStateInProgress)

	err := p.applyJulesState(context.Background(), entry, row, s)
	require.NoError(t, err)

	assert.Contains(t, storage.touched, "row-1")
	assert.Empty(t, storage.prRecorded)
	assert.Empty(t, storage.transitions)
}

// TestApplyJulesState_should_AppendExactlyOneNote_When_StateChangesThenRepeats guards
// Story 2.3.2b: QUEUED -> PLANNING writes one note; a repeated PLANNING poll writes none.
func TestApplyJulesState_should_AppendExactlyOneNote_When_StateChangesThenRepeats(t *testing.T) {
	t.Parallel()
	client := newFakeJulesStatusClient()
	storage := newFakeJulesPollerStorage()
	p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())

	entry := testEntry("item-1", julesSessionUUIDPrefix+"sessions/a")
	row := testRow("row-1", nil, time.Now())

	// Establish baseline: last seen QUEUED.
	require.NoError(t, p.applyJulesState(context.Background(), entry, row, newSession("sessions/a", jules.JulesStateQueued)))

	before := storage.noteCount()
	require.NoError(t, p.applyJulesState(context.Background(), entry, row, newSession("sessions/a", jules.JulesStatePlanning)))
	afterFirst := storage.notesFor("item-1")
	require.Len(t, storage.notes, before+1, "QUEUED -> PLANNING must write exactly one note")
	last := afterFirst[len(afterFirst)-1]
	assert.Equal(t, -1, last.criterionIndex)
	assert.Equal(t, "Jules session is now planning.", last.note)
	assert.Equal(t, string(BacklogStatusInProgress), last.status)

	countAfterFirstPlanning := storage.noteCount()
	require.NoError(t, p.applyJulesState(context.Background(), entry, row, newSession("sessions/a", jules.JulesStatePlanning)))
	assert.Equal(t, countAfterFirstPlanning, storage.noteCount(), "repeating the same state must write no additional note")
}

// TestApplyJulesState_should_RecordPRAndHandOffToReconcilePRPending_When_CompletedWithPullRequestOutput
// guards Story 2.3.2 (Integration, storage-backed): COMPLETED with a PR output records
// the PR via SetBacklogItemPRAndTransition and ends the session jules_completed.
func TestApplyJulesState_should_RecordPRAndHandOffToReconcilePRPending_When_CompletedWithPullRequestOutput(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{Title: "jules completed item", Status: string(BacklogStatusInProgress)})
	require.NoError(t, err)

	sessionUUID := julesSessionUUIDPrefix + "sessions/xyz"
	is, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: SessionRoleJulesWork,
	})
	require.NoError(t, err)

	client := newFakeJulesStatusClient()
	p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())

	entry := testEntry(item.ID, sessionUUID)
	row, err := storage.GetItemSessionBySessionUUID(ctx, sessionUUID)
	require.NoError(t, err)
	require.Equal(t, is.ID, row.ID)

	s := newSession("sessions/xyz", jules.JulesStateCompleted)
	s.Outputs = []jules.JulesSessionOutput{
		{PullRequest: &jules.JulesPullRequestOutput{URL: "https://github.com/tstapler/stapler-squad/pull/700"}},
	}

	require.NoError(t, p.applyJulesState(ctx, entry, row, s))

	updated, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), updated.Status)
	assert.Equal(t, 700, updated.PrNumber)
	assert.Equal(t, "https://github.com/tstapler/stapler-squad/pull/700", updated.PrURL)

	endedRow, err := storage.GetItemSession(ctx, is.ID)
	require.NoError(t, err)
	require.NotNil(t, endedRow.EndedAt)
	assert.Equal(t, "jules_completed", endedRow.EndReason)
}

// TestApplyJulesState_should_LogMultiplePROutputsAtWarn_When_CompletedWithMoreThanOnePROutput
// guards the interim behavior recorded in plan.md's Unresolved Questions for
// "does Session.outputs[] ever contain more than one pull request": the first
// non-empty pullRequest.url still wins, but len(outputs) > 1 must log
// "jules multiple pr outputs" at Warn so the assumption is observable.
func TestApplyJulesState_should_LogMultiplePROutputsAtWarn_When_CompletedWithMoreThanOnePROutput(t *testing.T) {
	getLog := captureSlog(t)
	client := newFakeJulesStatusClient()
	storage := newFakeJulesPollerStorage()
	p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())

	entry := testEntry("item-1", julesSessionUUIDPrefix+"sessions/a")
	row := testRow("row-1", nil, time.Now())
	s := newSession("sessions/a", jules.JulesStateCompleted)
	s.Outputs = []jules.JulesSessionOutput{
		{PullRequest: &jules.JulesPullRequestOutput{URL: "https://github.com/tstapler/stapler-squad/pull/1"}},
		{PullRequest: &jules.JulesPullRequestOutput{URL: "https://github.com/tstapler/stapler-squad/pull/2"}},
	}

	require.NoError(t, p.applyJulesState(context.Background(), entry, row, s))

	require.Len(t, storage.prRecorded, 1)
	assert.Equal(t, "https://github.com/tstapler/stapler-squad/pull/1", storage.prRecorded[0].prURL, "must take the first non-empty pullRequest.url")

	logOutput := getLog()
	assert.Contains(t, logOutput, "jules multiple pr outputs")
	assert.Contains(t, logOutput, "jules_session="+entry.SessionUUID)
	assert.Contains(t, logOutput, "output_count=2")
	assert.Contains(t, logOutput, "level=WARN")
}

// TestApplyJulesState_should_NotLogMultiplePROutputs_When_CompletedWithSinglePROutput
// is the negative case for the same interim behavior: a single output must not
// trip the "jules multiple pr outputs" warning.
func TestApplyJulesState_should_NotLogMultiplePROutputs_When_CompletedWithSinglePROutput(t *testing.T) {
	getLog := captureSlog(t)
	client := newFakeJulesStatusClient()
	storage := newFakeJulesPollerStorage()
	p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())

	entry := testEntry("item-1", julesSessionUUIDPrefix+"sessions/a")
	row := testRow("row-1", nil, time.Now())
	s := newSession("sessions/a", jules.JulesStateCompleted)
	s.Outputs = []jules.JulesSessionOutput{
		{PullRequest: &jules.JulesPullRequestOutput{URL: "https://github.com/tstapler/stapler-squad/pull/1"}},
	}

	require.NoError(t, p.applyJulesState(context.Background(), entry, row, s))

	assert.NotContains(t, getLog(), "jules multiple pr outputs")
}

// TestApplyJulesState_should_SurfaceMissingPR_When_CompletedWithEmptyOutputs guards
// Story 2.3.2: COMPLETED with no PR output is surfaced, not silently treated as success.
func TestApplyJulesState_should_SurfaceMissingPR_When_CompletedWithEmptyOutputs(t *testing.T) {
	t.Parallel()
	client := newFakeJulesStatusClient()
	storage := newFakeJulesPollerStorage()
	p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())

	entry := testEntry("item-1", julesSessionUUIDPrefix+"sessions/a")
	row := testRow("row-1", nil, time.Now())
	s := newSession("sessions/a", jules.JulesStateCompleted)
	s.Outputs = nil

	require.NoError(t, p.applyJulesState(context.Background(), entry, row, s))

	assert.Empty(t, storage.prRecorded, "no PR output must never call SetBacklogItemPRAndTransition")
	require.Contains(t, storage.ended, "row-1")
	assert.Equal(t, "jules_completed_no_pr", storage.ended["row-1"])
	notes := storage.notesFor("item-1")
	require.NotEmpty(t, notes)
	assert.Contains(t, notes[len(notes)-1].note, s.URL, "note must point at the Jules web URL")
}

// TestJulesIsValidGitHubPRURL_should_AcceptOnlyGenuineGitHubPullURLs guards the
// security-review fix: prNumberFromURLRe alone only anchors the trailing
// "/pull/(\d+)/?" shape, with no start-anchor or scheme/host check, so a
// value like "javascript:alert(1)//pull/1" satisfies it. isValidGitHubPRURL
// must additionally require an https://github.com/ URL before the PR-URL
// capture path accepts anything from the Jules API response.
func TestJulesIsValidGitHubPRURL_should_AcceptOnlyGenuineGitHubPullURLs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"genuine github pr url", "https://github.com/tstapler/stapler-squad/pull/123", true},
		{"genuine github pr url with trailing slash", "https://github.com/tstapler/stapler-squad/pull/123/", true},
		{"javascript uri smuggled past trailing regex", "javascript:alert(1)//pull/1", false},
		{"non-github host with pull path", "https://evil.com/x/pull/1", false},
		{"http instead of https", "http://github.com/tstapler/stapler-squad/pull/123", false},
		{"github host but no pull path", "https://github.com/tstapler/stapler-squad/issues/123", false},
		{"empty string", "", false},
		{"lookalike subdomain", "https://github.com.evil.com/x/pull/1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isValidGitHubPRURL(tt.url))
		})
	}
}

// TestApplyJulesState_should_RejectMaliciousPRURL_When_CompletedWithNonGitHubURL
// guards the same fix end-to-end through applyJulesState: a PR output whose
// URL isn't a genuine https://github.com/ URL must never reach
// SetBacklogItemPRAndTransition (i.e. never become item.PrURL, which
// GitHubBadge.tsx renders directly as an <a href>) and must fall back to the
// existing "completed with no PR" path instead of failing loudly, so a
// single malicious output doesn't wedge the whole session.
func TestApplyJulesState_should_RejectMaliciousPRURL_When_CompletedWithNonGitHubURL(t *testing.T) {
	getLog := captureSlog(t)
	client := newFakeJulesStatusClient()
	storage := newFakeJulesPollerStorage()
	p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())

	entry := testEntry("item-1", julesSessionUUIDPrefix+"sessions/a")
	row := testRow("row-1", nil, time.Now())
	s := newSession("sessions/a", jules.JulesStateCompleted)
	s.Outputs = []jules.JulesSessionOutput{
		{PullRequest: &jules.JulesPullRequestOutput{URL: "javascript:alert(1)//pull/1"}},
	}

	require.NoError(t, p.applyJulesState(context.Background(), entry, row, s))

	assert.Empty(t, storage.prRecorded, "an invalid PR URL must never reach SetBacklogItemPRAndTransition / item.PrURL")
	require.Contains(t, storage.ended, "row-1")
	assert.Equal(t, "jules_completed_no_pr", storage.ended["row-1"], "must fall back to the no-PR path, not error out")

	logOutput := getLog()
	assert.Contains(t, logOutput, "jules rejected invalid pr url")
	assert.Contains(t, logOutput, "url=javascript:alert(1)//pull/1")
	assert.Contains(t, logOutput, "level=WARN")
}

// TestApplyJulesState_should_LogUnknownStateAtError_When_StateIsUnrecognized guards
// Story 2.3.2: an unknown state logs Error with raw_state, touches progress, no transition.
func TestApplyJulesState_should_LogUnknownStateAtError_When_StateIsUnrecognized(t *testing.T) {
	getLog := captureSlog(t)
	client := newFakeJulesStatusClient()
	storage := newFakeJulesPollerStorage()
	p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())

	entry := testEntry("item-1", julesSessionUUIDPrefix+"sessions/a")
	row := testRow("row-1", nil, time.Now())
	s := newSession("sessions/a", jules.ParseJulesSessionState("AWAITING_HUMAN_TEA_BREAK"))

	require.NoError(t, p.applyJulesState(context.Background(), entry, row, s))

	assert.Contains(t, storage.touched, "row-1")
	assert.Empty(t, storage.transitions)
	logOutput := getLog()
	assert.Contains(t, logOutput, "jules unknown session state")
	assert.Contains(t, logOutput, "AWAITING_HUMAN_TEA_BREAK")
	assert.Contains(t, logOutput, "level=ERROR")
}

// TestApplyJulesState_should_HandleEveryDeclaredState_When_StatesEnumerated guards Story
// 2.3.2's exhaustiveness requirement: every exported jules.JulesSessionState constant
// produces a non-default effect. A future state added to jules/types.go without a row
// added here will not fail to compile, but will fail this test's "seven known states"
// count sentinel — a deliberate trip wire for whoever adds the new constant.
// julesStateConstPattern matches one `JulesState<Name> JulesSessionState =
// "..."` line inside jules/types.go's const block, capturing <Name> — used
// to enumerate the type's actually-declared constants from source, since
// JulesSessionState is a plain string type with no runtime reflection over
// its const set (see its doc comment in jules/types.go).
var julesStateConstPattern = regexp.MustCompile(`^\s*(JulesState\w+)\s+JulesSessionState\s*=`)

// declaredJulesStateConstantNames reads jules/types.go directly (resolved
// relative to this test file via runtime.Caller, so it works regardless of
// `go test`'s working directory) and returns every `JulesState*` constant
// name declared there. This is what makes
// TestApplyJulesState_should_HandleEveryDeclaredState_When_StatesEnumerated
// actually fail when a new state is added without a corresponding table
// row, rather than relying on a human remembering to update a hardcoded
// count.
func declaredJulesStateConstantNames(t *testing.T) []string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller(0) must resolve this test file's path")
	typesPath := filepath.Join(filepath.Dir(thisFile), "..", "jules", "types.go")

	src, err := os.ReadFile(typesPath)
	require.NoError(t, err, "read %s", typesPath)

	var names []string
	for _, line := range strings.Split(string(src), "\n") {
		if m := julesStateConstPattern.FindStringSubmatch(line); m != nil {
			names = append(names, m[1])
		}
	}
	require.NotEmpty(t, names, "found zero JulesState* constants in %s -- regex or file path is wrong", typesPath)
	return names
}

func TestApplyJulesState_should_HandleEveryDeclaredState_When_StatesEnumerated(t *testing.T) {
	t.Parallel()

	namedStates := map[string]jules.JulesSessionState{
		"JulesStateQueued":               jules.JulesStateQueued,
		"JulesStatePlanning":             jules.JulesStatePlanning,
		"JulesStateAwaitingPlanApproval": jules.JulesStateAwaitingPlanApproval,
		"JulesStateInProgress":           jules.JulesStateInProgress,
		"JulesStateCompleted":            jules.JulesStateCompleted,
		"JulesStateFailed":               jules.JulesStateFailed,
		"JulesStateUnknown":              jules.JulesStateUnknown,
	}

	for _, name := range declaredJulesStateConstantNames(t) {
		if _, ok := namedStates[name]; !ok {
			t.Errorf("jules.%s is declared in jules/types.go but has no row in this test's namedStates table -- add one so its applyJulesState handling is exercised here", name)
		}
	}

	i := 0
	for _, state := range namedStates {
		i++
		state := state
		t.Run(state.String(), func(t *testing.T) {
			client := newFakeJulesStatusClient()
			storage := newFakeJulesPollerStorage()
			p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())

			itemID := fmt.Sprintf("item-%d", i)
			sessionUUID := julesSessionUUIDPrefix + fmt.Sprintf("sessions/s%d", i)
			entry := testEntry(itemID, sessionUUID)
			row := testRow(fmt.Sprintf("row-%d", i), nil, time.Now())
			s := newSession(jules.JulesSessionName(strings.TrimPrefix(sessionUUID, julesSessionUUIDPrefix)), state)
			if state == jules.JulesStateFailed {
				s.Title = "build failed"
			}

			require.NoError(t, p.applyJulesState(context.Background(), entry, row, s))

			// A "non-default effect" means at least one of: progress touched,
			// the session ended, or a note appended — never a silent no-op.
			effect := len(storage.touched) > 0 || len(storage.ended) > 0 || storage.noteCount() > 0
			assert.True(t, effect, "state %s must produce a non-default effect", state)
		})
	}
}

// --- Story 2.3.3: failure/staleness/reservation reconciliation ---

// TestApplyJulesState_should_ReturnItemToReadyWithJulesMessage_When_StateIsFailed guards
// Story 2.3.3: FAILED ends the session and returns the item to ready with Jules' own
// message attributed.
func TestApplyJulesState_should_ReturnItemToReadyWithJulesMessage_When_StateIsFailed(t *testing.T) {
	t.Parallel()
	client := newFakeJulesStatusClient()
	storage := newFakeJulesPollerStorage()
	p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())

	entry := testEntry("item-1", julesSessionUUIDPrefix+"sessions/xyz")
	row := testRow("row-1", nil, time.Now())
	s := newSession("sessions/xyz", jules.JulesStateFailed)
	s.Title = "could not push branch: permission denied"
	s.URL = "https://jules.google.com/session/xyz"

	require.NoError(t, p.applyJulesState(context.Background(), entry, row, s))

	require.Contains(t, storage.ended, "row-1")
	assert.Equal(t, "jules_failed", storage.ended["row-1"])
	require.NotEmpty(t, storage.transitions)
	lastTransition := storage.transitions[len(storage.transitions)-1]
	assert.Equal(t, BacklogStatusReady, lastTransition.toStatus)
	assert.Equal(t, string(BacklogStatusInProgress), lastTransition.expectedStatus)

	notes := storage.notesFor("item-1")
	require.NotEmpty(t, notes)
	last := notes[len(notes)-1]
	assert.Contains(t, last.note, "could not push branch: permission denied")
	assert.Contains(t, last.note, "https://jules.google.com/session/xyz")
}

// TestJulesSessionPoller_tick_should_EndSessionAsSessionMissing_When_GetSessionReturnsNotFound
// guards Story 2.3.3: a vanished session does not retry forever.
func TestJulesSessionPoller_tick_should_EndSessionAsSessionMissing_When_GetSessionReturnsNotFound(t *testing.T) {
	t.Parallel()
	client := newFakeJulesStatusClient()
	storage := newFakeJulesPollerStorage()
	entry := testEntry("item-1", julesSessionUUIDPrefix+"sessions/gone")
	storage.addOpenSession(entry, testRow("row-1", nil, time.Now()))
	client.errs["sessions/gone"] = jules.ErrJulesSessionNotFound

	p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())
	p.tick(context.Background())

	require.Contains(t, storage.ended, "row-1")
	assert.Equal(t, "jules_session_missing", storage.ended["row-1"])
	require.NotEmpty(t, storage.transitions)
	assert.Equal(t, BacklogStatusReady, storage.transitions[len(storage.transitions)-1].toStatus)
}

// TestJulesSessionPoller_tick_should_TimeOutSession_When_StartedAtExceedsMaxSessionAge
// guards Story 2.3.3: a session exceeding MaxSessionAge is failed rather than polled
// indefinitely, and no further GetSession call is made for it on the next tick.
func TestJulesSessionPoller_tick_should_TimeOutSession_When_StartedAtExceedsMaxSessionAge(t *testing.T) {
	t.Parallel()
	client := newFakeJulesStatusClient()
	storage := newFakeJulesPollerStorage()

	started := time.Now().Add(-25 * time.Hour)
	entry := testEntry("item-1", julesSessionUUIDPrefix+"sessions/old")
	storage.addOpenSession(entry, testRow("row-1", &started, time.Now().Add(-25*time.Hour)))

	cfg := DefaultJulesSessionPollerConfig()
	cfg.MaxSessionAge = 24 * time.Hour
	p := NewJulesSessionPoller(client, storage, cfg)

	p.tick(context.Background())

	assert.Equal(t, 0, client.callCount(), "a timed-out session must not be polled")
	require.Contains(t, storage.ended, "row-1")
	assert.Equal(t, "jules_timed_out", storage.ended["row-1"])

	// Next tick: the ItemSession is now ended, so a fresh ListOpenJulesItemSessions
	// (which the fake does not simulate removing) would still hand it back if the
	// entry weren't removed. Simulate the real DB's ended_at IS NULL predicate:
	storage.mu.Lock()
	storage.entries = nil
	storage.mu.Unlock()

	p.tick(context.Background())
	assert.Equal(t, 0, client.callCount(), "no further GetSession call after the session is ended")
}

// TestJulesSessionPoller_tick_should_FailAbandonedReservation_When_PendingOlderThanTenMinutes
// guards Story 2.3.3 (Integration: storage + injected clock): a jules-pending- row
// older than 10 minutes is cleaned up.
func TestJulesSessionPoller_tick_should_FailAbandonedReservation_When_PendingOlderThanTenMinutes(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{Title: "jules pending item", Status: string(BacklogStatusInProgress)})
	require.NoError(t, err)

	pendingUUID := julesPendingUUIDPrefix + "reservation-1"
	parsedItemID, err := uuid.Parse(item.ID)
	require.NoError(t, err)
	is, err := storage.GetEntClient().ItemSession.Create().
		SetSessionUUID(pendingUUID).
		SetSessionRole(SessionRoleJulesWork).
		SetBacklogItemID(parsedItemID).
		SetCreatedAt(time.Now().Add(-15 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	client := newFakeJulesStatusClient()
	p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())
	p.nowFn = time.Now

	p.tick(ctx)

	assert.Equal(t, 0, client.callCount(), "a pending reservation must never be polled via GetSession")

	endedRow, err := storage.GetItemSession(ctx, is.ID.String())
	require.NoError(t, err)
	require.NotNil(t, endedRow.EndedAt)
	assert.Equal(t, "dispatch_incomplete", endedRow.EndReason)

	updated, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusReady), updated.Status)

	notes, err := storage.ListProgressNotesForItem(ctx, item.ID)
	require.NoError(t, err)
	require.NotEmpty(t, notes)
	assert.Contains(t, notes[len(notes)-1].Note, "jules.google.com")
}

// --- Story 2.3.4: reconnect-required ---

// TestJulesSessionPoller_tick_should_SetAuthReconnectRequiredAndSkipSession_When_GetSessionReturnsErrJulesNotConfigured
// guards Story 2.3.4: a 401/403 mid-poll does not end/transition/touch the session.
func TestJulesSessionPoller_tick_should_SetAuthReconnectRequiredAndSkipSession_When_GetSessionReturnsErrJulesNotConfigured(t *testing.T) {
	t.Parallel()
	client := newFakeJulesStatusClient()
	storage := newFakeJulesPollerStorage()
	entry := testEntry("item-1", julesSessionUUIDPrefix+"sessions/a")
	storage.addOpenSession(entry, testRow("row-1", nil, time.Now()))
	client.errs["sessions/a"] = jules.ErrJulesNotConfigured

	p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())
	require.False(t, p.AuthReconnectRequired())

	p.tick(context.Background())

	assert.True(t, p.AuthReconnectRequired())
	assert.Empty(t, storage.ended)
	assert.Empty(t, storage.transitions)
	assert.NotContains(t, storage.touched, "row-1")
}

// TestJulesSessionPoller_tick_should_AppendReauthNoteExactlyOnce_When_ThreeConsecutiveTicksReturnErrJulesNotConfigured
// guards Story 2.3.4: the condition is surfaced once per occurrence, not once per tick.
func TestJulesSessionPoller_tick_should_AppendReauthNoteExactlyOnce_When_ThreeConsecutiveTicksReturnErrJulesNotConfigured(t *testing.T) {
	t.Parallel()
	client := newFakeJulesStatusClient()
	storage := newFakeJulesPollerStorage()
	entry := testEntry("item-1", julesSessionUUIDPrefix+"sessions/a")
	storage.addOpenSession(entry, testRow("row-1", nil, time.Now()))
	client.errs["sessions/a"] = jules.ErrJulesNotConfigured

	p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())
	for i := 0; i < 3; i++ {
		p.tick(context.Background())
	}

	blockedNotes := 0
	for _, n := range storage.notesFor("item-1") {
		if n.note == julesAuthBlockedNote {
			blockedNotes++
		}
	}
	assert.Equal(t, 1, blockedNotes, "the reauth note must be written exactly once across three consecutive failing ticks")
}

// TestJulesSessionPoller_tick_should_ClearAuthReconnectRequiredAndAppendRecoveryNote_When_SubsequentTickSucceeds
// guards Story 2.3.4: recovery is automatic on the next successful poll.
func TestJulesSessionPoller_tick_should_ClearAuthReconnectRequiredAndAppendRecoveryNote_When_SubsequentTickSucceeds(t *testing.T) {
	t.Parallel()
	client := newFakeJulesStatusClient()
	storage := newFakeJulesPollerStorage()
	entry := testEntry("item-1", julesSessionUUIDPrefix+"sessions/a")
	storage.addOpenSession(entry, testRow("row-1", nil, time.Now()))
	client.errs["sessions/a"] = jules.ErrJulesNotConfigured

	p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())
	p.tick(context.Background())
	require.True(t, p.AuthReconnectRequired())

	delete(client.errs, "sessions/a")
	client.sessions["sessions/a"] = newSession("sessions/a", jules.JulesStateInProgress)

	p.tick(context.Background())

	assert.False(t, p.AuthReconnectRequired())
	recoveryNotes := 0
	for _, n := range storage.notesFor("item-1") {
		if n.note == julesAuthRestoredNote {
			recoveryNotes++
		}
	}
	assert.Equal(t, 1, recoveryNotes)

	// The very next tick resumes ordinary applyJulesState handling: progress
	// touched normally, no auth-blocked branch re-triggered.
	touchedBefore := len(storage.touched)
	p.tick(context.Background())
	assert.Greater(t, len(storage.touched), touchedBefore)
}

// TestJulesSessionPoller_tick_should_SetAuthReconnectRequiredOnceNotPerSession_When_TwoSessionsOnDifferentItemsBothFail
// guards Story 2.3.4: the flag is process-level, not per-session; each item still gets
// its own dedup'd note.
func TestJulesSessionPoller_tick_should_SetAuthReconnectRequiredOnceNotPerSession_When_TwoSessionsOnDifferentItemsBothFail(t *testing.T) {
	t.Parallel()
	client := newFakeJulesStatusClient()
	storage := newFakeJulesPollerStorage()
	storage.addOpenSession(testEntry("item-1", julesSessionUUIDPrefix+"sessions/a"), testRow("row-1", nil, time.Now()))
	storage.addOpenSession(testEntry("item-2", julesSessionUUIDPrefix+"sessions/b"), testRow("row-2", nil, time.Now()))
	client.errs["sessions/a"] = jules.ErrJulesNotConfigured
	client.errs["sessions/b"] = jules.ErrJulesNotConfigured

	p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())
	p.tick(context.Background())

	assert.True(t, p.AuthReconnectRequired())
	item1Blocked, item2Blocked := 0, 0
	for _, n := range storage.notesFor("item-1") {
		if n.note == julesAuthBlockedNote {
			item1Blocked++
		}
	}
	for _, n := range storage.notesFor("item-2") {
		if n.note == julesAuthBlockedNote {
			item2Blocked++
		}
	}
	assert.Equal(t, 1, item1Blocked)
	assert.Equal(t, 1, item2Blocked)
}

// TestJulesSessionPoller_tick_should_LeaveAuthReconnectRequiredUntouched_When_TransientOrSessionNotFoundErrorsOccur
// guards Story 2.3.4: every other error path is unaffected by the auth-reconnect flag.
func TestJulesSessionPoller_tick_should_LeaveAuthReconnectRequiredUntouched_When_TransientOrSessionNotFoundErrorsOccur(t *testing.T) {
	t.Parallel()
	client := newFakeJulesStatusClient()
	storage := newFakeJulesPollerStorage()
	storage.addOpenSession(testEntry("item-1", julesSessionUUIDPrefix+"sessions/a"), testRow("row-1", nil, time.Now()))
	storage.addOpenSession(testEntry("item-2", julesSessionUUIDPrefix+"sessions/b"), testRow("row-2", nil, time.Now()))
	client.errs["sessions/a"] = jules.ErrJulesTransient
	client.errs["sessions/b"] = jules.ErrJulesSessionNotFound

	p := NewJulesSessionPoller(client, storage, DefaultJulesSessionPollerConfig())
	p.tick(context.Background())

	assert.False(t, p.AuthReconnectRequired())
	// Existing Story 2.3.1/2.3.3 handling applies unchanged: ErrJulesSessionNotFound
	// ends the session; ErrJulesTransient just logs and swallows (no end/transition).
	require.Contains(t, storage.ended, "row-2")
	assert.Equal(t, "jules_session_missing", storage.ended["row-2"])
	assert.NotContains(t, storage.ended, "row-1")
}
