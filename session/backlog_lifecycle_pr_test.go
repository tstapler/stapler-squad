package session

import (
	"context"
	"errors"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/git"
)

// TestBacklogItemLink locks in the deep-link shape pushAndCreatePR and
// buildFallbackPRBody rely on to point a reviewer at the backlog item's
// detail view (web-app's `/backlog?item=` route).
func TestBacklogItemLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		itemID  string
		want    string
	}{
		{
			name:    "plain base URL and UUID",
			baseURL: "http://localhost:8543",
			itemID:  "b608ab1e-b86e-4130-8879-7328cd363063",
			want:    "http://localhost:8543/backlog?item=b608ab1e-b86e-4130-8879-7328cd363063",
		},
		{
			name:    "empty base URL",
			baseURL: "",
			itemID:  "b608ab1e-b86e-4130-8879-7328cd363063",
			want:    "/backlog?item=b608ab1e-b86e-4130-8879-7328cd363063",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, backlogItemLink(tt.baseURL, tt.itemID))
		})
	}
}

// TestBuildFallbackPRBody covers buildFallbackPRBody's pure composition of a
// PR body from the backlog item's own data: the item's description, a
// clickable deep link back to the backlog item, and an optional "## Test
// plan" checklist derived from acceptance criteria.
func TestBuildFallbackPRBody(t *testing.T) {
	t.Parallel()

	origin := PRFooterOrigin{BaseURL: "https://ssq.example.com", HostID: testHostID, Hostname: "onyx.lan"}
	const itemID = "b608ab1e-b86e-4130-8879-7328cd363063"
	wantLink := "Backlog item: https://ssq.example.com/backlog?item=b608ab1e-b86e-4130-8879-7328cd363063"

	tests := []struct {
		name           string
		item           *BacklogItemData
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "no acceptance criteria — no test plan section",
			item: &BacklogItemData{
				ID:          itemID,
				Description: "Fixes the flaky retry loop in the push remediation path.",
			},
			wantContains: []string{
				"## Summary",
				"Fixes the flaky retry loop in the push remediation path.",
				wantLink,
			},
			wantNotContain: []string{"## Test plan"},
		},
		{
			name: "acceptance criteria present — test plan checklist with mixed statuses",
			item: &BacklogItemData{
				ID:          itemID,
				Description: "Adds a deep link to the fallback PR body.",
				AcceptanceCriteria: mustSerializeAcCriteria(t, []domain.AcCriterion{
					{Index: 0, Text: "Body contains the backlog item link", Status: domain.AcStatusDone},
					{Index: 1, Text: "Body contains the description", Status: domain.AcStatusPending},
				}),
			},
			wantContains: []string{
				"## Summary",
				"Adds a deep link to the fallback PR body.",
				wantLink,
				"## Test plan",
				"- [x] Body contains the backlog item link",
				"- [ ] Body contains the description",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildFallbackPRBody(tt.item, origin)
			for _, want := range tt.wantContains {
				require.Contains(t, got, want)
			}
			for _, notWant := range tt.wantNotContain {
				require.NotContains(t, got, notWant)
			}
		})
	}
}

// TestBuildFallbackPRBody_SanitizesDescription confirms the description is
// routed through sanitizeField (HTML stripped, long text truncated) rather
// than dropped into the body verbatim.
func TestBuildFallbackPRBody_SanitizesDescription(t *testing.T) {
	t.Parallel()

	item := &BacklogItemData{
		ID:          "b608ab1e-b86e-4130-8879-7328cd363063",
		Description: "<script>alert(1)</script>" + strings.Repeat("a", 2000),
	}

	got := buildFallbackPRBody(item, PRFooterOrigin{BaseURL: "https://ssq.example.com"})

	require.NotContains(t, got, "<script>")
	require.Contains(t, got, "[truncated]")
}

var testHostID = func() HostID {
	id, err := ParseHostID("host_01ARZ3NDEKTSV4RRFFQ69G5FAV")
	if err != nil {
		panic(err)
	}
	return id
}()

