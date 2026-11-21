package session

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// fakeItemLinkStore is the second ItemLinkStore implementation. It records the
// arguments of each lookup and returns canned results, so the tests reach the
// store-error branches real Ent cannot produce.
type fakeItemLinkStore struct {
	linkRes   backlogdto.ItemSessionSummary
	linkErr   error
	itemErr   error
	priorRes  backlogdto.ItemSessionSummary
	priorErr  error
	linkCalls [][2]string
	itemCalls []string
	prior     []string
}

func (f *fakeItemLinkStore) GetItemSessionBySessionAndItem(_ context.Context, sessionUUID, itemID string) (backlogdto.ItemSessionSummary, error) {
	f.linkCalls = append(f.linkCalls, [2]string{sessionUUID, itemID})
	return f.linkRes, f.linkErr
}

func (f *fakeItemLinkStore) GetBacklogItem(_ context.Context, id string) (*backlogdto.BacklogItemData, error) {
	f.itemCalls = append(f.itemCalls, id)
	return &backlogdto.BacklogItemData{ID: id}, f.itemErr
}

func (f *fakeItemLinkStore) GetItemSessionBySessionUUID(_ context.Context, sessionUUID string) (backlogdto.ItemSessionSummary, error) {
	f.prior = append(f.prior, sessionUUID)
	return f.priorRes, f.priorErr
}

func TestResolveItemLink_should_ReturnLinkAndSkipDisambiguation_When_LinkLookupSucceeds(t *testing.T) {
	t.Parallel()
	f := &fakeItemLinkStore{linkRes: backlogdto.ItemSessionSummary{BacklogItemID: "item-1"}}
	got, linkErr := ResolveItemLink(context.Background(), f, "caller", "item-1")
	require.Nil(t, linkErr)
	assert.Equal(t, "item-1", got.BacklogItemID)
	assert.Equal(t, [][2]string{{"caller", "item-1"}}, f.linkCalls)
	assert.Empty(t, f.itemCalls)
	assert.Empty(t, f.prior)
}

func TestResolveItemLink_should_ReturnInternalError_When_LinkLookupFailsWithNonNotFound(t *testing.T) {
	t.Parallel()
	f := &fakeItemLinkStore{linkErr: errors.New("db down")}
	_, linkErr := ResolveItemLink(context.Background(), f, "caller", "item-1")
	require.NotNil(t, linkErr)
	assert.Equal(t, ItemLinkInternalError, linkErr.Code)
	assert.Contains(t, linkErr.Message, "link check failed: db down")
	assert.Empty(t, f.itemCalls, "a non-NotFound link error must not be disambiguated")
}

func TestResolveItemLink_should_ReturnInternalError_When_ItemLookupFailsWithNonNotFound(t *testing.T) {
	t.Parallel()
	f := &fakeItemLinkStore{linkErr: domain.ErrNotFound, itemErr: errors.New("item boom")}
	_, linkErr := ResolveItemLink(context.Background(), f, "caller", "item-1")
	require.NotNil(t, linkErr)
	assert.Equal(t, ItemLinkInternalError, linkErr.Code)
	assert.Contains(t, linkErr.Message, "get backlog item: item boom")
	assert.Equal(t, []string{"item-1"}, f.itemCalls)
}

func TestResolveItemLink_should_ReturnNotFound_When_ItemLookupReportsNotFound(t *testing.T) {
	t.Parallel()
	f := &fakeItemLinkStore{linkErr: domain.ErrNotFound, itemErr: domain.ErrNotFound}
	_, linkErr := ResolveItemLink(context.Background(), f, "caller", "item-1")
	require.NotNil(t, linkErr)
	assert.Equal(t, ItemLinkNotFound, linkErr.Code)
	assert.Equal(t, ItemNotFoundRemediation, linkErr.Remediation)
	assert.Empty(t, f.prior)
}

func TestResolveItemLink_should_NameThePriorItem_When_CallerIsLinkedToADifferentItem(t *testing.T) {
	t.Parallel()
	f := &fakeItemLinkStore{linkErr: domain.ErrNotFound, priorRes: backlogdto.ItemSessionSummary{BacklogItemID: "other"}}
	_, linkErr := ResolveItemLink(context.Background(), f, "caller", "item-1")
	require.NotNil(t, linkErr)
	assert.Equal(t, ItemLinkPermissionDenied, linkErr.Code)
	assert.Contains(t, linkErr.Remediation, "currently linked to a different item (other)")
	assert.Equal(t, []string{"caller"}, f.prior)
}

func TestResolveItemLink_should_UsePlainRelinkAdvice_When_PriorLinkIsSameEmptyOrFailed(t *testing.T) {
	t.Parallel()
	cases := map[string]*fakeItemLinkStore{
		"same item":   {linkErr: domain.ErrNotFound, priorRes: backlogdto.ItemSessionSummary{BacklogItemID: "item-1"}},
		"empty prior": {linkErr: domain.ErrNotFound},
		"prior error": {linkErr: domain.ErrNotFound, priorErr: errors.New("x"), priorRes: backlogdto.ItemSessionSummary{BacklogItemID: "other"}},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, linkErr := ResolveItemLink(context.Background(), f, "caller", "item-1")
			require.NotNil(t, linkErr)
			assert.Equal(t, ItemLinkPermissionDenied, linkErr.Code)
			assert.NotContains(t, linkErr.Remediation, "different item")
			assert.Contains(t, linkErr.Remediation, "link_session_to_item with item_id=item-1")
		})
	}
}
