package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// livenessProbeExecutor answers exactly the two tmux subprocess calls the
// liveness/cwd probes under test make, with no tmux server involved:
// list-sessions (CombinedOutput, behind DoesSessionExist/
// DoesSessionExistNoCache) and display-message -p '#{pane_current_path}'
// (Output, behind GetPaneCurrentPath). A zero sessionName reports the session
// as absent, which is how a confirmed-dead instance is simulated.
type livenessProbeExecutor struct {
	sessionName string
	paneCWD     string
}

func (e *livenessProbeExecutor) CombinedOutput(_ *exec.Cmd) ([]byte, error) {
	if e.sessionName == "" {
		return nil, fmt.Errorf("no server running on socket (fake: session absent)")
	}
	return []byte(e.sessionName + "\n"), nil
}

func (e *livenessProbeExecutor) Run(_ *exec.Cmd) error {
	return fmt.Errorf("livenessProbeExecutor: Run is unsupported; only list-sessions and display-message are expected on this path")
}

func (e *livenessProbeExecutor) Output(_ *exec.Cmd) ([]byte, error) {
	if e.sessionName == "" {
		return nil, fmt.Errorf("no server running on socket (fake: session absent)")
	}
	return []byte(e.paneCWD + "\n"), nil
}

// newLiveProbeInstance builds a real *session.Instance whose tmux backend is
// wired to livenessProbeExecutor, so IsBackendProcessAlive() reports true and
// GetCurrentWorkingDirectory() reports paneCWD, deterministically and with no
// tmux server. session.NewInstance (not a struct literal) is required:
// SetTmuxSession type-asserts i.processManager to *TmuxBackend, which only the
// constructor wires.
func newLiveProbeInstance(t *testing.T, title, paneCWD string) *session.Instance {
	t.Helper()
	return newProbeInstance(t, title, paneCWD, tmux.NewSessionName(title, tmux.TmuxPrefix).String())
}

// newDeadProbeInstance is newLiveProbeInstance's counterpart whose tmux session
// reports absent, so IsBackendProcessAlive() is false however plausible the
// instance's persisted cwd looks.
func newDeadProbeInstance(t *testing.T, title, paneCWD string) *session.Instance {
	t.Helper()
	return newProbeInstance(t, title, paneCWD, "")
}

func newProbeInstance(t *testing.T, title, paneCWD, liveSessionName string) *session.Instance {
	t.Helper()
	inst, err := session.NewInstance(session.InstanceOptions{Title: title, Path: t.TempDir()})
	require.NoError(t, err)

	inst.SetTmuxSession(tmux.NewTmuxSessionWithDeps(
		title, "true", &fakePtyFactory{},
		&livenessProbeExecutor{sessionName: liveSessionName, paneCWD: paneCWD},
	))
	return inst
}

// newEmptyPoller returns a ReviewQueuePoller that is never Start()ed, so it
// holds instances without running any background loop.
func newEmptyPoller() *session.ReviewQueuePoller {
	return session.NewReviewQueuePoller(session.NewReviewQueue(), session.NewInstanceStatusManager(), nil)
}

// --------------------------------------------------------------------------
// findConfirmedLiveInstance
// --------------------------------------------------------------------------

// TestFindConfirmedLiveInstance_MapHit_ReturnsWithoutStorageFallback pins the
// fast path: when the live poller already tracks the UUID, the poller's own
// instance is returned as-is and the (deliberately nil) concStorage fallback
// is never consulted — a nil concStorage would be a nil-pointer panic if the
// fast path stopped short-circuiting.
func TestFindConfirmedLiveInstance_MapHit_ReturnsWithoutStorageFallback(t *testing.T) {
	t.Parallel()

	inst := &session.Instance{Title: "map-hit", UUID: "uuid-map-hit", Path: "/tmp/test", Status: session.Active}
	poller := newEmptyPoller()
	poller.SetInstances([]*session.Instance{inst})

	svc := &SessionService{reviewQueuePoller: poller} // concStorage deliberately nil

	got := svc.findConfirmedLiveInstance("uuid-map-hit")

	require.NotNil(t, got, "a session tracked in the live poller must be returned by the map fast path")
	assert.Same(t, inst, got, "the fast path must return the poller's own live instance, not a reconstructed shadow")
}