// TestPRFooter_AlwaysIdentifiesItemAndHost covers the footer's purpose: from
// any machine the owner can tell which instance and item a PR came from, with
// a clickable https link only when a non-loopback address is known.
func TestPRFooter_AlwaysIdentifiesItemAndHost(t *testing.T) {
	t.Parallel()

	item := &BacklogItemData{ID: "b608ab1e-b86e-4130-8879-7328cd363063"}
	tests := []struct {
		name         string
		origin       PRFooterOrigin
		wantContains []string
		wantLink     bool
	}{
		{
			name:   "link, hostname and host ID",
			origin: PRFooterOrigin{BaseURL: "https://onyx.lan:8444", HostID: testHostID, Hostname: "onyx.lan"},
			wantContains: []string{
				"Backlog item: https://onyx.lan:8444/backlog?item=" + item.ID,
				"ssq://onyx.lan/backlog/v1/" + item.ID,
				testHostID.String(),
			},
			wantLink: true,
		},
		{
			name:   "no base URL falls back to text-only with host ID in the ssq link",
			origin: PRFooterOrigin{HostID: testHostID},
			wantContains: []string{
				"Backlog item: " + item.ID,
				"ssq://" + testHostID.String() + "/backlog/v1/" + item.ID,
				testHostID.String(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := prFooter(tt.origin, item)
			for _, want := range tt.wantContains {
				require.Contains(t, got, want)
			}
			require.Equal(t, tt.wantLink, strings.Contains(got, "https://"))
			require.NotContains(t, got, "127.0.0.1")
			require.NotContains(t, got, "localhost")

			prov, ok := ParsePRProvenanceComment(got)
			require.True(t, ok, "footer must be readable by ParsePRProvenanceComment")
			require.Equal(t, testHostID, prov.HostID)
			require.Equal(t, item.ID, prov.ItemID)
		})
	}
}

func TestPRFooter_WithoutHostStillNamesItem(t *testing.T) {
	t.Parallel()

	got := prFooter(PRFooterOrigin{}, &BacklogItemData{ID: "abc"})
	require.Equal(t, "Backlog item: abc\n", got)
}

func TestBuildFallbackPRBody_TextOnlyFooter_WhenNoBaseURL(t *testing.T) {
	t.Parallel()

	item := &BacklogItemData{ID: "b608ab1e-b86e-4130-8879-7328cd363063", Description: "Fixes the thing."}
	got := buildFallbackPRBody(item, PRFooterOrigin{HostID: testHostID})

	require.Contains(t, got, "Backlog item: "+item.ID)
	require.Contains(t, got, testHostID.String())
	require.NotContains(t, got, "/backlog?item=")
	require.NotContains(t, got, "http")
}

func TestAppendBacklogFooter(t *testing.T) {
	t.Parallel()

	item := &BacklogItemData{ID: "b608ab1e-b86e-4130-8879-7328cd363063"}
	origin := PRFooterOrigin{BaseURL: "https://ssq.example.com", HostID: testHostID, Hostname: "onyx.lan"}
	got := appendBacklogFooter("## Summary\n\nBody text.\n\n", origin, item)

	require.True(t, strings.HasPrefix(got, "## Summary\n\nBody text.\n\nBacklog item: https://ssq.example.com/backlog?item="+item.ID))
	require.Contains(t, got, testHostID.String())
}

func TestNonLoopbackBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, in, want string }{
		{"public https", "https://ssq.example.com", "https://ssq.example.com"},
		{"trailing slash trimmed", "https://ssq.example.com/", "https://ssq.example.com"},
		{"lan hostname with port", "https://onyx.lan:8444", "https://onyx.lan:8444"},
		{"lan IP", "http://192.168.1.5:8543", "http://192.168.1.5:8543"},
		{"empty", "", ""},
		{"localhost", "http://localhost:8543", ""},
		{"loopback v4", "http://127.0.0.1:8543", ""},
		{"loopback v6", "http://[::1]:8543", ""},
		{"unspecified", "http://0.0.0.0:8543", ""},
		{"no scheme", "ssq.example.com", ""},
		{"custom scheme", "ssq://host/backlog", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, NonLoopbackBaseURL(tt.in))
		})
	}
}

func TestListenerPRFooterOrigin(t *testing.T) {
	t.Parallel()

	l := &BacklogLifecycleListener{}
	require.Equal(t, PRFooterOrigin{}, l.getPRFooterOrigin(), "unset fns must yield an empty origin")

	l.SetDashboardBaseURLFn(func() string { return "http://127.0.0.1:8543" })
	require.Equal(t, "", l.getPRFooterOrigin().BaseURL, "loopback must be dropped")

	l.SetDashboardBaseURLFn(func() string { return "https://ssq.example.com" })
	l.SetHostRefFn(func() (HostID, string) { return testHostID, "onyx.lan" })
	require.Equal(t, PRFooterOrigin{BaseURL: "https://ssq.example.com", HostID: testHostID, Hostname: "onyx.lan"}, l.getPRFooterOrigin())
}

