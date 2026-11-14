package session

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/internal/fixturetest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	entsession "github.com/tstapler/stapler-squad/session/ent/session"
	"github.com/tstapler/stapler-squad/session/ent/sessionpr"
)

type legacySession struct {
	title, uuid, owner, repo, host string
	number                         int
	terminal                       bool
}

func addLegacySession(t *testing.T, repo *EntRepository, s legacySession) {
	t.Helper()
	b := repo.Client.Session.Create().
		SetTitle(s.title).SetPath("/tmp/" + s.title).SetProgram("claude").SetStatus(0).
		SetUUID(s.uuid).SetGithubPrNumber(s.number).SetGithubOwner(s.owner).SetGithubRepo(s.repo).
		SetGithubHost(s.host).SetGithubPrStatusTerminal(s.terminal)
	require.NoError(t, b.Exec(context.Background()))
}

func addLegacyItem(t *testing.T, repo *EntRepository, title, status string, number int, prURL string) string {
	t.Helper()
	it, err := repo.Client.BacklogItem.Create().SetTitle(title).SetPriority(3).SetStatus(status).
		SetPrNumber(number).SetPrURL(prURL).Save(context.Background())
	require.NoError(t, err)
	return it.ID.String()
}

func attachItemSession(t *testing.T, repo *EntRepository, itemID, sessionUUID string) {
	t.Helper()
	_, err := repo.CreateItemSession(context.Background(), backlogdto.ItemSessionData{ItemID: itemID, SessionUUID: sessionUUID, SessionRole: instance.SessionRoleWork})
	require.NoError(t, err)
}

func runBackfill(t *testing.T, repo *EntRepository) PRBackfillReport {
	t.Helper()
	rep, err := repo.runPRBackfill(context.Background(), nil)
	require.NoError(t, err)
	return rep
}

func stateOf(t *testing.T, repo *EntRepository, ref domain.PRRef) *ent.PullRequestState {
	t.Helper()
	return reload(t, repo, ref)
}

func TestBackfill_ShouldSeedFromWhatIsStored_WhenSessionHasLegacyPR(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	addLegacySession(t, repo, legacySession{title: "refactor-auth", uuid: "u-term", owner: "Acme", repo: "App", number: 11, terminal: true})
	addLegacySession(t, repo, legacySession{title: "open-ish", uuid: "u-open", owner: "acme", repo: "app", number: 12})

	rep := runBackfill(t, repo)

	assert.Equal(t, 2, rep.CreatedSessionLinks)
	terminal := stateOf(t, repo, fixturetest.MustPRRef(t, "", "acme", "app", 11))
	assert.Equal(t, "unknown", terminal.State)
	assert.True(t, terminal.Terminal, "terminal flag true seeds an unverified terminal row")
	assert.Equal(t, "https://github.com/acme/app/pull/11", terminal.URL, "empty stored url is synthesized")
	nonTerminal := stateOf(t, repo, fixturetest.MustPRRef(t, "", "acme", "app", 12))
	assert.Equal(t, "unknown", nonTerminal.State)
	assert.False(t, nonTerminal.Terminal, "never a guessed open")

	views, err := repo.ListBySessionUUIDs(ctx, []string{"u-term"})
	require.NoError(t, err)
	require.Len(t, views["u-term"], 1)
	assert.Equal(t, domain.PRRolePrimary, views["u-term"][0].Role)
	assert.Equal(t, domain.AttachSourceBackfill, views["u-term"][0].AttachSource)

	again := runBackfill(t, repo)
	assert.Equal(t, 0, again.CreatedSessionLinks, "a second run creates nothing")
	assert.Equal(t, 2, countLinks(t, repo))
}

