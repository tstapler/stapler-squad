package session

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// fakePRPersister records AttachSessionPR calls; err and res are returned as set.
type fakePRPersister struct {
	mu    sync.Mutex
	calls []fakePRCall
	err   error
	res   PRLinkResult
	// terminalWrites records UpdateGitHubPRStatusTerminal calls; terminalErr is returned from them.
	terminalWrites []terminalWrite
	terminalErr    error
}

type terminalWrite struct {
	title    string
	terminal bool
}

type fakePRCall struct {
	uuid   string
	ref    domain.PRRef
	link   PRLink
	mirror PRMirror
}

func (f *fakePRPersister) AttachSessionPR(_ context.Context, uuid string, ref domain.PRRef, link PRLink, m PRMirror) (PRLinkResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakePRCall{uuid, ref, link, m})
	return f.res, f.err
}

func (f *fakePRPersister) UpdateGitHubPRStatusTerminal(title string, terminal bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.terminalWrites = append(f.terminalWrites, terminalWrite{title, terminal})
	return f.terminalErr
}

func (f *fakePRPersister) PersistPRMirror(string, string, int, bool) error { return nil }

func (f *fakePRPersister) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func newPRTestInstance(t *testing.T) *Instance {
	t.Helper()
	inst := &Instance{
		Title: "multi-pr", UUID: "uuid-multi-pr", Path: t.TempDir(), Program: "echo", Branch: "feat/x",
		GitHubOwner: "acme", GitHubRepo: "app",
	}
	finishInstanceConstruction(inst)
	return inst
}

func attachNumbers(t *testing.T, inst *Instance) []int {
	t.Helper()
	prs := inst.Snapshot().GitHub.PRs
	out := make([]int, 0, len(prs))
	for _, v := range prs {
		out = append(out, v.Ref.Number())
	}
	return out
}

func TestAttachPR_ShouldKeepBoth_WhenTwoGoroutinesAttach(t *testing.T) {
	t.Parallel()
	inst := newPRTestInstance(t)
	r12, r13 := mustPRRef(t, "", "acme", "app", 12), mustPRRef(t, "", "acme", "app", 13)

	var wg sync.WaitGroup
	for _, c := range []struct {
		ref domain.PRRef
		src domain.AttachSource
	}{{r12, domain.AttachSourceBranchDiscovery}, {r13, domain.AttachSourceReportPRCreated}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := inst.AttachPR(c.ref, c.src, PRInitial{}); err != nil {
				t.Errorf("AttachPR(%s): %v", c.ref, err)
			}
		}()
	}
	wg.Wait()

	prs := inst.Snapshot().GitHub.PRs
	if len(prs) != 2 {
		t.Fatalf("PRs = %v, want both #12 and #13", attachNumbers(t, inst))
	}
	if prs[1].AttachedAt.Before(prs[0].AttachedAt) {
		t.Errorf("PRs not in attach order: %v then %v", prs[0].AttachedAt, prs[1].AttachedAt)
	}
	if prs[0].Role != domain.PRRolePrimary || prs[1].Role != domain.PRRoleFollowup {
		t.Errorf("roles = %s, %s; want primary, followup", prs[0].Role, prs[1].Role)
	}
}

func TestAttachPR_ShouldNotMutatePublishedSnapshot_WhenAdding(t *testing.T) {
	t.Parallel()
	inst := newPRTestInstance(t)
	if err := inst.AttachPR(mustPRRef(t, "", "acme", "app", 12), domain.AttachSourceBranchDiscovery, PRInitial{}); err != nil {
		t.Fatal(err)
	}
	earlier := inst.Snapshot()
	if err := inst.AttachPR(mustPRRef(t, "", "acme", "app", 13), domain.AttachSourceBranchDiscovery, PRInitial{}); err != nil {
		t.Fatal(err)
	}
	if got := len(earlier.GitHub.PRs); got != 1 {
		t.Errorf("earlier snapshot has %d PRs after a later attach, want 1", got)
	}
	if got := len(inst.Snapshot().GitHub.PRs); got != 2 {
		t.Errorf("current snapshot has %d PRs, want 2", got)
	}
}

