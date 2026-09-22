package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tokens"
)

type stampingBacklogReader struct {
	fakeBacklogReader
	stamped map[string]string // item session ID -> conversation UUID
}

func (f *stampingBacklogReader) UpdateItemSessionConversationUUID(_ context.Context, id, conv string) error {
	f.stamped[id] = conv
	return nil
}

func TestBackfillConversationUUIDs(t *testing.T) {
	t.Parallel()
	created := time.Now().UTC().Add(-time.Hour)
	entry := func(id string, at time.Time) session.ItemSessionBacklogEntry {
		return session.ItemSessionBacklogEntry{ItemSessionID: id, SessionUUID: uuid.NewString(), SessionRole: session.SessionRoleWork, CreatedAt: at}
	}
	wt := "/home/u/.stapler-squad/worktrees/x"

	tests := []struct {
		name    string
		entries []session.ItemSessionBacklogEntry
		results []*tokens.ParseResult
		want    map[string]string
	}{
		{"unique match within window", []session.ItemSessionBacklogEntry{entry("is-1", created)},
			[]*tokens.ParseResult{newResult("conv-1", "claude-sonnet-4", wt, 1, 1, 0, created.Add(4*time.Second))},
			map[string]string{"is-1": "conv-1"}},
		{"ambiguous: two candidates in window", []session.ItemSessionBacklogEntry{entry("is-1", created), entry("is-2", created.Add(time.Minute))},
			[]*tokens.ParseResult{newResult("conv-1", "claude-sonnet-4", wt, 1, 1, 0, created.Add(90*time.Second))},
			map[string]string{}},
		{"transcript before the item session", []session.ItemSessionBacklogEntry{entry("is-1", created)},
			[]*tokens.ParseResult{newResult("conv-1", "claude-sonnet-4", wt, 1, 1, 0, created.Add(-2*time.Minute))},
			map[string]string{}},
		{"not a worktree transcript", []session.ItemSessionBacklogEntry{entry("is-1", created)},
			[]*tokens.ParseResult{newResult("conv-1", "claude-sonnet-4", "/home/u/code/kibitzer", 1, 1, 0, created.Add(time.Second))},
			map[string]string{}},
		{"synthetic session uuid", []session.ItemSessionBacklogEntry{{ItemSessionID: "is-1", SessionUUID: "headless-triage-x", CreatedAt: created}},
			[]*tokens.ParseResult{newResult("conv-1", "claude-sonnet-4", wt, 1, 1, 0, created.Add(time.Second))},
			map[string]string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reader := &stampingBacklogReader{fakeBacklogReader: fakeBacklogReader{entries: tc.entries}, stamped: map[string]string{}}
			svc := NewInsightsService(&fakeTokenStore{results: tc.results}, tokens.DefaultPricingTable(),
				tokens.NewAssociator(&fakeSessionStorage{}), reader)

			n, err := svc.BackfillConversationUUIDs(context.Background())

			require.NoError(t, err)
			assert.Equal(t, tc.want, reader.stamped)
			assert.Equal(t, len(tc.want), n)
		})
	}
}