func TestBackfill_ShouldMergeFailClosed_WhenSessionAndItemShareAPR(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		itemStatus string
		wantTerm   bool
	}{
		{"terminal session with pr_pending item stays non-terminal", "pr_pending", false},
		{"terminal session with done item is unverified terminal", "done", true},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := newPRRepo(t)
			n := 21 + i
			addLegacySession(t, repo, legacySession{title: "s", uuid: "u1", owner: "acme", repo: "app", number: n, terminal: true})
			addLegacyItem(t, repo, "it", tc.itemStatus, n, fmt.Sprintf("https://github.com/acme/app/pull/%d", n))

			runBackfill(t, repo)

			row := stateOf(t, repo, fixturetest.MustPRRef(t, "", "acme", "app", n))
			assert.Equal(t, "unknown", row.State)
			assert.Equal(t, tc.wantTerm, row.Terminal)
		})
	}
}

func TestMergeBackfillSeeds_ShouldBeIndependentOfOrder(t *testing.T) {
	t.Parallel()
	ref := fixturetest.MustPRRef(t, "", "acme", "app", 21)
	term := prBackfillCandidate{ref: ref, terminal: true}
	open := prBackfillCandidate{ref: ref, terminal: false}
	assert.False(t, mergeBackfillSeeds([]prBackfillCandidate{term}, []prBackfillCandidate{open})[ref])
	assert.False(t, mergeBackfillSeeds([]prBackfillCandidate{open}, []prBackfillCandidate{term})[ref])
	assert.True(t, mergeBackfillSeeds([]prBackfillCandidate{term}, []prBackfillCandidate{term})[ref])
}

func TestBackfill_ShouldNotTopUpState_WhenRowAlreadyExists(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ref := fixturetest.MustPRRef(t, "", "acme", "app", 31)
	seedState(t, repo, ref, domain.PRStateMerged, true)
	addLegacySession(t, repo, legacySession{title: "s", uuid: "u1", owner: "acme", repo: "app", number: 31})

	runBackfill(t, repo)

	assert.Equal(t, "merged", stateOf(t, repo, ref).State, "EnsureState never rewrites an existing row")
	assert.Equal(t, 1, countLinks(t, repo))
}

func TestBackfill_ShouldSkipWithoutGuessing_WhenSessionHasNoOwnerRepoOrUUID(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	addLegacySession(t, repo, legacySession{title: "no-repo", uuid: "u-norepo", number: 9})
	addLegacySession(t, repo, legacySession{title: "no-uuid-1", uuid: "", owner: "acme", repo: "app", number: 14})
	addLegacySession(t, repo, legacySession{title: "no-uuid-2", uuid: "", owner: "acme", repo: "app", number: 14})

	rep := runBackfill(t, repo)

	assert.Equal(t, 1, rep.SkippedNoRepo)
	assert.Equal(t, 2, rep.SkippedNoUUID)
	assert.Equal(t, 0, countLinks(t, repo))
	assert.Equal(t, 0, countStates(t, repo))
	assert.ElementsMatch(t, []string{"no-repo", "no-uuid-1", "no-uuid-2"}, rep.SkippedTitles)
}

func TestBackfill_ShouldNotResurrectOrDuplicate_WhenDetachedOrAlreadyLinked(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	ref11 := fixturetest.MustPRRef(t, "", "acme", "app", 11)
	addLegacySession(t, repo, legacySession{title: "detached", uuid: "u-detached", owner: "acme", repo: "app", number: 11})
	_, err := repo.DeleteSessionPR(ctx, SessionPROwner("u-detached"), fixturetest.MustPRRef(t, "", "Acme", "App", 11))
	require.NoError(t, err)

	addLegacySession(t, repo, legacySession{title: "linked", uuid: "u-linked", owner: "acme", repo: "app", number: 12})
	upsertLink(t, repo, SessionPROwner("u-linked"), fixturetest.MustPRRef(t, "", "acme", "app", 12), domain.AttachSourceManual)

	rep := runBackfill(t, repo)
	assert.Equal(t, 1, rep.SkippedTombstoned)
	assert.Equal(t, 0, rep.CreatedSessionLinks)
	assert.Equal(t, 1, countLinks(t, repo))
	detached, err := repo.ListBySessionUUIDs(ctx, []string{"u-detached"})
	require.NoError(t, err)
	assert.Empty(t, detached["u-detached"])
	_ = ref11

	primaries, err := repo.Client.SessionPR.Query().Where(sessionpr.SessionUUID("u-linked"), sessionpr.Role("primary")).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, primaries)
}

