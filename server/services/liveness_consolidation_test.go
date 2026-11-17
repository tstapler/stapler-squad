package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/launchcmd"
	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/session/tymux"
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
func newLiveProbeInstance(t *testing.T, title, paneCWD string) *instance.Instance {
	t.Helper()
	return newProbeInstance(t, title, paneCWD, tmux.NewSessionName(title, tmux.TmuxPrefix).String())
}

// newDeadProbeInstance is newLiveProbeInstance's counterpart whose tmux session
// reports absent, so IsBackendProcessAlive() is false however plausible the
// instance's persisted cwd looks.
func newDeadProbeInstance(t *testing.T, title, paneCWD string) *instance.Instance {
	t.Helper()
	return newProbeInstance(t, title, paneCWD, "")
}

func newProbeInstance(t *testing.T, title, paneCWD, liveSessionName string) *instance.Instance {
	t.Helper()
	inst, err := instance.NewInstance(instance.InstanceOptions{Title: title, Path: t.TempDir()})
	require.NoError(t, err)

	inst.SetTmuxSession(tmux.NewTmuxSessionWithDeps(
		title, "true", &fakePtyFactory{},
		&livenessProbeExecutor{sessionName: liveSessionName, paneCWD: paneCWD},
	))
	return inst
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

	inst := &instance.Instance{Title: "map-hit", UUID: "uuid-map-hit", Path: "/tmp/test", Status: domain.Active}
	poller := svctest.NewEmptyPoller()
	poller.SetInstances([]*instance.Instance{inst})

	svc := &SessionService{ReviewQueuePoller: poller} // concStorage deliberately nil

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

	svc := &SessionService{ReviewQueuePoller: svctest.NewEmptyPoller()}

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

	storage := svctest.CreateTestStorage(t)
	const uuid = "uuid-shadow-dead"
	require.NoError(t, storage.AddInstance(&instance.Instance{
		Title:  "shadow-dead-" + t.Name(),
		UUID:   uuid,
		Path:   t.TempDir(),
		Status: domain.Active,
		// No tmux session by this (test-name-unique) title exists on the
		// per-process isolated socket every tmux call in a `go test` binary
		// resolves to, so the shadow's list-sessions probe answers "absent"
		// without ever reaching the developer machine's real tmux server.
		TmuxPrefix: tmux.TmuxPrefix,
		Program:    "claude",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}))

	svc := &SessionService{storage: storage, concStorage: storage, ReviewQueuePoller: svctest.NewEmptyPoller()}

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
	socket := svctest.StartRealTmuxSession(t, title)

	storage := svctest.CreateTestStorage(t)
	const uuid = "uuid-shadow-alive"
	require.NoError(t, storage.AddInstance(&instance.Instance{
		Title:            title,
		UUID:             uuid,
		Path:             t.TempDir(),
		Status:           domain.Active,
		TmuxServerSocket: socket,
		TmuxPrefix:       tmux.TmuxPrefix,
		Program:          "claude",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}))

	// Poller is empty: the live map says "dead", the OS says "alive".
	svc := &SessionService{storage: storage, concStorage: storage, ReviewQueuePoller: svctest.NewEmptyPoller()}

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
	socket := svctest.StartRealTmuxSession(t, title)

	inst, err := instance.FromInstanceDataDeferred(domain.InstanceData{
		Title:            title,
		UUID:             uuid,
		Path:             t.TempDir(),
		Status:           domain.Active,
		TmuxServerSocket: socket,
		TmuxPrefix:       tmux.TmuxPrefix,
		Program:          "claude",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	})
	require.NoError(t, err)

	poller := svctest.NewEmptyPoller()
	poller.SetInstances([]*instance.Instance{inst})
	svc := &SessionService{ReviewQueuePoller: poller}

	require.True(t, svc.IsSessionLive(uuid), "precondition: instance is genuinely alive in tmux and registered in the poller")

	require.NoError(t, svc.KillTmuxPaneOnly(context.Background(), uuid))

	assert.False(t, svc.IsSessionLive(uuid),
		"after a successful kill, the poller-registered instance must no longer report live — otherwise every later confirmed-liveness check (spawnSessionAfterGates' 8b2) keeps seeing a genuinely-dead session as live forever")
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

	withPoller := &SessionService{ReviewQueuePoller: svctest.NewEmptyPoller()}
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
	siblingCWD := filepath.Join(worktree, "server", "services")
	// CanonicalizeWorktreePath (server/services/session_service.go's
	// OtherLiveSessionInsideWorktree) resolves symlinks via filepath.EvalSymlinks,
	// which silently falls back to filepath.Clean on a nonexistent path -- so a
	// sibling cwd that's never created on disk would skip symlink resolution
	// while worktree (a real t.TempDir()) gets resolved, breaking the prefix
	// match on any host with a symlink hop in its temp dir (macOS: /var ->
	// /private/var). Must exist on disk for this test to actually exercise the
	// canonicalization path it's guarding.
	require.NoError(t, os.MkdirAll(siblingCWD, 0o755))
	sibling := newLiveProbeInstance(t, "live-sibling-"+t.Name(), siblingCWD)
	poller := svctest.NewEmptyPoller()
	poller.SetInstances([]*instance.Instance{sibling})
	svc := &SessionService{ReviewQueuePoller: poller}

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
		inst        *instance.Instance
		excludeUUID string
	}{
		{"target session itself is excluded", self, self.UUID},
		{"live session in a sibling directory does not count as inside", unrelated, "uuid-target"},
		{"dead session inside the worktree does not count", dead, "uuid-target"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			poller := svctest.NewEmptyPoller()
			poller.SetInstances([]*instance.Instance{tc.inst})
			svc := &SessionService{ReviewQueuePoller: poller}

			blockingUUID, blocked := svc.OtherLiveSessionInsideWorktree(tc.excludeUUID, worktree)

			assert.False(t, blocked, "must not block; got blockingUUID=%q", blockingUUID)
		})
	}
}

