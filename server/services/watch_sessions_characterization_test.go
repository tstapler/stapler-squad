//go:build !windows

package services

// Characterization of WatchSessions and the unary stream-adjacent RPCs through
// the public SessionService and the Connect wire.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/federation"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/external"
	"github.com/tstapler/stapler-squad/session/instance"
)

type watchStream struct {
	t      *testing.T
	events chan *sessionv1.SessionEvent
	cancel context.CancelFunc
}

// watch opens WatchSessions and returns once the server has subscribed to the bus.
func (e *charEnv) watch(t *testing.T, req *sessionv1.WatchSessionsRequest) *watchStream {
	t.Helper()
	before := e.bus.SubscriberCount()
	ctx, cancel := context.WithCancel(context.Background())
	w := &watchStream{t: t, events: make(chan *sessionv1.SessionEvent, 64), cancel: cancel}
	done := make(chan struct{})
	go func() {
		defer close(done)
		stream, err := e.client.WatchSessions(ctx, connect.NewRequest(req))
		if err != nil {
			return
		}
		for stream.Receive() {
			w.events <- stream.Msg()
		}
		_ = stream.Close()
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(charWait):
			t.Error("WatchSessions client did not exit")
		}
	})
	require.Eventually(t, func() bool { return e.bus.SubscriberCount() > before }, charWait, 5*time.Millisecond)
	return w
}

func (w *watchStream) next() *sessionv1.SessionEvent {
	w.t.Helper()
	select {
	case ev := <-w.events:
		return ev
	case <-time.After(charWait):
		w.t.Fatal("timed out waiting for a session event")
		return nil
	}
}

// describe renders the parts of an event the contract is about.
func describe(ev *sessionv1.SessionEvent) string {
	switch x := ev.Event.(type) {
	case *sessionv1.SessionEvent_SessionCreated:
		return "created " + x.SessionCreated.Session.GetTitle()
	case *sessionv1.SessionEvent_SessionUpdated:
		return "updated " + x.SessionUpdated.Session.GetTitle() + " " + strings.Join(x.SessionUpdated.UpdatedFields, ",")
	case *sessionv1.SessionEvent_SessionDeleted:
		return "deleted " + x.SessionDeleted.SessionId
	default:
		return "other"
	}
}

func (w *watchStream) take(n int) []string {
	w.t.Helper()
	out := make([]string, 0, n)
	for range n {
		out = append(out, describe(w.next()))
	}
	return out
}

func watchInstance(title, category string, st domain.Status, hidden bool) *instance.Instance {
	inst := &instance.Instance{Title: title, UUID: uuid.NewString(), Category: category, Status: st, Hidden: hidden, Program: "claude", Path: "/tmp/test"}
	instance.FinishInstanceForTest(inst)
	return inst
}

func addAll(e *charEnv, insts ...*instance.Instance) {
	for _, i := range insts {
		e.poller.AddInstance(i)
	}
}

func str(s string) *string { return &s }

func TestWatchSessionsChar_ShouldSnapshotVisibleMatchesInOrderThenFollowTheBus_When_Fresh(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	a := watchInstance("a", "x", domain.Active, false)
	b := watchInstance("b", "y", domain.Paused, false)
	c := watchInstance("c", "x", domain.Active, true)
	d := watchInstance("d", "x", domain.Paused, false)
	addAll(e, a, b, c, d)

	w := e.watch(t, &sessionv1.WatchSessionsRequest{})
	snapshot := make([]*sessionv1.SessionEvent, 0, 3)
	for range 3 {
		snapshot = append(snapshot, w.next())
	}
	assert.Equal(t, []string{"created a", "created b", "created d"}, []string{describe(snapshot[0]), describe(snapshot[1]), describe(snapshot[2])})
	for _, ev := range snapshot {
		assert.Zero(t, ev.Seq, "snapshot events carry no sequence number")
		assert.Empty(t, ev.DeployId)
		assert.NotNil(t, ev.Timestamp)
	}

	e.bus.Publish(events.NewSessionUpdatedEvent(c, []string{"hidden"}))
	e.bus.Publish(events.NewSessionUpdatedEvent(b, []string{"status"}))
	e.bus.Publish(events.NewSessionDeletedEvent("gone"))
	live := w.next()
	assert.Equal(t, "updated b status", describe(live), "the hidden session's event is not forwarded")
	assert.NotZero(t, live.Seq)
	assert.Equal(t, events.DeployID(), live.DeployId)
	assert.Equal(t, "deleted gone", describe(w.next()))
}