func TestBackfill_ShouldImportAsFollowup_WhenSessionAlreadyHasAPrimary(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	// Rollback case: the session has links (#11 primary) and the old binary wrote #12 to the legacy columns.
	addLegacySession(t, repo, legacySession{title: "rolled-back", uuid: "u1", owner: "acme", repo: "app", number: 12})
	upsertLink(t, repo, SessionPROwner("u1"), fixturetest.MustPRRef(t, "", "acme", "app", 11), domain.AttachSourceManual)

	rep := runBackfill(t, repo)

	assert.Equal(t, 1, rep.ImportedUnrepresented)
	views, err := repo.ListBySessionUUIDs(ctx, []string{"u1"})
	require.NoError(t, err)
	roles := map[int]domain.PRRole{}
	for _, v := range views["u1"] {
		roles[v.Ref.Number()] = v.Role
	}
	assert.Equal(t, map[int]domain.PRRole{11: domain.PRRolePrimary, 12: domain.PRRoleFollowup}, roles)
}

func TestBackfill_ShouldSeedItemsFromStatusWithoutGuessing(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	url := func(n int) string { return fmt.Sprintf("https://github.com/acme/app/pull/%d", n) }
	done := addLegacyItem(t, repo, "done-item", "done", 21, url(21))
	addLegacyItem(t, repo, "pending-item", "pr_pending", 22, url(22))
	addLegacyItem(t, repo, "archived-item", "archived", 30, url(30))

	rep := runBackfill(t, repo)

	assert.Equal(t, 3, rep.CreatedItemLinks)
	links, err := repo.ListByItem(ctx, done)
	require.NoError(t, err)
	require.Len(t, links, 1)
	assert.Equal(t, domain.AttachSourceBackfill, links[0].AttachSource)
	for n, wantTerminal := range map[int]bool{21: true, 22: false, 30: false} {
		row := stateOf(t, repo, fixturetest.MustPRRef(t, "", "acme", "app", n))
		assert.Equal(t, "unknown", row.State, "#%d", n)
		assert.Equal(t, wantTerminal, row.Terminal, "#%d", n)
	}
}

func TestParseItemPRRef_ShouldRejectNonHTTPScheme(t *testing.T) {
	t.Parallel()
	for _, u := range []string{"javascript://github.com/o/r/pull/5", "ftp://github.com/o/r/pull/5", "//github.com/o/r/pull/5"} {
		_, err := ParseItemPRRef(u, 5)
		assert.ErrorIs(t, err, ErrItemBadPRRef, u)
	}
}

func TestParseItemPRRef_ShouldUseURLHostAndRequireMatchingNumber(t *testing.T) {
	t.Parallel()
	ref, err := ParseItemPRRef("https://ghe.acme.io/acme/app/pull/7", 7)
	require.NoError(t, err)
	assert.Equal(t, "ghe.acme.io/acme/app#7", ref.String())
	ref, err = ParseItemPRRef("https://github.com/Acme/App/pull/12", 12)
	require.NoError(t, err)
	assert.Equal(t, fixturetest.MustPRRef(t, "", "acme", "app", 12), ref)

	for name, tc := range map[string]struct {
		url string
		n   int
	}{
		"number mismatch": {"https://ghe.acme.io/acme/app/pull/8", 7},
		"empty url":       {"", 5},
		"not a pull path": {"https://github.com/acme/app/issues/5", 5},
		"no host":         {"acme/app/pull/5", 5},
	} {
		_, err := ParseItemPRRef(tc.url, tc.n)
		assert.ErrorIs(t, err, ErrItemBadPRRef, name)
	}
}