func TestAttachPR_ShouldProjectActivePRToLegacyFields_WhenAnyScopeState(t *testing.T) {
	t.Parallel()
	h := installRecordingHandler(t)
	inst := newPRTestInstance(t)
	r11 := mustPRRef(t, "", "acme", "app", 11)
	if err := inst.AttachPR(r11, domain.AttachSourceBranchDiscovery, PRInitial{State: domain.PRStateOpen}); err != nil {
		t.Fatal(err)
	}
	inst.MarkPRTerminal(r11, domain.PRStateMerged)

	if err := inst.AttachPR(mustPRRef(t, "", "acme", "app", 12), domain.AttachSourceBranchDiscovery, PRInitial{State: domain.PRStateOpen}); err != nil {
		t.Fatal(err)
	}

	g := inst.Snapshot().GitHub
	if g.GitHubPRNumber != 12 || g.GitHubPRURL != "https://github.com/acme/app/pull/12" || g.GitHubPRStatusTerminal {
		t.Errorf("mirror = %d %q terminal=%v, want 12 + url + false", g.GitHubPRNumber, g.GitHubPRURL, g.GitHubPRStatusTerminal)
	}
	if !loggedActivePRChange(h, 11, 12) {
		t.Errorf("no 'session active pr changed' record with from=11 to=12 in %v", h.all())
	}
}

func loggedActivePRChange(h *recordingHandler, from, to int64) bool {
	for _, r := range h.recordsAtLevel(slog.LevelInfo) {
		if r.msg != "session active pr changed" {
			continue
		}
		if intAttr(r, "from") == from && intAttr(r, "to") == to {
			return true
		}
	}
	return false
}

func TestAttachPR_ShouldShowNewestNonTerminal_WhenNoBranchMatch(t *testing.T) {
	t.Parallel()
	inst := newPRTestInstance(t)
	r11, r12 := mustPRRef(t, "", "acme", "app", 11), mustPRRef(t, "", "acme", "app", 12)
	if err := inst.AttachPR(r11, domain.AttachSourceBranchDiscovery, PRInitial{State: domain.PRStateOpen, HeadBranch: "other-a"}); err != nil {
		t.Fatal(err)
	}
	if err := inst.AttachPR(r12, domain.AttachSourceBranchDiscovery, PRInitial{State: domain.PRStateOpen, HeadBranch: "other-b"}); err != nil {
		t.Fatal(err)
	}
	if got := inst.Snapshot().GitHub.GitHubPRNumber; got != 12 {
		t.Errorf("legacy number = %d, want newest non-terminal 12", got)
	}
	if got := attachNumbers(t, inst); len(got) != 2 || got[0] != 11 {
		t.Errorf("#11 link not kept untouched: %v", got)
	}
}

func TestAttachPR_ShouldReturnErrNoSessionUUID_WhenUUIDEmpty(t *testing.T) {
	t.Parallel()
	inst := newPRTestInstance(t)
	inst.UUID = ""
	finishInstanceConstruction(inst)
	err := inst.AttachPR(mustPRRef(t, "", "acme", "app", 12), domain.AttachSourceBranchDiscovery, PRInitial{})
	if !errors.Is(err, ErrNoSessionUUID) {
		t.Fatalf("AttachPR err = %v, want ErrNoSessionUUID", err)
	}
	if len(inst.Snapshot().GitHub.PRs) != 0 {
		t.Error("a link was created for a session without a uuid")
	}

	inst.SetGitHubPRNumber(12) // wrapper falls back to the legacy scalar
	if got := inst.Snapshot().GitHub.GitHubPRNumber; got != 12 {
		t.Errorf("legacy scalar = %d, want 12", got)
	}
	if len(inst.Snapshot().GitHub.PRs) != 0 {
		t.Error("wrapper created a link for a session without a uuid")
	}
}

func TestSetGitHubPR_ShouldAttachThroughAttachPR_WhenRepoAndUUIDResolved(t *testing.T) {
	t.Parallel()
	inst := newPRTestInstance(t)
	inst.SetGitHubPR("https://github.com/acme/app/pull/12", 12)
	if got := attachNumbers(t, inst); len(got) != 1 || got[0] != 12 {
		t.Fatalf("PRs = %v, want [12]", got)
	}
	if inst.Snapshot().GitHub.PRs[0].AttachSource != domain.AttachSourcePRCreationService {
		t.Errorf("source = %s", inst.Snapshot().GitHub.PRs[0].AttachSource)
	}
}

