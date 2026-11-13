package backlog

import (
	"context"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/git"
)

type itemPRFixture struct {
	t       *testing.T
	storage *session.Storage
	repo    *session.EntRepository
	itemID  string
}

func newItemPRFixture(t *testing.T, legacyNumber int) *itemPRFixture {
	t.Helper()
	storage, cleanup := createTestStorage(t)
	t.Cleanup(cleanup)
	item, err := storage.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{
		Title: "item prs", AcceptanceCriteria: `[]`, Priority: 1,
		Status: string(session.BacklogStatusPRPending), RepoPath: "/tmp/fake-repo",
	})
	require.NoError(t, err)
	f := &itemPRFixture{t: t, storage: storage, repo: storage.Repo, itemID: item.ID}
	if legacyNumber > 0 {
		url := "https://github.com/acme/app/pull/" + strconv.Itoa(legacyNumber)
		_, err = storage.UpdateBacklogItem(context.Background(), item.ID, backlogdto.BacklogItemUpdate{PrURL: &url, PrNumber: &legacyNumber}, nil)
		require.NoError(t, err)
	}
	return f
}

func testRef(n int) domain.PRRef {
	ref, err := domain.NewPRRef("github.com", "acme", "app", n)
	if err != nil {
		panic(err)
	}
	return ref
}

func (f *itemPRFixture) session(role string) string {
	f.t.Helper()
	id := uuid.New().String()
	_, err := f.storage.CreateItemSession(context.Background(), backlogdto.ItemSessionData{ItemID: f.itemID, SessionUUID: id, SessionRole: role})
	require.NoError(f.t, err)
	return id
}

func (f *itemPRFixture) link(owner session.PROwner, n int, state domain.PRState) {
	f.t.Helper()
	res, err := f.repo.UpsertSessionPR(context.Background(), owner, testRef(n), session.PRLink{
		Role: domain.PRRolePrimary, Source: domain.AttachSourceManual, Initial: session.PRInitial{State: state, Terminal: state.Terminal()},
	})
	require.NoError(f.t, err)
	require.False(f.t, res.Tombstoned)
}

func (f *itemPRFixture) detach(owner session.PROwner, n int) {
	f.t.Helper()
	_, err := f.repo.DeleteSessionPR(context.Background(), owner, testRef(n))
	require.NoError(f.t, err)
}

func (f *itemPRFixture) itemEnt() *ent.BacklogItem {
	f.t.Helper()
	id, err := uuid.Parse(f.itemID)
	require.NoError(f.t, err)
	it, err := f.repo.Client.BacklogItem.Get(context.Background(), id)
	require.NoError(f.t, err)
	return it
}

func (f *itemPRFixture) mirror() int {
	f.t.Helper()
	return f.itemEnt().PrNumber
}

func TestReconcilePRPending_ShouldSyncMirrorAtEndOfPass_OnlyWhenBacklogReaderOn(t *testing.T) {
	t.Parallel()
	for _, on := range []bool{false, true} {
		on := on
		t.Run(map[bool]string{false: "off", true: "on"}[on], func(t *testing.T) {
			t.Parallel()
			f := newItemPRFixture(t, 21)
			f.link(session.ItemPROwner(f.itemID), 22, domain.PRStateOpen)
			listener := NewBacklogLifecycleListener(f.storage)
			listener.backlogReaderOverride = func() bool { return on }
			overridePRPendingChecker(t, listener, &fakePRPendingChecker{merged: false, status: &git.PRStatus{}})

			listener.ReconcilePRPending(context.Background(), f.repo)

			want := 21
			if on {
				want = 22
			}
			assert.Equal(t, want, f.mirror())
		})
	}
}