func TestBackfill_ShouldCountPoisonedRowsAndStillImportHealthyOnes(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	addLegacySession(t, repo, legacySession{title: "healthy", uuid: "u-ok", owner: "acme", repo: "app", number: 1})
	addLegacySession(t, repo, legacySession{title: "blank-owner", uuid: "u-bad", owner: "acme", repo: "   ", number: 2})
	addLegacySession(t, repo, legacySession{title: "over-cap", uuid: "u-cap", owner: "acme", repo: "app", number: 900})
	for n := 100; n < 100+domain.MaxSessionPRLinks; n++ {
		_, err := repo.UpsertSessionPR(ctx, SessionPROwner("u-cap"), fixturetest.MustPRRef(t, "", "acme", "app", n), domain.PRLink{Role: domain.PRRoleFollowup, Source: domain.AttachSourceManual})
		require.NoError(t, err)
	}
	addLegacyItem(t, repo, "no-url", "done", 5, "")

	first := runBackfill(t, repo)
	assert.Equal(t, 1, first.CreatedSessionLinks, "the healthy session is imported")
	assert.Equal(t, 1, first.SkippedInvalid, "over the 50-link cap")
	assert.Equal(t, 1, first.SkippedNoRepo, "a blank repo is no repo")
	assert.Equal(t, 1, first.SkippedItemBadRef)
	assert.ElementsMatch(t, []string{"blank-owner", "over-cap", "no-url"}, first.SkippedTitles)

	second := runBackfill(t, repo)
	assert.Equal(t, 0, second.CreatedSessionLinks, "a second boot behaves identically and creates nothing new")
	assert.Equal(t, 1, second.SkippedInvalid)
	assert.Equal(t, 1, second.SkippedItemBadRef)
}

func TestBackfill_ShouldNotRecreateSessionDerivedOrDetachedItemPR(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	url := "https://github.com/acme/app/pull/30"
	ref := fixturetest.MustPRRef(t, "", "acme", "app", 30)

	// #30 reachable only through the item's latest work session: no item-direct link, ever.
	viaSession := addLegacyItem(t, repo, "via-session", "pr_pending", 30, url)
	attachItemSession(t, repo, viaSession, "uuid-work")
	upsertLink(t, repo, SessionPROwner("uuid-work"), ref, domain.AttachSourceBranchDiscovery)
	runBackfill(t, repo)
	runBackfill(t, repo)
	links, err := repo.ListByItem(ctx, viaSession)
	require.NoError(t, err)
	assert.Empty(t, links)

	// After the session link is detached, the legacy scalar still must not re-create it.
	_, err = repo.DeleteSessionPR(ctx, SessionPROwner("uuid-work"), ref)
	require.NoError(t, err)
	runBackfill(t, repo)
	links, err = repo.ListByItem(ctx, viaSession)
	require.NoError(t, err)
	assert.Empty(t, links, "a session tombstone covers the item's scalar")

	// Only the scalar exists (pre-feature or rollback): one item-direct link, created once.
	legacyOnly := addLegacyItem(t, repo, "legacy-only", "pr_pending", 31, "https://github.com/acme/app/pull/31")
	runBackfill(t, repo)
	runBackfill(t, repo)
	links, err = repo.ListByItem(ctx, legacyOnly)
	require.NoError(t, err)
	assert.Len(t, links, 1)
}