func TestWatchSessionsChar_ShouldFilterSnapshotAndLiveEvents_When_FiltersAreSet(t *testing.T) {
	t.Parallel()
	active := sessionv1.SessionStatus_SESSION_STATUS_ACTIVE
	unspecified := sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED
	for _, tc := range []struct {
		name     string
		req      *sessionv1.WatchSessionsRequest
		snapshot []string
		live     []string
	}{
		{"category", &sessionv1.WatchSessionsRequest{CategoryFilter: str("x")},
			[]string{"created a", "created d"}, []string{"updated a f", "updated d f", "deleted gone"}},
		{"status", &sessionv1.WatchSessionsRequest{StatusFilter: &active},
			[]string{"created a", "created b"}, []string{"updated a f", "updated b f", "deleted gone"}},
		{"empty filters", &sessionv1.WatchSessionsRequest{CategoryFilter: str(""), StatusFilter: &unspecified},
			[]string{"created a", "created b", "created d"}, []string{"updated a f", "updated b f", "updated d f", "deleted gone"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newCharEnv(t, true)
			a := watchInstance("a", "x", domain.Active, false)
			b := watchInstance("b", "y", domain.Active, false)
			d := watchInstance("d", "x", domain.Paused, false)
			addAll(e, a, b, d)

			w := e.watch(t, tc.req)
			assert.Equal(t, tc.snapshot, w.take(len(tc.snapshot)))
			for _, inst := range []*instance.Instance{a, b, d} {
				e.bus.Publish(events.NewSessionUpdatedEvent(inst, []string{"f"}))
			}
			e.bus.Publish(events.NewSessionDeletedEvent("gone"))
			assert.Equal(t, tc.live, w.take(len(tc.live)))
		})
	}
}

// A reconnect replays the bus buffer and sends no snapshot. Replay applies the
// hidden filter only; category and status filters do not apply to it.
func TestWatchSessionsChar_ShouldReplayWithoutSnapshot_When_AfterSeqIsSet(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	a := watchInstance("a", "x", domain.Active, false)
	b := watchInstance("b", "y", domain.Active, false)
	hidden := watchInstance("h", "x", domain.Active, true)
	addAll(e, a, b, hidden)
	first := events.NewSessionUpdatedEvent(a, []string{"one"})
	e.bus.Publish(first)
	e.bus.Publish(events.NewSessionUpdatedEvent(hidden, []string{"two"}))
	e.bus.Publish(events.NewSessionUpdatedEvent(b, []string{"three"}))
	e.bus.Publish(events.NewSessionUpdatedEvent(a, []string{"four"}))

	w := e.watch(t, &sessionv1.WatchSessionsRequest{AfterSeq: first.Seq, CategoryFilter: str("x")})
	assert.Equal(t, []string{"updated b three", "updated a four"}, w.take(2))
	e.bus.Publish(events.NewSessionUpdatedEvent(b, []string{"five"}))
	e.bus.Publish(events.NewSessionUpdatedEvent(a, []string{"six"}))
	assert.Equal(t, "updated a six", describe(w.next()), "live events after the replay are filtered again")
}

func TestWatchSessionsChar_ShouldSnapshotStorage_When_NoPollerIsWired(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, false)
	require.NoError(t, e.storage.AddInstance(instance.NewPausedTestInstance("stored")))
	w := e.watch(t, &sessionv1.WatchSessionsRequest{})
	assert.Equal(t, "created stored", describe(w.next()))
}