// TestConversationOwnedByOtherLiveSession_should_ReportOwnership_When_LiveSiblingSharesUUIDAndPath
// is worktree-envvars-hijack validation.md row 29 / Story 1.4.2's SessionService-level
// unit test: unlike OtherLiveSessionInsideWorktree, path overlap alone must never be
// enough to report ownership -- the sibling must also already own this exact
// conversation UUID, so a session detecting its OWN first conversation at a shared
// path is never mistaken for adopting someone else's.
func TestConversationOwnedByOtherLiveSession_should_ReportOwnership_When_LiveSiblingSharesUUIDAndPath(t *testing.T) {
	t.Parallel()

	const conversationUUID = "550e8400-e29b-41d4-a716-446655440000"
	sharedPath := t.TempDir()

	t.Run("no ReviewQueuePoller: nothing can be proven, so nothing may be reported owned", func(t *testing.T) {
		t.Parallel()
		svc := &SessionService{}
		_, owned := svc.ConversationOwnedByOtherLiveSession(conversationOwnershipQuery{selfUUID: "self-uuid", conversationUUID: conversationUUID, path: sharedPath})
		assert.False(t, owned)
	})

	t.Run("live sibling shares UUID and path: reports ownership", func(t *testing.T) {
		t.Parallel()
		sibling := newLiveProbeInstance(t, "owner-"+t.Name(), sharedPath)
		sibling.SetHistoryInfo(conversationUUID, filepath.Join(sharedPath, conversationUUID+".jsonl"))
		poller := svctest.NewEmptyPoller()
		poller.SetInstances([]*instance.Instance{sibling})
		svc := &SessionService{ReviewQueuePoller: poller}

		ownerUUID, owned := svc.ConversationOwnedByOtherLiveSession(conversationOwnershipQuery{selfUUID: "self-uuid", conversationUUID: conversationUUID, path: sharedPath})

		require.True(t, owned, "a live sibling that already owns this exact conversation UUID at this path must be reported")
		assert.Equal(t, sibling.UUID, ownerUUID)
	})

	t.Run("self-exclusion: a session's own record is never reported as another owner", func(t *testing.T) {
		t.Parallel()
		self := newLiveProbeInstance(t, "self-"+t.Name(), sharedPath)
		self.SetHistoryInfo(conversationUUID, filepath.Join(sharedPath, conversationUUID+".jsonl"))
		poller := svctest.NewEmptyPoller()
		poller.SetInstances([]*instance.Instance{self})
		svc := &SessionService{ReviewQueuePoller: poller}

		_, owned := svc.ConversationOwnedByOtherLiveSession(conversationOwnershipQuery{selfUUID: self.UUID, conversationUUID: conversationUUID, path: sharedPath})

		assert.False(t, owned, "cold-restore self-recovery must never be blocked by a session's own record")
	})

	t.Run("path shared but UUID differs: not reported owned (legitimate SessionTypeDirectory path-sharing)", func(t *testing.T) {
		t.Parallel()
		sibling := newLiveProbeInstance(t, "different-uuid-"+t.Name(), sharedPath)
		sibling.SetHistoryInfo("11111111-1111-1111-1111-111111111111", filepath.Join(sharedPath, "other.jsonl"))
		poller := svctest.NewEmptyPoller()
		poller.SetInstances([]*instance.Instance{sibling})
		svc := &SessionService{ReviewQueuePoller: poller}

		_, owned := svc.ConversationOwnedByOtherLiveSession(conversationOwnershipQuery{selfUUID: "self-uuid", conversationUUID: conversationUUID, path: sharedPath})

		assert.False(t, owned, "path overlap alone must never be enough -- the sibling must own this exact UUID")
	})

	t.Run("dead sibling: not reported owned", func(t *testing.T) {
		t.Parallel()
		dead := newDeadProbeInstance(t, "dead-"+t.Name(), sharedPath)
		dead.SetHistoryInfo(conversationUUID, filepath.Join(sharedPath, conversationUUID+".jsonl"))
		poller := svctest.NewEmptyPoller()
		poller.SetInstances([]*instance.Instance{dead})
		svc := &SessionService{ReviewQueuePoller: poller}

		_, owned := svc.ConversationOwnedByOtherLiveSession(conversationOwnershipQuery{selfUUID: "self-uuid", conversationUUID: conversationUUID, path: sharedPath})

		assert.False(t, owned, "a no-longer-live sibling must not block adoption")
	})
}