func TestBackfill_ShouldResolveOneStateRow_WhenSessionAndItemSpellPRDifferentlyCased(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	addLegacySession(t, repo, legacySession{title: "s", uuid: "u1", owner: "Acme", repo: "App", number: 12})
	itemID := addLegacyItem(t, repo, "it", "pr_pending", 12, "https://github.com/Acme/App/pull/12")
	runBackfill(t, repo)
	assert.Equal(t, 1, countStates(t, repo))
	assert.Equal(t, 2, countLinks(t, repo))

	// A tombstone written for Acme/App#12 blocks the import for a second item.
	other := addLegacyItem(t, repo, "other", "pr_pending", 12, "https://github.com/acme/app/pull/12")
	_, err := repo.DeleteSessionPR(context.Background(), ItemPROwner(other), fixturetest.MustPRRef(t, "", "Acme", "App", 12))
	require.NoError(t, err)
	rep := runBackfill(t, repo)
	assert.Equal(t, 0, rep.CreatedItemLinks)
	links, err := repo.ListByItem(context.Background(), other)
	require.NoError(t, err)
	assert.Empty(t, links)
	_ = itemID
}

// ---- read cost -----------------------------------------------------------------------

func TestBackfill_ShouldReadOnlyCandidateRows_WhenFewSessionsHaveAPR(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	builders := make([]*ent.SessionCreate, 0, 500)
	flush := func() {
		if len(builders) > 0 {
			require.NoError(t, repo.Client.Session.CreateBulk(builders...).Exec(ctx))
			builders = builders[:0]
		}
	}
	for i := 0; i < 10000; i++ {
		b := repo.Client.Session.Create().SetTitle(fmt.Sprintf("bulk-%05d", i)).SetPath("/tmp/x").SetProgram("claude").SetStatus(0).SetUUID(fmt.Sprintf("bulk-u-%05d", i))
		if i%250 == 0 { // 40 sessions with a legacy PR
			b = b.SetGithubPrNumber(i/250 + 1).SetGithubOwner("acme").SetGithubRepo("app")
		}
		builders = append(builders, b)
		if len(builders) == 500 {
			flush()
		}
	}
	flush()

	var cands []prBackfillCandidate
	stmts := countStatements(t, repo, func() {
		var err error
		cands, err = repo.collectSessionPRCandidates(ctx, &PRBackfillReport{})
		require.NoError(t, err)
	})
	assert.Len(t, cands, 40)
	assert.LessOrEqual(t, stmts, int64(3), "one candidate query, one links query, one tombstone query")
}

func TestBackfill_ShouldStayBounded_WhenManyItemsAreAlreadyRepresented(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	const n = 5000
	stateIDs := make([]int, 0, n)
	for start := 0; start < n; start += 500 {
		b := make([]*ent.PullRequestStateCreate, 0, 500)
		for i := start; i < start+500; i++ {
			b = append(b, repo.Client.PullRequestState.Create().SetHost("github.com").SetOwner("acme").SetRepo("app").SetNumber(i+1))
		}
		rows, err := repo.Client.PullRequestState.CreateBulk(b...).Save(ctx)
		require.NoError(t, err)
		for _, r := range rows {
			stateIDs = append(stateIDs, r.ID)
		}
	}
	for start := 0; start < n; start += 500 {
		items := make([]*ent.BacklogItemCreate, 0, 500)
		for i := start; i < start+500; i++ {
			items = append(items, repo.Client.BacklogItem.Create().SetTitle(fmt.Sprintf("it-%d", i)).SetPriority(3).SetStatus("done").
				SetPrNumber(i+1).SetPrURL(fmt.Sprintf("https://github.com/acme/app/pull/%d", i+1)))
		}
		rows, err := repo.Client.BacklogItem.CreateBulk(items...).Save(ctx)
		require.NoError(t, err)
		links := make([]*ent.SessionPRCreate, 0, 500)
		for j, row := range rows {
			links = append(links, repo.Client.SessionPR.Create().SetPullRequestStateID(stateIDs[start+j]).
				SetBacklogItemID(row.ID.String()).SetAttachSource("manual"))
		}
		require.NoError(t, repo.Client.SessionPR.CreateBulk(links...).Exec(ctx))
	}

	var rep PRBackfillReport
	stmts := countStatements(t, repo, func() {
		var err error
		rep, err = repo.runPRBackfill(ctx, nil)
		require.NoError(t, err)
	})
	assert.Zero(t, rep.CreatedItemLinks+rep.CreatedSessionLinks, "everything is already represented")
	const perChunk = 6 // item sessions, item links, session links, 2 tombstone reads, plus slack
	assert.LessOrEqual(t, stmts, int64(perChunk*(n/500)+4))
}