func TestAttachPR_ShouldPersistFromSnapshot_WhenPersisterInjected(t *testing.T) {
	t.Parallel()
	inst := newPRTestInstance(t)
	p := &fakePRPersister{}
	inst.SetPRPersister(p)
	r12 := mustPRRef(t, "", "acme", "app", 12)
	if err := inst.AttachPR(r12, domain.AttachSourceBranchDiscovery, PRInitial{State: domain.PRStateOpen}); err != nil {
		t.Fatal(err)
	}
	if err := inst.AttachPR(r12, domain.AttachSourceBranchDiscovery, PRInitial{State: domain.PRStateOpen}); err != nil {
		t.Fatal(err)
	}
	if p.callCount() != 1 {
		t.Fatalf("persist calls = %d, want 1 (repeat attach is a no-op)", p.callCount())
	}
	c := p.calls[0]
	if c.uuid != "uuid-multi-pr" || c.mirror.Number != 12 || c.link.Role != domain.PRRolePrimary {
		t.Errorf("persist call = %+v", c)
	}
	if inst.PRPersistPending() {
		t.Error("pending set after a successful persist")
	}
}

func TestAttachPR_ShouldMarkPending_WhenSessionRowMissingOrPersistFails(t *testing.T) {
	t.Parallel()
	for name, perr := range map[string]error{
		"row not yet persisted": ErrSessionNotPersisted,
		"database error":        errors.New("disk I/O error"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			inst := newPRTestInstance(t)
			inst.SetPRPersister(&fakePRPersister{err: perr})
			if err := inst.AttachPR(mustPRRef(t, "", "acme", "app", 12), domain.AttachSourceCreatedFromURL, PRInitial{}); err != nil {
				t.Fatalf("AttachPR returned %v; persist failures must not fail the attach", err)
			}
			if !inst.PRPersistPending() {
				t.Error("prPersistPending not set")
			}
			if got := attachNumbers(t, inst); len(got) != 1 {
				t.Errorf("in-memory PRs = %v, want the attach kept", got)
			}
		})
	}
}

func TestAttachPR_ShouldDropLink_WhenRepositoryReportsTombstone(t *testing.T) {
	t.Parallel()
	inst := newPRTestInstance(t)
	inst.SetPRPersister(&fakePRPersister{res: PRLinkResult{Tombstoned: true}})
	if err := inst.AttachPR(mustPRRef(t, "", "acme", "app", 12), domain.AttachSourceBranchDiscovery, PRInitial{}); err != nil {
		t.Fatal(err)
	}
	if got := attachNumbers(t, inst); len(got) != 0 {
		t.Errorf("tombstoned PR still in memory: %v", got)
	}
}

func TestDetachPinApply_ShouldUpdateSetAndFlags(t *testing.T) {
	t.Parallel()
	inst := newPRTestInstance(t)
	r11, r12 := mustPRRef(t, "", "acme", "app", 11), mustPRRef(t, "", "acme", "app", 12)
	_ = inst.AttachPR(r11, domain.AttachSourceBranchDiscovery, PRInitial{State: domain.PRStateOpen})
	_ = inst.AttachPR(r12, domain.AttachSourceBranchDiscovery, PRInitial{State: domain.PRStateOpen})

	if !inst.PinPR(r11, true) || inst.Snapshot().GitHub.GitHubPRNumber != 11 {
		t.Errorf("pin did not make #11 active: %d", inst.Snapshot().GitHub.GitHubPRNumber)
	}
	if !inst.ApplyPRUpdate(r12, PRStatusUpdate{State: "merged", Terminal: true}) {
		t.Fatal("ApplyPRUpdate did not find #12")
	}
	if inst.MarkPRTerminal(r12, domain.PRStateOpen) {
		t.Error("MarkPRTerminal accepted a non-terminal state")
	}
	if !inst.DetachPR(r11) || inst.DetachPR(r11) {
		t.Error("DetachPR should report true once, then false")
	}
	snap := inst.Snapshot().GitHub
	if !snap.HasPRLinks || snap.GitHubPRNumber != 12 || !snap.GitHubPRStatusTerminal {
		t.Errorf("after detach: %+v", snap)
	}
	inst.DetachPR(r12)
	if inst.Snapshot().GitHub.HasPRLinks {
		t.Error("HasPRLinks still true with an empty set")
	}
}

func TestAttachPR_ShouldClearLegacyUnrepresented_WhenAttachRepresentsLegacyNumber(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		attach  int
		wantSet bool
	}{{"unrelated number keeps the flag", 12, true}, {"legacy number clears it", 11, false}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			inst := newPRTestInstance(t)
			inst.GitHubPRNumber, inst.LegacyPRUnrepresented = 11, true
			finishInstanceConstruction(inst)
			_ = inst.AttachPR(mustPRRef(t, "", "acme", "app", tt.attach), domain.AttachSourceBackfill, PRInitial{})
			if got := inst.Snapshot().GitHub.LegacyPRUnrepresented; got != tt.wantSet {
				t.Errorf("LegacyPRUnrepresented = %v, want %v", got, tt.wantSet)
			}
		})
	}
}