func mustSerializeAcCriteria(t *testing.T, criteria []domain.AcCriterion) domain.AcCriteriaJSON {
	t.Helper()
	serialized, err := domain.SerializeAcCriteria(criteria)
	require.NoError(t, err)
	return serialized
}

// failingCIListener wires a listener over a still-CI-failing PR and returns it
// with its stamped-capable notifier and fix spawner.
func failingCIListener(t *testing.T, storage *Storage, prNumber int) (*BacklogLifecycleListener, *autoRemediatingNotifier, *BacklogItemData) {
	t.Helper()
	item := newPRPendingTestItem(t, storage, prNumber)
	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		status: &git.PRStatus{CIFailing: true, FeedbackText: "## Failing CI checks\n- build FAILED\n"},
	})
	listener.SetPRFixSpawner(&fakePRFixSpawner{})
	n := &autoRemediatingNotifier{}
	listener.SetNotifier(n)
	return listener, n, item
}

// T-AR-01: the first sighting of a PR that needs a fix publishes the WARNING
// through the auto-remediating (stamped) path, exactly once.
func TestPRNeedsAttention_ShouldStampAutoRemediating_WhenWarningPublishedAtBackoffGate(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	listener, n, _ := failingCIListener(t, storage, 9301)

	listener.ReconcilePRPending(context.Background(), storage.repo)
	listener.ReconcilePRPending(context.Background(), storage.repo)

	assert.Equal(t, []string{"PR needs attention"}, n.stamped, "one stamped WARNING on first sighting, none on the next tick")
	assert.Empty(t, n.titles(), "the automation-is-acting WARNING must not also go out unstamped")
}

// T-AR-02: when the automation gives up (the fifth attempt parks the row), the
// escalation is a plain, UNSTAMPED notification, so it counts as a pending
// decision and the stamped WARNING never masks it.
func TestPRNeedsAttention_ShouldHaveUnstampedEscalation_WhenAutomationFails(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	listener, n, item := failingCIListener(t, storage, 9302)
	er := storage.repo

	for attempt := 1; attempt <= 5; attempt++ {
		listener.ReconcilePRPending(context.Background(), er)
		backdateNextRemediationAt(t, er, item.ID, domain.StuckReasonPRNeedsFix, time.Now().Add(-time.Second))
	}

	assert.Equal(t, []string{"PR needs attention"}, n.stamped)
	require.Equal(t, []string{"Auto-rework paused"}, n.titles(), "the dead-end escalation goes through plain Notify")
	assert.True(t, n.calls[0].Urgent, "the escalation is urgent")
	assert.EqualValues(t, 8, n.calls[0].NotificationType)
}

// newPreflightFixture is newPushAndCreatePRTestFixture with a real git repo
// whose origin remote resolves to tstapler/stapler-squad.
func newPreflightFixture(t *testing.T, storage *Storage) (*BacklogItemData, ItemSessionSummary) {
	t.Helper()
	ctx := context.Background()
	repoDir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"remote", "add", "origin", "https://github.com/tstapler/stapler-squad.git"},
	} {
		cmd := safeexec.CommandContext(ctx, "git", args...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title: "Preflight item", AcceptanceCriteria: `[]`, Priority: 1,
		Status: string(BacklogStatusReview), RepoPath: repoDir,
	})
	require.NoError(t, err)
	sessionUUID := uuid.New().String()
	is, err := storage.CreateItemSession(ctx, ItemSessionData{ItemID: item.ID, SessionUUID: sessionUUID, SessionRole: SessionRoleWork})
	require.NoError(t, err)
	inst := newTestInstance("preflight-" + sessionUUID[:8])
	inst.UUID = sessionUUID
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(repoDir, repoDir+"/../wt", "preflight-test", "backlog/preflight-test", "abc123")
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))
	return item, ItemSessionSummary{ID: is.ID, SessionUUID: sessionUUID, BacklogItemID: item.ID}
}

type preflightHarness struct {
	listener *BacklogLifecycleListener
	notifier *fakeNotifier
	creator  *fakePRCreator
	now      time.Time
	fetches  int32
}

