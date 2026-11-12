package insights

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/tokens"
)

type stampingBacklogReader struct {
	svctest.FakeBacklogReader
	stamped map[string]string // item session ID -> conversation UUID
}

func (f *stampingBacklogReader) UpdateItemSessionConversationUUID(_ context.Context, id, conv string) error {
	f.stamped[id] = conv
	return nil
}

func TestBackfillConversationUUIDs(t *testing.T) {
	t.Parallel()
	created := time.Now().UTC().Add(-time.Hour)
	entry := func(id string, at time.Time) backlogdto.ItemSessionBacklogEntry {
		return backlogdto.ItemSessionBacklogEntry{ItemSessionID: id, SessionUUID: uuid.NewString(), SessionRole: session.SessionRoleWork, CreatedAt: at}
	}
	wt := "/home/u/.stapler-squad/worktrees/x"

	tests := []struct {
		name    string
		entries []backlogdto.ItemSessionBacklogEntry
		results []*tokens.ParseResult
		want    map[string]string
	}{
		{"unique match within window", []backlogdto.ItemSessionBacklogEntry{entry("is-1", created)},
			[]*tokens.ParseResult{svctest.NewResult("conv-1", "claude-sonnet-4", wt, 1, 1, 0, created.Add(4*time.Second))},
			map[string]string{"is-1": "conv-1"}},
		{"ambiguous: two candidates in window", []backlogdto.ItemSessionBacklogEntry{entry("is-1", created), entry("is-2", created.Add(time.Minute))},
			[]*tokens.ParseResult{svctest.NewResult("conv-1", "claude-sonnet-4", wt, 1, 1, 0, created.Add(90*time.Second))},
			map[string]string{}},
		{"transcript before the item session", []backlogdto.ItemSessionBacklogEntry{entry("is-1", created)},
			[]*tokens.ParseResult{svctest.NewResult("conv-1", "claude-sonnet-4", wt, 1, 1, 0, created.Add(-2*time.Minute))},
			map[string]string{}},
		{"not a worktree transcript", []backlogdto.ItemSessionBacklogEntry{entry("is-1", created)},
			[]*tokens.ParseResult{svctest.NewResult("conv-1", "claude-sonnet-4", "/home/u/code/kibitzer", 1, 1, 0, created.Add(time.Second))},
			map[string]string{}},
		{"synthetic session uuid", []backlogdto.ItemSessionBacklogEntry{{ItemSessionID: "is-1", SessionUUID: "headless-triage-x", CreatedAt: created}},
			[]*tokens.ParseResult{svctest.NewResult("conv-1", "claude-sonnet-4", wt, 1, 1, 0, created.Add(time.Second))},
			map[string]string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reader := &stampingBacklogReader{FakeBacklogReader: svctest.FakeBacklogReader{Entries: tc.entries}, stamped: map[string]string{}}
			svc := NewInsightsService(&svctest.FakeTokenStore{Results: tc.results}, tokens.DefaultPricingTable(),
				tokens.NewAssociator(&fakeSessionStorage{}), reader)

			n, err := svc.BackfillConversationUUIDs(context.Background())

			require.NoError(t, err)
			assert.Equal(t, tc.want, reader.stamped)
			assert.Equal(t, len(tc.want), n)
		})
	}
}

// TestBackfillConversationUUIDs_RespectsCanceledContext guards the fix: the
// O(n×m) match loop must check ctx itself, not rely solely on the (unreached,
// in this test) UpdateItemSessionConversationUUID DB call to enforce the
// caller's timeout.
func TestBackfillConversationUUIDs_RespectsCanceledContext(t *testing.T) {
	t.Parallel()
	created := time.Now().UTC().Add(-time.Hour)
	wt := "/home/u/.stapler-squad/worktrees/x"
	entries := []backlogdto.ItemSessionBacklogEntry{
		{ItemSessionID: "is-1", SessionUUID: uuid.NewString(), SessionRole: session.SessionRoleWork, CreatedAt: created},
	}
	results := []*tokens.ParseResult{svctest.NewResult("conv-1", "claude-sonnet-4", wt, 1, 1, 0, created.Add(4*time.Second))}
	reader := &stampingBacklogReader{FakeBacklogReader: svctest.FakeBacklogReader{Entries: entries}, stamped: map[string]string{}}
	svc := NewInsightsService(&svctest.FakeTokenStore{Results: results}, tokens.DefaultPricingTable(),
		tokens.NewAssociator(&fakeSessionStorage{}), reader)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	n, err := svc.BackfillConversationUUIDs(ctx)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, n)
	assert.Empty(t, reader.stamped, "a canceled context must stop the match loop before it writes any stamp")
}