// TestClaimConversationUUID_should_BlockSecondClaimant_When_FirstClaimIsInFlight
// is the regression test for the TOCTOU race ConversationOwnedByOtherLiveSession
// alone cannot close: two sessions concurrently cold-restoring into the same
// project directory can both run DetectByPath and both see the same "most
// recently modified" candidate as unowned, since neither is IsBackendProcessAlive()
// yet. claimConversationUUID closes that gap with its own atomic claim.
func TestClaimConversationUUID_should_BlockSecondClaimant_When_FirstClaimIsInFlight(t *testing.T) {
	t.Parallel()
	const uuid = "550e8400-e29b-41d4-a716-446655440000"
	svc := &SessionService{}

	ownerUUID, claimed := svc.claimConversationUUID(uuid, "first-instance")
	require.False(t, claimed, "the first claimant must win an uncontested claim")
	assert.Empty(t, ownerUUID)

	ownerUUID, claimed = svc.claimConversationUUID(uuid, "second-instance")
	require.True(t, claimed, "a second instance racing the same candidate while the first claim is in flight must be blocked")
	assert.Equal(t, "first-instance", ownerUUID)
}

// TestClaimConversationUUID_should_AllowReentry_When_SameInstanceClaimsAgain covers
// a retried Start() (e.g. after a transient spawn failure) re-resolving the same
// candidate: the instance must not be blocked by its own earlier claim.
func TestClaimConversationUUID_should_AllowReentry_When_SameInstanceClaimsAgain(t *testing.T) {
	t.Parallel()
	const uuid = "550e8400-e29b-41d4-a716-446655440001"
	svc := &SessionService{}

	_, claimed := svc.claimConversationUUID(uuid, "self-instance")
	require.False(t, claimed)

	_, claimed = svc.claimConversationUUID(uuid, "self-instance")
	assert.False(t, claimed, "re-claiming a UUID this same instance already holds must not block itself")
}

// TestClaimConversationUUID_should_AllowTakeover_When_PriorClaimExpired covers the
// failure mode a permanent claim would create: an instance that claimed a UUID but
// never went alive (Start() errored before spawning) must not strand it forever.
func TestClaimConversationUUID_should_AllowTakeover_When_PriorClaimExpired(t *testing.T) {
	t.Parallel()
	const uuid = "550e8400-e29b-41d4-a716-446655440002"
	svc := &SessionService{}
	svc.inFlightConversationClaims.Store(uuid, conversationClaim{
		instanceUUID: "stale-instance",
		claimedAt:    time.Now().Add(-conversationClaimTTL - time.Second),
	})

	ownerUUID, claimed := svc.claimConversationUUID(uuid, "new-instance")

	assert.False(t, claimed, "an expired claim must not block a new claimant")
	assert.Empty(t, ownerUUID)
}