// TestFindConfirmedLiveInstance_MapMiss_NoConcStorage_ReturnsNil covers the
// documented fake-InstanceStore degradation: with no concrete *session.Storage
// there is no persisted record to reconstruct a shadow instance from, so the
// only honest answer is nil rather than a panic or a false positive.
func TestFindConfirmedLiveInstance_MapMiss_NoConcStorage_ReturnsNil(t *testing.T) {
	t.Parallel()

	svc := &SessionService{reviewQueuePoller: newEmptyPoller()}

	assert.Nil(t, svc.findConfirmedLiveInstance("uuid-not-tracked"))
}

// TestFindConfirmedLiveInstance_MapMiss_ShadowConfirmsDead_ReturnsNil is the
// negative half of the storage fallback: the session is persisted (so a shadow
// instance IS built) but its tmux identity resolves to an isolated socket with
// no server behind it, so IsBackendProcessAlive() confirms death and the
// fallback must still return nil. Guards against the fallback over-correcting
// into "persisted therefore alive".
func TestFindConfirmedLiveInstance_MapMiss_ShadowConfirmsDead_ReturnsNil(t *testing.T) {
	t.Parallel()

	storage := createTestStorage(t)
	const uuid = "uuid-shadow-dead"
	require.NoError(t, storage.AddInstance(&session.Instance{
		Title:  "shadow-dead-" + t.Name(),
		UUID:   uuid,
		Path:   t.TempDir(),
		Status: session.Active,
		// No tmux session by this (test-name-unique) title exists on the
		// per-process isolated socket every tmux call in a `go test` binary
		// resolves to, so the shadow's list-sessions probe answers "absent"
		// without ever reaching the developer machine's real tmux server.
		TmuxPrefix: tmux.TmuxPrefix,
		Program:    "claude",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}))

	svc := &SessionService{storage: storage, concStorage: storage, reviewQueuePoller: newEmptyPoller()}

	assert.Nil(t, svc.findConfirmedLiveInstance(uuid),
		"a persisted session whose tmux process is genuinely gone must still be reported dead")
}

// TestFindConfirmedLiveInstance_MapMiss_ShadowConfirmsAlive_ReturnsInstance is
// THE regression test for the 2026-09-12 duplicate-session incident. It fails
// against the pre-fix behavior — `FindLiveInstance(uuid) != nil` alone, i.e.
// this test minus findConfirmedLiveInstance's shadow-instance fallback —
// because the session is deliberately absent from the live poller map while
// its real tmux session is still running. Only the direct OS/tmux truth check
// can tell those two situations apart, and concluding "dead" here is exactly
// what let a duplicate session spawn into a worktree the original was still
// writing to.
func TestFindConfirmedLiveInstance_MapMiss_ShadowConfirmsAlive_ReturnsInstance(t *testing.T) {
	t.Parallel()

	title := "shadow-alive-" + fmt.Sprint(time.Now().UnixNano())
	socket := startRealTmuxSession(t, title)

	storage := createTestStorage(t)
	const uuid = "uuid-shadow-alive"
	require.NoError(t, storage.AddInstance(&session.Instance{
		Title:            title,
		UUID:             uuid,
		Path:             t.TempDir(),
		Status:           session.Active,
		TmuxServerSocket: socket,
		TmuxPrefix:       tmux.TmuxPrefix,
		Program:          "claude",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}))

	// Poller is empty: the live map says "dead", the OS says "alive".
	svc := &SessionService{storage: storage, concStorage: storage, reviewQueuePoller: newEmptyPoller()}

	got := svc.findConfirmedLiveInstance(uuid)

	require.NotNil(t, got, "session missing from the live poller map but still running in tmux must be reported live")
	assert.True(t, svc.IsSessionLive(uuid), "IsSessionLive must route through the same confirmed-liveness check")
}

