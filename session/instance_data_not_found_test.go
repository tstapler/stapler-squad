package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/registry"
)

// *Storage must keep satisfying the registry leaf's persistence port.
var _ registry.Store = (*Storage)(nil)

// Every layer must keep matching the one domain sentinel with errors.Is.
func TestInstanceDataNotFound_should_MatchDomainSentinel_When_IDIsMissing(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	t.Run("storage lookup returns the sentinel", func(t *testing.T) {
		_, err := storage.FindInstanceDataByID("no-such-session")
		assert.ErrorIs(t, err, domain.ErrInstanceDataNotFound)
	})
	t.Run("archive treats the sentinel as not archived", func(t *testing.T) {
		archived, err := storage.ArchiveInstanceDataByID("no-such-session", time.Now())
		require.NoError(t, err)
		assert.False(t, archived)
	})
	t.Run("registry maps the sentinel to ErrSessionNotFound", func(t *testing.T) {
		_, _, err := registry.NewRegistry(storage, nil).Acquire("no-such-session")
		assert.ErrorIs(t, err, registry.ErrSessionNotFound)
	})
	t.Run("ent repository returns the sentinel", func(t *testing.T) {
		t.Parallel()
		repo, repoCleanup := createTestEntRepository(t)
		defer repoCleanup()
		_, err := repo.FindByIDWithOptions(context.Background(), "no-such-session", domain.LoadMinimal)
		assert.ErrorIs(t, err, domain.ErrInstanceDataNotFound)
		_, err = repo.FindByIDWithOptions(context.Background(), "", domain.LoadMinimal)
		assert.ErrorIs(t, err, domain.ErrInstanceDataNotFound, "empty id takes the early-return branch")
	})
}