// TestClaimConversationUUID_should_LetExactlyOneWin_When_ManyGoroutinesRaceTheSameUUID
// is the concurrency-level proof: run under -race, many goroutines claiming the same
// UUID simultaneously must produce exactly one winner, mirroring the real
// concurrent-cold-restore scenario this guards against.
func TestClaimConversationUUID_should_LetExactlyOneWin_When_ManyGoroutinesRaceTheSameUUID(t *testing.T) {
	t.Parallel()
	const uuid = "550e8400-e29b-41d4-a716-446655440003"
	const racers = 50
	svc := &SessionService{}

	var wg sync.WaitGroup
	var wins atomic.Int32
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			<-start
			if _, claimed := svc.claimConversationUUID(uuid, fmt.Sprintf("racer-%d", n)); !claimed {
				wins.Add(1)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	assert.Equal(t, int32(1), wins.Load(), "exactly one goroutine must win the claim")
}

// --------------------------------------------------------------------------
// findConfirmedLiveWorkSession (spawnSessionAfterGates' 8b2 cap)
// --------------------------------------------------------------------------

// fakePtyFactory is a tmux.PtyFactory test double that never forks a real
// subprocess (this sandboxed test environment does not permit it — see
// session_service_test.go's newRealControllerForTest doc comment for the
// same constraint). On success, StartWithSize hands back a genuine PTY pair
// via github.com/creack/pty's Open (which allocates the master/slave devices
// directly rather than forking) and an unstarted *exec.Cmd — safe because
// RestoreWithWorkDir only ever calls cmd.Wait() on it (never cmd.Start()),
// and Wait on a never-started Cmd just returns "exec: not started" instead
// of spawning anything.
type fakePtyFactory struct {
	startErr error

	mu      sync.Mutex
	created []*os.File
}

func (f *fakePtyFactory) Start(cmd *exec.Cmd) (*os.File, *exec.Cmd, error) {
	return f.StartWithSize(cmd, nil)
}

func (f *fakePtyFactory) StartWithSize(_ *exec.Cmd, _ *pty.Winsize) (*os.File, *exec.Cmd, error) {
	if f.startErr != nil {
		return nil, nil, f.startErr
	}
	master, slave, err := pty.Open()
	if err != nil {
		return nil, nil, err
	}
	_ = slave.Close()
	f.mu.Lock()
	f.created = append(f.created, master)
	f.mu.Unlock()
	return master, safeexec.CommandContext(context.Background(), "true"), nil
}

func (f *fakePtyFactory) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, file := range f.created {
		_ = file.Close()
	}
	f.created = nil
}

// storedSessionTymux is a tymux manager for a pane-less view: it has no pane
// (HasSession/IsAlive false) but its record's stored daemon session is live.
type storedSessionTymux struct {
	tymux.TymuxManager // nil; any unlisted method must not be reached
}

func (storedSessionTymux) SetStoredSessionID(string)          {}
func (storedSessionTymux) SetSessionLabel(string)             {}
func (storedSessionTymux) SetCommand(launchcmd.LaunchCommand) {}
func (storedSessionTymux) HasSession() bool                   { return false }
func (storedSessionTymux) IsAlive() bool                      { return false }
func (storedSessionTymux) StoredSessionLive() bool            { return true }
func (storedSessionTymux) PersistedSessionID() string         { return "d-1" }
func (storedSessionTymux) GetSessionIdentifier() string       { return "" }

// A tymux record the poller map lost, whose shadow view has no pane, is still live
// when its stored daemon session is. Probing with IsBackendProcessAlive alone calls
// it dead, which lets backlog spawn a duplicate agent into the worktree.
func TestFindConfirmedLiveInstance_MapMiss_TymuxShadowWithLiveStoredSession_ReturnsInstance(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	session.SetTymuxManagerFactoryForTest(t, func() tymux.TymuxManager { return storedSessionTymux{} })
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	const uuid = "uuid-tymux-stored-live"
	require.NoError(t, repo.Create(context.Background(), domain.InstanceData{
		Title:          "tymux-stored-live",
		UUID:           uuid,
		Path:           t.TempDir(),
		Status:         domain.Active,
		Program:        "claude",
		Backend:        domain.BackendTymux,
		TymuxSessionID: "d-1",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}))
	svc := &SessionService{storage: storage, concStorage: storage, ReviewQueuePoller: svctest.NewEmptyPoller()}

	assert.NotNil(t, svc.findConfirmedLiveInstance(uuid), "a stored daemon session that is live means the session is live")
	assert.True(t, svc.IsSessionLive(uuid))
}