// TestKillTmuxPaneOnly_DeregistersFromPollerOnSuccess is the regression test
// for a real bug found in code review: findConfirmedLiveInstance's fast path
// (FindLiveInstance's poller-map hit) trusts poller registration
// unconditionally, with no IsBackendProcessAlive() check — that verification
// only runs on the map-miss fallback path. KillTmuxPaneOnly used to kill the
// tmux pane but never deregister the Instance from the poller, so a
// subsequent IsSessionLive call on the same UUID kept hitting the fast path
// and reporting it live — exactly the failure mode spawnSessionAfterGates'
// 8b2 check (findConfirmedLiveWorkSession) exists to catch, self-inflicted by
// killEndedWorkSessionPanes' own kill a few lines earlier in the same call.
// Uses a real, isolated-socket tmux session (not a mock) so this proves an
// actual kill-session subprocess call, not just an in-memory bookkeeping
// change.
func TestKillTmuxPaneOnly_DeregistersFromPollerOnSuccess(t *testing.T) {
	t.Parallel()

	title := "kill-pane-dereg-" + fmt.Sprint(time.Now().UnixNano())
	const uuid = "uuid-kill-pane-dereg"
	socket := startRealTmuxSession(t, title)

	inst, err := session.FromInstanceDataDeferred(session.InstanceData{
		Title:            title,
		UUID:             uuid,
		Path:             t.TempDir(),
		Status:           session.Active,
		TmuxServerSocket: socket,
		TmuxPrefix:       tmux.TmuxPrefix,
		Program:          "claude",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	})
	require.NoError(t, err)

	poller := newEmptyPoller()
	poller.SetInstances([]*session.Instance{inst})
	svc := &SessionService{reviewQueuePoller: poller}

	require.True(t, svc.IsSessionLive(uuid), "precondition: instance is genuinely alive in tmux and registered in the poller")

	require.NoError(t, svc.KillTmuxPaneOnly(context.Background(), uuid))

	assert.False(t, svc.IsSessionLive(uuid),
		"after a successful kill, the poller-registered instance must no longer report live — otherwise every later confirmed-liveness check (spawnSessionAfterGates' 8b2) keeps seeing a genuinely-dead session as live forever")
}

// startRealTmuxSession starts a real, detached tmux session named exactly as a
// persisted instance with this title resolves to, on the per-process isolated
// socket tmux.ResolveSocket hands every tmux call inside a `go test` binary,
// and returns that socket. Real tmux (rather than a fake executor) is
// unavoidable here: findConfirmedLiveInstance builds its shadow instance
// internally via session.FromInstanceDataDeferred, so a test never gets a
// handle to inject a test double before IsBackendProcessAlive() runs.
//
// Teardown kills only this one session, never the server: the isolated socket
// is shared by every test in the binary, so kill-server would take their tmux
// sessions down too.
func startRealTmuxSession(t *testing.T, title string) string {
	t.Helper()
	socket := string(tmux.ResolveSocket(""))
	require.NotEmpty(t, socket,
		"tmux calls must resolve to the per-process isolated test socket; refusing to create a session on the shared default server")
	name := tmux.NewSessionName(title, tmux.TmuxPrefix).String()

	// tmux.Binary() (honoring TMUX_BIN), never a bare "tmux" off PATH: a
	// developer shell often points TMUX_BIN at a different build, and a client
	// of one build talking to a server started by another reports the server as
	// gone ("server exited unexpectedly") — which would look exactly like the
	// regression this test exists to catch.
	bin := tmux.Binary()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := safeexec.CommandContext(ctx, bin, "-L", socket,
		"new-session", "-d", "-s", name, "sleep", "300").CombinedOutput()
	require.NoError(t, err, "tmux new-session -L %s -s %s: %s", socket, name, out)

	// Prove the session is actually visible on that socket before the test
	// relies on it: a silently-dead server would otherwise make this test look
	// like a real regression in findConfirmedLiveInstance.
	listed, listErr := safeexec.CommandContext(ctx, bin, "-L", socket, "list-sessions").CombinedOutput()
	require.NoError(t, listErr, "tmux list-sessions -L %s: %s", socket, listed)
	require.Contains(t, string(listed), name)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = safeexec.CommandContext(cleanupCtx, bin, "-L", socket, "kill-session", "-t", name).Run() //nolint:errcheck // best-effort teardown
	})
	return socket
}