func TestBackfill_ShouldCapWritesPerBoot_AndDeferTheRest(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	const total = maxBackfillWritesPerBoot + 500
	for start := 0; start < total; start += 500 {
		items := make([]*ent.BacklogItemCreate, 0, 500)
		for i := start; i < min(start+500, total); i++ {
			items = append(items, repo.Client.BacklogItem.Create().SetTitle(fmt.Sprintf("cap-%d", i)).SetPriority(3).SetStatus("pr_pending").
				SetPrNumber(i+1).SetPrURL(fmt.Sprintf("https://github.com/acme/app/pull/%d", i+1)))
		}
		require.NoError(t, repo.Client.BacklogItem.CreateBulk(items...).Exec(ctx))
	}

	first := runBackfill(t, repo)
	assert.Equal(t, maxBackfillWritesPerBoot, first.CreatedItemLinks)
	assert.Equal(t, 500, first.Deferred)

	second := runBackfill(t, repo)
	assert.Equal(t, 500, second.CreatedItemLinks, "the next boot imports the deferred rest")
	assert.Zero(t, second.Deferred)
}

// ---- maintenance lifecycle ------------------------------------------------------------------

type prMaintenanceClock struct {
	sleeps []time.Duration
	tick   chan time.Time
}

func (f *prMaintenanceClock) config(hook prBackfillHook) prMaintenanceConfig {
	f.tick = make(chan time.Time)
	return prMaintenanceConfig{
		sleep: func(_ context.Context, d time.Duration) error {
			f.sleeps = append(f.sleeps, d)
			return nil
		},
		newTick:  func(time.Duration) (<-chan time.Time, func()) { return f.tick, func() {} },
		interval: time.Minute,
		hook:     hook,
	}
}

func newMaintenanceStorage(t *testing.T) (*Storage, *EntRepository) {
	t.Helper()
	repo := newPRRepo(t)
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	return storage, repo
}

func TestMaintenance_ShouldRetryThenSwallow_AndImportOnTheNextTick(t *testing.T) {
	storage, repo := newMaintenanceStorage(t)
	ctx := context.Background()
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	addLegacySession(t, repo, legacySession{title: "has-links", uuid: "u1", owner: "acme", repo: "app", number: 12})
	upsertLink(t, repo, SessionPROwner("u1"), fixturetest.MustPRRef(t, "", "acme", "app", 11), domain.AttachSourceManual)
	unverified := fixturetest.MustPRRef(t, "", "acme", "app", 40)
	seedState(t, repo, unverified, domain.PRStateUnknown, true)

	busy := errors.New("database is locked")
	failures := 4
	clock := &prMaintenanceClock{}
	storage.runPRStartupMaintenance(ctx, clock.config(func(domain.PRRef) error {
		if failures > 0 {
			failures--
			return busy
		}
		return nil
	}))

	assert.Equal(t, []time.Duration{time.Second, 4 * time.Second, 16 * time.Second}, clock.sleeps, "three retries with backoff")
	assert.Equal(t, int64(1), storage.PRMaintenanceFailures())
	assert.Equal(t, 1, countLinks(t, repo), "no link from the failing candidate persisted")
	assert.False(t, repo.prVerifyCountersTrusted.Load(), "counters stay untrusted until a pass succeeds")

	clock.tick <- time.Now()        // the 5-minute ticker's next pass
	storage.prMaintenance.wg.Wait() // the ticker stops itself after its first success
	storage.StopPRMaintenance()

	assert.Equal(t, 2, countLinks(t, repo), "the retry imported #12")
	assert.True(t, repo.prVerifyCountersTrusted.Load())
}