func TestWatchSessionsChar_ShouldUnsubscribeAndReturnNil_When_ClientCancels(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	before := e.bus.SubscriberCount()
	w := e.watch(t, &sessionv1.WatchSessionsRequest{})
	w.cancel()
	require.NoError(t, e.exit.wait(t))
	require.Eventually(t, func() bool { return e.bus.SubscriberCount() == before }, charWait, 5*time.Millisecond)
}

func TestWatchSessionsChar_ShouldSendPeerSnapshotAndPeerEvents_When_Federated(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	addAll(e, watchInstance("local", "x", domain.Active, false))
	mirror := federation.NewStaticMirror("host_01ARZ3NDEKTSV4RRFFQ69G5FAV", []*sessionv1.Session{
		{Id: "p1", Title: "peer-x", Category: "x"}, {Id: "p2", Title: "peer-y", Category: "y"},
	})
	// Wired after construction: the handler must read it per call.
	e.svc.SetFederation(mirror)

	w := e.watch(t, &sessionv1.WatchSessionsRequest{CategoryFilter: str("x")})
	assert.Equal(t, []string{"created local", "created peer-x"}, w.take(2))
	mirror.Watch().Observe("h2", []*sessionv1.Session{{Id: "h2/no", Title: "no", Category: "y"}, {Id: "h2/yes", Title: "yes", Category: "x"}})
	ev := w.next()
	assert.Equal(t, "created yes", describe(ev))
	assert.Zero(t, ev.Seq, "peer events do not move the client's sequence cursor")

	// A reconnect gets no peer snapshot.
	r := e.watch(t, &sessionv1.WatchSessionsRequest{AfterSeq: 1_000_000})
	mirror.Watch().Observe("h3", []*sessionv1.Session{{Id: "h3/new", Title: "new"}})
	assert.Equal(t, "created new", describe(r.next()))
}

func TestWatchSessionsChar_ShouldSendHeartbeats_When_Idle(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("waits one heartbeat interval")
	}
	e := newCharEnv(t, true)
	w := e.watch(t, &sessionv1.WatchSessionsRequest{})
	select {
	case ev := <-w.events:
		assert.True(t, ev.Heartbeat)
		assert.NotNil(t, ev.Timestamp)
		assert.Nil(t, ev.Event)
	case <-time.After(events.HeartbeatInterval + charWait):
		t.Fatal("no heartbeat")
	}
}

func TestGetSessionDiffChar_ShouldReturnTheDocumentedCodes_When_TheRequestIsBad(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	call := func(ctx context.Context, req *sessionv1.GetSessionDiffRequest) error {
		_, err := e.svc.GetSessionDiff(ctx, connect.NewRequest(req))
		return err
	}
	ctx := context.Background()
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(call(ctx, &sessionv1.GetSessionDiffRequest{})))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(call(ctx, &sessionv1.GetSessionDiffRequest{Id: "a", ContextLines: -2})))
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	assert.Equal(t, connect.CodeCanceled, connect.CodeOf(call(cancelled, &sessionv1.GetSessionDiffRequest{Id: "a"})))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(call(ctx, &sessionv1.GetSessionDiffRequest{Id: "a"})))
}

// Not parallel: fanoutGitRepo sets git identity env.
func TestGetSessionDiffChar_ShouldDiffTheStoredDirectory_When_NoLiveInstance(t *testing.T) {
	dir := fanoutGitRepo(t)
	e := newCharEnv(t, true)
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = "line"
	}
	file := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	fanoutGitRun(t, dir, "add", ".")
	fanoutGitRun(t, dir, "commit", "-q", "-m", "base")
	lines[20] = "changed"
	require.NoError(t, os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"), 0o600))

	stored := instance.NewPausedTestInstance("dir-session")
	stored.Path = dir
	require.NoError(t, e.storage.AddInstance(stored))
	diff := func(contextLines int32) *sessionv1.DiffStats {
		resp, err := e.svc.GetSessionDiff(context.Background(), connect.NewRequest(&sessionv1.GetSessionDiffRequest{Id: "dir-session", ContextLines: contextLines}))
		require.NoError(t, err)
		return resp.Msg.DiffStats
	}
	def, one, whole, huge := diff(0), diff(1), diff(-1), diff(20_000)
	assert.EqualValues(t, 1, def.Added)
	assert.EqualValues(t, 1, def.Removed)
	assert.Less(t, len(one.Content), len(def.Content))
	assert.Greater(t, len(whole.Content), len(def.Content))
	assert.Equal(t, whole.Content, huge.Content, "a huge context_lines means the whole file")
}

