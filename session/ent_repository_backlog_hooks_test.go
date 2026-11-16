package session

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

type hookPublisher struct {
	fn func(*backlogdto.BacklogItemData, backlogdto.BacklogItemChange)
}

func (p hookPublisher) PublishItemChanged(i *backlogdto.BacklogItemData, c backlogdto.BacklogItemChange) {
	p.fn(i, c)
}

type hookChain struct {
	fn func(*backlogdto.BacklogItemData)
}

func (c hookChain) Dispatch(i *backlogdto.BacklogItemData) { c.fn(i) }

// The publisher and chain firer run only after the status change is durable,
// and the chain firer gets exactly one call per done transition.
func TestEntRepository_should_publishAndChainFireAfterCommit_When_TransitionedToDone(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	nextWF := uuid.New() // chain fires only for items with a next workflow
	item, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "t", Status: string(domain.BacklogStatusReview)})
	require.NoError(t, err)
	_, err = repo.UpdateBacklogItem(ctx, item.ID, backlogdto.BacklogItemUpdate{NextWorkflowID: &nextWF}, nil)
	require.NoError(t, err)

	var pubStatus, chainStatus []string
	var chainCalls int
	readStatus := func() string {
		got, gerr := repo.GetBacklogItem(ctx, item.ID)
		require.NoError(t, gerr)
		return got.Status
	}
	repo.SetItemChangePublisher(hookPublisher{func(_ *backlogdto.BacklogItemData, c backlogdto.BacklogItemChange) {
		if c.Kind == backlogdto.ChangeStatusTransition {
			pubStatus = append(pubStatus, readStatus())
		}
	}})
	repo.SetChainFirer(hookChain{func(*backlogdto.BacklogItemData) {
		chainCalls++
		chainStatus = append(chainStatus, readStatus())
	}})

	_, err = repo.RawTransitionBacklogItemStatus(ctx, item.ID, domain.BacklogStatusDone, nil, instance.TriggeredBySystem)
	require.NoError(t, err)

	assert.Equal(t, []string{string(domain.BacklogStatusDone)}, pubStatus)
	assert.Equal(t, 1, chainCalls)
	assert.Equal(t, []string{string(domain.BacklogStatusDone)}, chainStatus)
}

// The chain firer is called only for a done transition of an item that has a
// next workflow and has not already fired its chain.
func TestEntRepository_should_notChainFire_When_GuardNotSatisfied(t *testing.T) {
	t.Parallel()
	nextWF := uuid.New()
	tests := []struct {
		name       string
		to         domain.BacklogStatus
		withNext   bool
		chainFired bool
	}{
		{name: "non_done_transition", to: domain.BacklogStatusInProgress, withNext: true},
		{name: "chain_already_fired", to: domain.BacklogStatusDone, withNext: true, chainFired: true},
		{name: "no_next_workflow", to: domain.BacklogStatusDone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo, cleanup := createTestEntRepository(t)
			defer cleanup()
			ctx := context.Background()

			item, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "t", Status: string(domain.BacklogStatusReview)})
			require.NoError(t, err)
			upd := backlogdto.BacklogItemUpdate{ChainFired: &tc.chainFired}
			if tc.withNext {
				upd.NextWorkflowID = &nextWF
			}
			_, err = repo.UpdateBacklogItem(ctx, item.ID, upd, nil)
			require.NoError(t, err)
			pre, err := repo.GetBacklogItem(ctx, item.ID)
			require.NoError(t, err)
			require.Equal(t, tc.chainFired, pre.ChainFired, "test setup: chain_fired must be persisted")
			require.Equal(t, tc.withNext, pre.NextWorkflowID != nil, "test setup: next_workflow_id must be persisted")

			var chainCalls int
			repo.SetChainFirer(hookChain{func(*backlogdto.BacklogItemData) { chainCalls++ }})
			_, err = repo.RawTransitionBacklogItemStatus(ctx, item.ID, tc.to, nil, instance.TriggeredBySystem)
			require.NoError(t, err)
			assert.Zero(t, chainCalls)
		})
	}
}