func TestMaintenance_ShouldStartNoTicker_WhenFirstPassSucceeds(t *testing.T) {
	storage, repo := newMaintenanceStorage(t)
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	addLegacySession(t, repo, legacySession{title: "s", uuid: "u1", owner: "acme", repo: "app", number: 12})

	clock := &prMaintenanceClock{}
	storage.runPRStartupMaintenance(context.Background(), clock.config(nil))
	storage.StopPRMaintenance()

	assert.Empty(t, clock.sleeps)
	assert.Equal(t, int64(0), storage.PRMaintenanceFailures())
	assert.Equal(t, 1, countLinks(t, repo))
}

func TestMaintenance_ShouldStopTicker_WhenContextCancelled(t *testing.T) {
	storage, repo := newMaintenanceStorage(t)
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	addLegacySession(t, repo, legacySession{title: "s", uuid: "u1", owner: "acme", repo: "app", number: 12})

	ctx, cancel := context.WithCancel(context.Background())
	clock := &prMaintenanceClock{}
	storage.runPRStartupMaintenance(ctx, clock.config(func(domain.PRRef) error { return errors.New("busy") }))
	cancel()
	storage.StopPRMaintenance()
	assert.Equal(t, 0, countLinks(t, repo))
}

func TestMaintenance_ShouldNotCountFailure_WhenShutdownInterruptsBoot(t *testing.T) {
	storage, repo := newMaintenanceStorage(t)
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	addLegacySession(t, repo, legacySession{title: "s", uuid: "u1", owner: "acme", repo: "app", number: 12})

	ctx, cancel := context.WithCancel(context.Background())
	clock := &prMaintenanceClock{}
	storage.runPRStartupMaintenance(ctx, clock.config(func(domain.PRRef) error {
		cancel() // SIGTERM arrives mid-pass
		return errors.New("busy")
	}))
	storage.StopPRMaintenance()

	assert.Equal(t, int64(0), storage.PRMaintenanceFailures(), "an interrupted boot is not a maintenance failure")
	assert.Empty(t, clock.sleeps, "no backoff after cancellation")
}

func TestMaintenance_ShouldResetVerifyCountersOnlyAfterImport(t *testing.T) {
	storage, repo := newMaintenanceStorage(t)
	ref := fixturetest.MustPRRef(t, "", "acme", "app", 40)
	seedState(t, repo, ref, domain.PRStateUnknown, true)
	for i := 0; i < 3; i++ {
		_, err := repo.UpdatePullRequestState(context.Background(), ref, failObs(prRepoT0.Add(time.Duration(i)*time.Minute), PRObservationCountedFailure, "network"))
		require.NoError(t, err)
	}

	require.NoError(t, storage.RunPRStartupMaintenance(context.Background()))
	storage.StopPRMaintenance()

	assert.Equal(t, 0, stateOf(t, repo, ref).PollFailCount)
}

func TestNewEntRepository_ShouldNotRunPRMaintenance(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ref := fixturetest.MustPRRef(t, "", "acme", "app", 40)
	seedState(t, repo, ref, domain.PRStateUnknown, true)
	for i := 0; i < 3; i++ {
		_, err := repo.UpdatePullRequestState(context.Background(), ref, failObs(prRepoT0.Add(time.Duration(i)*time.Minute), PRObservationCountedFailure, "network"))
		require.NoError(t, err)
	}
	addLegacySession(t, repo, legacySession{title: "s", uuid: "u1", owner: "acme", repo: "app", number: 12})

	// A second process (ssq-hooks, a CLI subcommand) opening the same database.
	second, err := NewEntRepository(WithDatabasePath(repo.DbPath))
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })

	assert.Equal(t, 3, stateOf(t, repo, ref).PollFailCount, "counters untouched")
	assert.Equal(t, 0, countLinks(t, repo), "no link imported")
}