func TestDetachPR_ShouldClearLegacyScalars_WhenLastLinkRemoved(t *testing.T) {
	t.Parallel()
	inst := newPRTestInstance(t)
	ref := mustPRRef(t, "", "acme", "app", 12)
	require.NoError(t, inst.AttachPR(ref, domain.AttachSourceManual, PRInitial{State: domain.PRStateOpen, Terminal: true}))
	require.True(t, inst.DetachPR(ref))

	snap := inst.Snapshot().GitHub
	assert.Zero(t, snap.GitHubPRNumber)
	assert.Empty(t, snap.GitHubPRURL)
	assert.False(t, snap.GitHubPRStatusTerminal)
}

func intAttr(r capturedRecord, key string) int64 {
	switch v := r.attrs[key].(type) {
	case int:
		return int64(v)
	case int64:
		return v
	}
	return -1
}

func TestSetGitHubResolution_ShouldAttachCreatedFromURL_WithoutOverwritingHeldPR(t *testing.T) {
	t.Parallel()
	inst := newPRTestInstance(t)
	require.NoError(t, inst.AttachPR(mustPRRef(t, "", "acme", "app", 11), domain.AttachSourceBranchDiscovery, PRInitial{State: domain.PRStateOpen}))

	inst.SetGitHubResolution(GitHubResolution{
		Path: inst.Snapshot().Path, Owner: "acme", Repo: "app",
		PRNumber: 12, PRURL: "https://github.com/acme/app/pull/12",
	})

	prs := inst.Snapshot().GitHub.PRs
	require.Len(t, prs, 2, "the resolved PR joins the held one")
	assert.Equal(t, domain.AttachSourceCreatedFromURL, prs[1].AttachSource)
	assert.Equal(t, domain.PRRoleFollowup, prs[1].Role)
}

func TestNewInstance_ShouldAttachCreatedFromURL_WhenOptionsCarryPR(t *testing.T) {
	t.Parallel()
	inst, err := NewInstance(InstanceOptions{
		Title: "from-pr", Path: t.TempDir(), Program: "echo",
		GitHubOwner: "acme", GitHubRepo: "app",
		GitHubPRNumber: 12, GitHubPRURL: "https://github.com/acme/app/pull/12",
	})
	require.NoError(t, err)

	prs := inst.Snapshot().GitHub.PRs
	require.Len(t, prs, 1)
	assert.Equal(t, domain.AttachSourceCreatedFromURL, prs[0].AttachSource)
	assert.True(t, inst.PRPersistPending(), "no row yet: the first full save writes the link")
	assert.Equal(t, 12, inst.Snapshot().GitHub.GitHubPRNumber)
}

func TestInstanceToSession_ShouldMapActivePR_WhenSessionHoldsSeveral(t *testing.T) {
	t.Parallel()
	inst := newPRTestInstance(t)
	r11 := mustPRRef(t, "", "acme", "app", 11)
	require.NoError(t, inst.AttachPR(r11, domain.AttachSourceBranchDiscovery, PRInitial{State: domain.PRStateOpen}))
	inst.MarkPRTerminal(r11, domain.PRStateMerged)
	require.NoError(t, inst.AttachPR(mustPRRef(t, "", "acme", "app", 12), domain.AttachSourceBranchDiscovery, PRInitial{State: domain.PRStateOpen}))

	s := InstanceToSession(inst)

	require.NotNil(t, s.Git)
	assert.Equal(t, 12, s.Git.PRNumber)
	assert.Equal(t, "https://github.com/acme/app/pull/12", s.Git.PRURL)
}

func TestAttachPR_ShouldObservePRCountThree_WhenThirdPRAttaches(t *testing.T) {
	t.Parallel()
	inst := newPRTestInstance(t)
	before := SessionPRCountObservations(3)
	for _, n := range []int{11, 12, 13} {
		require.NoError(t, inst.AttachPR(mustPRRef(t, "", "acme", "app", n), domain.AttachSourceBranchDiscovery, PRInitial{State: domain.PRStateOpen}))
	}
	assert.GreaterOrEqual(t, SessionPRCountObservations(3), before+1, "third attach observed ssq_session_pr_count=3")
}