// --------------------------------------------------------------------------
// OtherLiveSessionInsideWorktree
// --------------------------------------------------------------------------

// TestOtherLiveSessionInsideWorktree_NoPollerOrEmptyPath_NotBlocked covers the
// two guard clauses: with nothing to iterate (or nothing to compare against)
// the answer must be "not blocked", so an unwired service can never stop a
// pause/stop it has no evidence against.
func TestOtherLiveSessionInsideWorktree_NoPollerOrEmptyPath_NotBlocked(t *testing.T) {
	t.Parallel()

	noPoller := &SessionService{}
	_, blocked := noPoller.OtherLiveSessionInsideWorktree("uuid-a", "/tmp/wt")
	assert.False(t, blocked, "no reviewQueuePoller wired: nothing can be proven, so nothing may be blocked")

	withPoller := &SessionService{reviewQueuePoller: newEmptyPoller()}
	_, blocked = withPoller.OtherLiveSessionInsideWorktree("uuid-a", "")
	assert.False(t, blocked, "an empty worktree path must never match every session's cwd by prefix")
}

// TestOtherLiveSessionInsideWorktree_LiveSiblingInsideWorktree_Blocks is the
// regression test for the stop_session/pause_session incident: rework rounds of
// one backlog item deliberately share a worktree, and both tools delete the
// target's worktree, so a still-running sibling round's real cwd inside that
// directory must block the operation and name the occupant. It fails against
// the pre-fix behavior (no guard at all — the worktree was deleted regardless).
func TestOtherLiveSessionInsideWorktree_LiveSiblingInsideWorktree_Blocks(t *testing.T) {
	t.Parallel()

	worktree := t.TempDir()
	sibling := newLiveProbeInstance(t, "live-sibling-"+t.Name(), filepath.Join(worktree, "server", "services"))
	poller := newEmptyPoller()
	poller.SetInstances([]*session.Instance{sibling})
	svc := &SessionService{reviewQueuePoller: poller}

	blockingUUID, blocked := svc.OtherLiveSessionInsideWorktree("uuid-target", worktree)

	require.True(t, blocked, "a live session whose real cwd is inside the worktree must block its deletion")
	assert.Equal(t, sibling.UUID, blockingUUID, "the blocking session's UUID must be reported so the operator knows what to stop")
}

// TestOtherLiveSessionInsideWorktree_ExcludedAndUnrelatedAndDead_NotBlocked
// covers the three ways a candidate must be discarded. The sibling-directory
// case is the subtle one: a plain strings.HasPrefix without the path-separator
// boundary would treat "<worktree>-other" as living inside "<worktree>".
func TestOtherLiveSessionInsideWorktree_ExcludedAndUnrelatedAndDead_NotBlocked(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	worktree := filepath.Join(base, "wt")
	require.NoError(t, os.MkdirAll(worktree, 0o755))

	self := newLiveProbeInstance(t, "self-"+t.Name(), worktree)
	unrelated := newLiveProbeInstance(t, "unrelated-"+t.Name(), worktree+"-other")
	dead := newDeadProbeInstance(t, "dead-"+t.Name(), worktree)

	cases := []struct {
		name        string
		inst        *session.Instance
		excludeUUID string
	}{
		{"target session itself is excluded", self, self.UUID},
		{"live session in a sibling directory does not count as inside", unrelated, "uuid-target"},
		{"dead session inside the worktree does not count", dead, "uuid-target"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			poller := newEmptyPoller()
			poller.SetInstances([]*session.Instance{tc.inst})
			svc := &SessionService{reviewQueuePoller: poller}

			blockingUUID, blocked := svc.OtherLiveSessionInsideWorktree(tc.excludeUUID, worktree)

			assert.False(t, blocked, "must not block; got blockingUUID=%q", blockingUUID)
		})
	}
}

// --------------------------------------------------------------------------
// findConfirmedLiveWorkSession (spawnSessionAfterGates' 8b2 cap)
// --------------------------------------------------------------------------