func newPreflightHarness(storage *Storage, cached func() (github.RepoSettingsResult, bool)) *preflightHarness {
	h := &preflightHarness{
		listener: NewBacklogLifecycleListener(storage),
		notifier: &fakeNotifier{},
		creator:  &fakePRCreator{createURL: "https://github.com/tstapler/stapler-squad/pull/1", createNumber: 1},
		now:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	h.listener.SetNotifier(h.notifier)
	h.listener.SetPRCreatorFactory(func(_, _, _, _, _ string) prCreator { return h.creator })
	h.listener.preflightNowFn = func() time.Time { return h.now }
	h.listener.SetRepoSettingsSeam(RepoSettingsSeam{
		Cached: func(github.RepoRef) (github.RepoSettingsResult, bool) { return cached() },
		Fetch: func(ctx context.Context, _ github.RepoRef) github.RepoSettingsResult {
			if github.GitHubCallOriginFrom(ctx) == github.OriginBacklogSync {
				atomic.AddInt32(&h.fetches, 1)
			}
			return github.RepoSettingsResult{}
		},
	})
	return h
}

func (h *preflightHarness) infoCalls() []fakeNotifierCall {
	var out []fakeNotifierCall
	for _, c := range h.notifier.calls {
		if c.NotificationType == autoMergePreflightType {
			out = append(out, c)
		}
	}
	return out
}

// autoMergeOff is a cached OK result whose allow_auto_merge is false.
var autoMergeOff = github.RepoSettingsResult{Status: github.RepoSettingsOK}

func TestPushAndCreatePR_Preflight_OncePerRepoPer24h(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	h := newPreflightHarness(storage, func() (github.RepoSettingsResult, bool) { return autoMergeOff, true })

	var firstItemID string
	for i := 0; i < 5; i++ {
		item, is := newPreflightFixture(t, storage)
		if i == 0 {
			firstItemID = item.ID
		}
		h.creator.createCalled = false
		h.listener.pushAndCreatePR(context.Background(), item, is)
		require.True(t, h.creator.createCalled, "PR creation is never blocked")
	}
	info := h.infoCalls()
	require.Len(t, info, 1, "exactly one preflight in 24h")
	c := info[0]
	assert.False(t, c.Urgent)
	assert.False(t, c.Important)
	assert.Equal(t, firstItemID, c.RecipientID)
	assert.Contains(t, c.Message, "tstapler/stapler-squad")
	assert.Contains(t, c.Message, "PR will still be created")
	assert.NotContains(t, strings.ToLower(c.Message), "unknown")
	assert.NotContains(t, c.Message, "allow_auto_merge")

	key := "automerge-off:github.com/tstapler/stapler-squad"
	assert.False(t, h.listener.claimPreflight(key), "still inside the window")
	h.now = h.now.Add(25 * time.Hour)
	assert.True(t, h.listener.claimPreflight(key), "claim reopens after 24h")
}

func TestPushAndCreatePR_Preflight_SuppressesAutoMergeFailureNoticeForThatPR(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	h := newPreflightHarness(storage, func() (github.RepoSettingsResult, bool) { return autoMergeOff, true })
	h.creator.autoMergeErr = errors.New("auto-merge is not enabled")

	item, is := newPreflightFixture(t, storage)
	h.listener.pushAndCreatePR(context.Background(), item, is)
	assert.Len(t, h.infoCalls(), 1)
	assert.NotContains(t, h.notifier.titles(), "Auto-merge not enabled", "one message for this PR")

	item2, is2 := newPreflightFixture(t, storage)
	h.listener.pushAndCreatePR(context.Background(), item2, is2)
	assert.Len(t, h.infoCalls(), 1, "preflight deduped for the repo")
	assert.Contains(t, h.notifier.titles(), "Auto-merge not enabled", "existing notice is the single message for the second PR")
}

func TestPushAndCreatePR_Preflight_ColdCacheWarmsInBackgroundAndSendsNothing(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	h := newPreflightHarness(storage, func() (github.RepoSettingsResult, bool) { return github.RepoSettingsResult{}, false })

	item, is := newPreflightFixture(t, storage)
	h.listener.pushAndCreatePR(context.Background(), item, is)
	h.listener.preflightWarmWG.Wait()
	assert.EqualValues(t, 1, atomic.LoadInt32(&h.fetches), "one warm-up carrying the background call origin")
	assert.True(t, h.creator.createCalled)
	assert.Empty(t, h.infoCalls())
}

func TestPushAndCreatePR_Preflight_SilentWhenUndetermined(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	h := newPreflightHarness(storage, func() (github.RepoSettingsResult, bool) {
		return github.RepoSettingsResult{Status: github.RepoSettingsError, Reason: "timeout"}, true
	})
	item, is := newPreflightFixture(t, storage)
	h.listener.pushAndCreatePR(context.Background(), item, is)
	assert.Empty(t, h.infoCalls(), "failure to determine the setting sends nothing")
}
