package session

import (
	"context"
	"fmt"
	"testing"

	"github.com/tstapler/stapler-squad/session/internal/fixturetest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestListSessionPRViewsByUUIDs_ShouldStayWithinStatementBudget_WhenManySessions(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	s := &Storage{Repo: repo}
	ctx := context.Background()
	uuids := make([]string, 0, 1001)
	for i := 0; i < 200; i++ {
		uuids = append(uuids, fmt.Sprintf("sess-%03d", i))
		upsertLink(t, repo, SessionPROwner(uuids[i]), fixturetest.MustPRRef(t, "", "acme", "app", i+1), domain.AttachSourceBackfill)
	}

	var got map[string][]domain.PRView
	stmts := countStatements(t, repo, func() {
		var err error
		got, err = s.ListSessionPRViewsByUUIDs(ctx, uuids)
		require.NoError(t, err)
	})
	assert.Len(t, got, 200)
	assert.LessOrEqual(t, stmts, int64(2), "200 uuids: one links query plus one states query")

	for i := 200; i < 1001; i++ {
		uuids = append(uuids, fmt.Sprintf("none-%04d", i))
	}
	stmts = countStatements(t, repo, func() {
		_, err := s.ListSessionPRViewsByUUIDs(ctx, uuids)
		require.NoError(t, err)
	})
	assert.LessOrEqual(t, stmts, int64(6), "1001 uuids: at most 2*ceil(1001/500)")
}

func TestListItemGuardViewsByItemIDs_ShouldReturnDirectLinksWithoutTombstoned(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	s := &Storage{Repo: repo}
	ctx := context.Background()
	item := addLegacyItem(t, repo, "item", "in_progress", 0, "")
	open := fixturetest.MustPRRef(t, "", "acme", "app", 22)
	gone := fixturetest.MustPRRef(t, "", "acme", "app", 23)
	upsertLink(t, repo, ItemPROwner(item), open, domain.AttachSourceReportPRCreated)
	upsertLink(t, repo, ItemPROwner(item), gone, domain.AttachSourceReportPRCreated)
	_, err := repo.DeleteSessionPR(ctx, ItemPROwner(item), gone)
	require.NoError(t, err)

	got, err := s.ListItemGuardViewsByItemIDs(ctx, []string{item})
	require.NoError(t, err)
	require.Len(t, got[item].Direct, 1)
	assert.Equal(t, open, got[item].Direct[0].Ref)
	assert.False(t, got[item].LegacyUnrepresented)
}

func TestListItemGuardViewsByItemIDs_ShouldFlagLegacyUnrepresented_WhenNumberHasNoLink(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	s := &Storage{Repo: repo}
	ctx := context.Background()
	ref := fixturetest.MustPRRef(t, "", "acme", "app", 5)
	unlinked := addLegacyItem(t, repo, "unlinked", "pr_pending", 5, synthesizePRURL(ref))
	badRef := addLegacyItem(t, repo, "bad-ref", "pr_pending", 5, "")
	linked := addLegacyItem(t, repo, "linked", "pr_pending", 5, synthesizePRURL(ref))
	upsertLink(t, repo, ItemPROwner(linked), ref, domain.AttachSourceBackfill)
	tombstoned := addLegacyItem(t, repo, "tombstoned", "pr_pending", 5, synthesizePRURL(ref))
	upsertLink(t, repo, ItemPROwner(tombstoned), ref, domain.AttachSourceBackfill)
	_, err := repo.DeleteSessionPR(ctx, ItemPROwner(tombstoned), ref)
	require.NoError(t, err)
	noNumber := addLegacyItem(t, repo, "no-number", "pr_pending", 0, "")

	got, err := s.ListItemGuardViewsByItemIDs(ctx, []string{unlinked, badRef, linked, tombstoned, noNumber})
	require.NoError(t, err)
	assert.True(t, got[unlinked].LegacyUnrepresented, "number with no link anywhere")
	assert.True(t, got[badRef].LegacyUnrepresented, "unbuildable ref fails closed")
	assert.False(t, got[linked].LegacyUnrepresented)
	assert.False(t, got[tombstoned].LegacyUnrepresented, "a detach tombstone covers the number")
	assert.False(t, got[noNumber].LegacyUnrepresented)
}

func TestListItemGuardViewsByItemIDs_ShouldStayWithinStatementBudget_WhenManyItems(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	s := &Storage{Repo: repo}
	ctx := context.Background()
	ids := make([]string, 0, 200)
	for i := 0; i < 200; i++ {
		id := addLegacyItem(t, repo, fmt.Sprintf("item-%03d", i), "in_progress", 0, "")
		ids = append(ids, id)
		upsertLink(t, repo, ItemPROwner(id), fixturetest.MustPRRef(t, "", "acme", "app", i+1), domain.AttachSourceReportPRCreated)
	}
	stmts := countStatements(t, repo, func() {
		got, err := s.ListItemGuardViewsByItemIDs(ctx, ids)
		require.NoError(t, err)
		assert.Len(t, got, 200)
	})
	assert.LessOrEqual(t, stmts, int64(4), "links, states, tombstones, item rows: constant in the item count")
}

func TestListItemIDsBySessionUUIDs_ShouldReturnEveryItemOfASession(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	s := &Storage{Repo: repo}
	ctx := context.Background()
	x := addLegacyItem(t, repo, "x", "in_progress", 0, "")
	y := addLegacyItem(t, repo, "y", "in_progress", 0, "")
	attachItemSession(t, repo, x, "shared")
	attachItemSession(t, repo, y, "shared")
	attachItemSession(t, repo, x, "solo")

	got, err := s.ListItemIDsBySessionUUIDs(ctx, []string{"shared", "solo", "none"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{x, y}, got["shared"], "all items, not only the newest")
	assert.Equal(t, []string{x}, got["solo"])
	assert.NotContains(t, got, "none")
}

func TestLegacyOnlyPRSafe_ShouldFollowThePersistedTerminalFlag(t *testing.T) {
	t.Parallel()
	assert.True(t, LegacyOnlyPRSafe(domain.InstanceData{GitHubPRNumber: 9, GitHubPRStatusTerminal: true}))
	assert.False(t, LegacyOnlyPRSafe(domain.InstanceData{GitHubPRNumber: 9, GitHubPRStatusTerminal: false}))
}

func TestLegacyNumberRepresented_ShouldMatchRepoAndNumber(t *testing.T) {
	t.Parallel()
	d := domain.InstanceData{GitHubPRNumber: 12, GitHubOwner: "Acme", GitHubRepo: "App", GitHubHost: "github.com"}
	rows := make([]domain.PRView, 0, 2)
	rows = append(rows, domain.PRView{Ref: fixturetest.MustPRRef(t, "", "acme", "app", 11)})
	assert.False(t, LegacyNumberRepresented(d, rows), "links {#11} do not carry legacy #12")
	rows = append(rows, domain.PRView{Ref: fixturetest.MustPRRef(t, "", "acme", "app", 12)})
	assert.True(t, LegacyNumberRepresented(d, rows))
	assert.False(t, LegacyNumberRepresented(domain.InstanceData{GitHubPRNumber: 12}, rows), "unbuildable ref is never represented")
}