func TestGetDetectionEventsChar_ShouldReturnEmptyOrNotFound_When_NoController(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	addAll(e, watchInstance("a", "", domain.Active, false))
	get := func(id string) (*connect.Response[sessionv1.GetDetectionEventsResponse], error) {
		return e.svc.GetDetectionEvents(context.Background(), connect.NewRequest(&sessionv1.GetDetectionEventsRequest{SessionId: id}))
	}
	_, err := get("nope")
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	resp, err := get("a")
	require.NoError(t, err, "no status manager")
	assert.Empty(t, resp.Msg.Events)
	e.svc.SetStatusManager(instance.NewInstanceStatusManager())
	resp, err = get("a")
	require.NoError(t, err, "no controller")
	assert.Empty(t, resp.Msg.Events)
}

func TestGetTerminalSnapshotChar_ShouldReturnTheLastLines_When_PollerOrExternalDiscoveryKnowsTheSession(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	p := newCharPane(t)
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = string(rune('A' + i))
	}
	p.pm.capture = strings.Join(lines, "\n")
	e.poller.AddInstance(p.inst)
	// A pane found only through external discovery proves that setter reaches the lookup.
	ext := newCharPane(t)
	ext.pm.capture = "external"
	discovery := external.NewExternalSessionDiscovery()
	discovery.AddSessionForTest("/tmp/char.sock", ext.inst)
	e.svc.SetExternalDiscovery(discovery)

	get := func(id string, n int32) (*sessionv1.GetTerminalSnapshotResponse, error) {
		resp, err := e.client.GetTerminalSnapshot(context.Background(), connect.NewRequest(&sessionv1.GetTerminalSnapshotRequest{SessionId: id, LastNLines: n}))
		if err != nil {
			return nil, err
		}
		return resp.Msg, nil
	}
	_, err := get("", 0)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = get("nope", 0)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

	got, err := get(p.inst.Title, 0)
	require.NoError(t, err)
	assert.Equal(t, strings.Join(lines[10:], "\n"), got.Content, "default is the last 20 lines")
	assert.False(t, got.IsEmpty)
	got, err = get(p.inst.Title, 2)
	require.NoError(t, err)
	assert.Equal(t, strings.Join(lines[28:], "\n"), got.Content)
	got, err = get("/tmp/char.sock", 5)
	require.NoError(t, err)
	assert.Equal(t, "external", got.Content)
}

func TestWriteToSessionChar_ShouldWriteOnlyWhenAllowed_When_Called(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	p := newCharPane(t)
	instance.SetHiddenForTest(t, p.inst, true)
	e.poller.AddInstance(p.inst)
	write := func(id, input string) error {
		_, err := e.client.WriteToSession(context.Background(), connect.NewRequest(&sessionv1.WriteToSessionRequest{SessionId: id, Input: input}))
		return err
	}
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(write("", "x")))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(write(p.inst.Title, "")))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(write("nope", "x")))

	err := write(p.inst.Title, "x")
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.ErrorContains(t, err, "background session and is read-only")
	assert.Empty(t, p.pm.writes())

	// The guards flag is wired once at construction and read on every call.
	e.svc.guards.SetEnabled(false)
	require.NoError(t, write(p.inst.Title, "typed"))
	assert.Equal(t, []string{"typed"}, p.pm.writes())
}