// TestFindConfirmedLiveWorkSession covers the 8b2 concurrent-liveness cap's
// contract. The "open + confirmed live" case is the one that fails against an
// EndedAt-only staleness check, since that check alone is what the 2026-09-12
// incident showed can be wrong. The "already-tombstoned but confirmed live,
// in mustRecheck" case is the guard's actual reason for existing per its doc
// comment: 8b (findActiveWorkSession) already blocks every EndedAt==nil work
// session regardless of confirmed liveness, so this function only ever runs
// against sessions 8b let through — i.e. ones already marked ended.
// Excluding EndedAt!=nil sessions entirely (the pre-fix behavior) made 8b2
// permanently unreachable in production; see
// TestSpawnSessionFromItem_should_Refuse_When_WronglyTombstonedSessionIsConfirmedLive
// for the end-to-end regression. The "not in mustRecheck" case is the
// performance fix on top of that: killEndedWorkSessionPanes (8a2) already
// confirmed dead every EndedAt!=nil session it doesn't report back in
// mustRecheck, so this must not pay for an extra IsSessionLive call — proven
// here by a stopper that would (wrongly) claim it live if asked.
func TestFindConfirmedLiveWorkSession(t *testing.T) {
	t.Parallel()

	ended := time.Now().Add(-time.Hour)
	open := session.ItemSessionSummary{SessionUUID: "uuid-open-work", Role: session.SessionRoleWork}
	tombstoned := session.ItemSessionSummary{SessionUUID: "uuid-ended-work", Role: session.SessionRoleWork, EndedAt: &ended}
	review := session.ItemSessionSummary{SessionUUID: "uuid-review", Role: session.SessionRoleReview}
	allEndedRecheck := map[string]bool{"uuid-ended-work": true}

	cases := []struct {
		name        string
		stopper     SessionStopper
		prior       []session.ItemSessionSummary
		mustRecheck map[string]bool
		wantUUID    string
	}{
		{
			name:    "nil stopper never blocks",
			stopper: nil,
			prior:   []session.ItemSessionSummary{open},
		},
		{
			name:        "already-tombstoned work session confirmed live still blocks the spawn",
			stopper:     &mockSessionStopper{liveUUIDs: map[string]bool{"uuid-ended-work": true}},
			prior:       []session.ItemSessionSummary{tombstoned},
			mustRecheck: allEndedRecheck,
			wantUUID:    "uuid-ended-work",
		},
		{
			name:        "tombstoned work session the stopper cannot confirm live does not block",
			stopper:     &mockSessionStopper{liveUUIDs: map[string]bool{}},
			prior:       []session.ItemSessionSummary{tombstoned},
			mustRecheck: allEndedRecheck,
		},
		{
			name:    "already-confirmed-dead tombstoned session is not rechecked even if the stopper would (wrongly) call it live",
			stopper: &mockSessionStopper{liveUUIDs: map[string]bool{"uuid-ended-work": true}},
			prior:   []session.ItemSessionSummary{tombstoned},
			// mustRecheck omitted: killEndedWorkSessionPanes already confirmed
			// this one dead, so IsSessionLive must never be consulted for it.
		},
		{
			name:    "non-work role is out of scope",
			stopper: &mockSessionStopper{liveUUIDs: map[string]bool{"uuid-review": true}},
			prior:   []session.ItemSessionSummary{review},
		},
		{
			name:    "open work session the stopper cannot confirm live does not block",
			stopper: &mockSessionStopper{liveUUIDs: map[string]bool{}},
			prior:   []session.ItemSessionSummary{open},
		},
		{
			name:     "open work session confirmed live blocks the spawn regardless of mustRecheck",
			stopper:  &mockSessionStopper{liveUUIDs: map[string]bool{"uuid-open-work": true}},
			prior:    []session.ItemSessionSummary{tombstoned, review, open},
			wantUUID: "uuid-open-work",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := findConfirmedLiveWorkSession(tc.stopper, tc.prior, tc.mustRecheck)
			if tc.wantUUID == "" {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tc.wantUUID, got.SessionUUID)
		})
	}
}
