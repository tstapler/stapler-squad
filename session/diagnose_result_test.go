package session

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDiagnoseResult(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, raw string
		ok        bool
	}{
		{"empty", "", false},
		{"malformed", "{nope", false},
		{"valid", `{"outcome":"NOTE_ONLY","summary":"s"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, ok := ParseDiagnoseResult(tc.raw)
			assert.Equal(t, tc.ok, ok)
			if ok {
				assert.Equal(t, "NOTE_ONLY", d.Outcome)
			}
		})
	}
}

func TestParseWorkflowSuggestions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, raw string
		want      int
	}{
		{"empty", "", 0},
		{"malformed", "[{", 0},
		{"valid", `[{"suggestion_id":"a","recommendation":"r"},{"suggestion_id":"b"}]`, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Len(t, ParseWorkflowSuggestions(tc.raw), tc.want)
		})
	}
}

func newSuggestionItem(t *testing.T) (*EntRepository, string) {
	t.Helper()
	repo := NewTestEntRepository(t)
	item, err := repo.CreateBacklogItem(context.Background(), BacklogItemData{Title: "x", Status: string(BacklogStatusIdea)})
	require.NoError(t, err)
	return repo, item.ID
}

func TestAppendWorkflowSuggestion_CapAndDuplicate(t *testing.T) {
	t.Parallel()
	repo, itemID := newSuggestionItem(t)
	ctx := context.Background()
	first, err := repo.AppendWorkflowSuggestion(ctx, itemID, WorkflowSuggestionData{ActionType: "operator_note", Recommendation: "dup"})
	require.NoError(t, err)

	_, err = repo.AppendWorkflowSuggestion(ctx, itemID, WorkflowSuggestionData{ActionType: "operator_note", Recommendation: "dup"})
	require.ErrorIs(t, err, ErrDuplicateWorkflowSuggestion)

	// A dismissed duplicate no longer blocks, and no longer counts toward the cap.
	require.NoError(t, repo.DismissWorkflowSuggestion(ctx, itemID, first))
	for i := 0; i < MaxActiveWorkflowSuggestions; i++ {
		_, err = repo.AppendWorkflowSuggestion(ctx, itemID, WorkflowSuggestionData{ActionType: "operator_note", Recommendation: fmt.Sprintf("r%d", i)})
		require.NoError(t, err)
	}
	_, err = repo.AppendWorkflowSuggestion(ctx, itemID, WorkflowSuggestionData{ActionType: "operator_note", Recommendation: "over"})
	require.ErrorIs(t, err, ErrTooManyWorkflowSuggestions)
}

func TestAppendWorkflowSuggestion_ParallelAppendsPersist(t *testing.T) {
	t.Parallel()
	repo, itemID := newSuggestionItem(t)
	ctx := context.Background()
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = repo.AppendWorkflowSuggestion(ctx, itemID, WorkflowSuggestionData{ActionType: "operator_note", Recommendation: fmt.Sprintf("p%d", i)})
		}()
	}
	wg.Wait()
	for _, e := range errs {
		require.NoError(t, e)
	}
	item, err := repo.GetBacklogItem(ctx, itemID)
	require.NoError(t, err)
	assert.Len(t, ParseWorkflowSuggestions(item.WorkflowSuggestions), n)
}