// oldBinaryWritesLegacyNumber is what a reverted binary does: it records a PR in
// the legacy columns only and creates no link.
func oldBinaryWritesLegacyNumber(t *testing.T, s *Storage, title string, n int) {
	t.Helper()
	require.NoError(t, s.Repo.Client.Session.Update().Where(entsession.Title(title)).
		SetGithubPrNumber(n).SetGithubPrURL(fmt.Sprintf("https://github.com/acme/app/pull/%d", n)).Exec(context.Background()))
}

func rollbackSequence(t *testing.T, s *Storage, title string) *instance.Instance {
	t.Helper()
	require.NoError(t, s.RunPRStartupMaintenance(context.Background()))
	inst := loadByTitle(t, s, title)
	require.NoError(t, s.UpdateInstance(inst)) // full save
	return inst
}

func TestRollbackRedeploy_ShouldImportBeforeProjection_WhenOldBinaryWroteLegacyOnly(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	prLoadFixture(t, storage, "rolled-back", "u-rb", "acme", "app", 0)
	linkPR(t, storage, "u-rb", 11, domain.PRRolePrimary, time.Now())
	oldBinaryWritesLegacyNumber(t, storage, "rolled-back", 12)
	t.Cleanup(storage.StopPRMaintenance)
	driftBefore := PRMirrorDriftCorrected()

	inst := rollbackSequence(t, storage, "rolled-back")

	links, err := storage.Repo.ListBySessionUUIDs(context.Background(), []string{"u-rb"})
	require.NoError(t, err)
	byNumber := map[int]domain.PRView{}
	for _, v := range links["u-rb"] {
		byNumber[v.Ref.Number()] = v
	}
	require.Contains(t, byNumber, 12, "the legacy-only PR must be imported before projection")
	assert.Equal(t, domain.PRRoleFollowup, byNumber[12].Role, "a session with a primary imports as followup")
	assert.Equal(t, domain.AttachSourceBackfill, byNumber[12].AttachSource)
	assert.Equal(t, domain.PRRolePrimary, byNumber[11].Role)

	snap := inst.Snapshot().GitHub
	require.Len(t, snap.PRs, 2)
	want, ok := instance.ProjectLegacyPRMirror(snap.PRs, inst.Snapshot().Branch)
	require.True(t, ok)
	row := sessionMirror(t, storage, "rolled-back")
	assert.Equal(t, want.Number, row.GitHubPRNumber, "the stored columns read ActivePR of {11,12}")
	assert.Equal(t, want.URL, row.GitHubPRURL)

	corrected, _ := reconcilerFor(storage, []*instance.Instance{inst}).RunOnce(context.Background())
	assert.Zero(t, corrected, "the import is not drift")
	assert.Equal(t, driftBefore, PRMirrorDriftCorrected())

	again := loadByTitle(t, storage, "rolled-back")
	assert.Len(t, again.Snapshot().GitHub.PRs, 2, "a second boot loses nothing")
}

func TestRollbackRedeploy_ShouldNotImportTombstonedValue_AndOverwriteItWithTheProjection(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	prLoadFixture(t, storage, "detached", "u-dt", "acme", "app", 0)
	linkPR(t, storage, "u-dt", 11, domain.PRRolePrimary, time.Now())
	_, err := storage.Repo.DeleteSessionPR(context.Background(), SessionPROwner("u-dt"), fixturetest.MustPRRef(t, "", "acme", "app", 12))
	require.NoError(t, err)
	oldBinaryWritesLegacyNumber(t, storage, "detached", 12)
	t.Cleanup(storage.StopPRMaintenance)

	inst := rollbackSequence(t, storage, "detached")

	require.Len(t, inst.Snapshot().GitHub.PRs, 1, "the tombstoned PR is not imported")
	assert.Equal(t, 11, sessionMirror(t, storage, "detached").GitHubPRNumber, "the projection overwrites it")
}
