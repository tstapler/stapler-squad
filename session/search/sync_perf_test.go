package search

import (
	"context"
	"math/rand"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/claudehistory"
)

// buildIndexFrom indexes only the given documents: the oracle for batch removal.
func buildIndexFrom(docs map[int32][]string, order []int32) *InvertedIndex {
	idx := NewInvertedIndex()
	for _, id := range order {
		tokens, ok := docs[id]
		if !ok {
			continue
		}
		idx.AddDocumentSimple(id, tokens)
	}
	return idx
}

func TestRemoveDocuments_should_MatchFreshIndex_When_BatchRemoved(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(1))
	for round := 0; round < 50; round++ {
		docs := map[int32][]string{}
		var order []int32
		for id := int32(1); id <= 30; id++ {
			tokens := make([]string, 1+rng.Intn(8))
			for i := range tokens {
				tokens[i] = "t" + strconv.Itoa(rng.Intn(12))
			}
			docs[id] = tokens
			order = append(order, id)
		}
		idx := buildIndexFrom(docs, order)

		var doomed []int32
		for _, id := range order {
			if rng.Intn(3) == 0 {
				doomed = append(doomed, id)
				delete(docs, id)
			}
		}
		idx.RemoveDocuments(append(doomed, 9999)) // unknown ids are ignored

		want := buildIndexFrom(docs, order)
		if !reflect.DeepEqual(idx.Index, want.Index) {
			t.Fatalf("round %d: postings differ after removing %v", round, doomed)
		}
		if idx.TotalDocs != want.TotalDocs || idx.AvgDocLength != want.AvgDocLength {
			t.Fatalf("round %d: totals %d/%v want %d/%v", round, idx.TotalDocs, idx.AvgDocLength, want.TotalDocs, want.AvgDocLength)
		}
	}
}

func TestAvgDocLength_should_StayCorrect_When_TotalNotYetComputed(t *testing.T) {
	t.Parallel()
	// A gob-loaded index has DocLengths but no running total.
	idx := &InvertedIndex{Index: map[string]*PostingsList{}, DocLengths: map[int32]int{1: 4, 2: 6}, TotalDocs: 2, AvgDocLength: 5}
	idx.AddDocumentSimple(3, []string{"a", "b"})
	if idx.AvgDocLength != 4 {
		t.Fatalf("avg = %v, want 4", idx.AvgDocLength)
	}
}

func TestSearch_should_KeepPreviousCopy_When_SessionTranscriptIsUnreadable(t *testing.T) {
	t.Parallel()
	e := NewSearchEngine()
	if err := e.IndexMessage("s1", 0, "user", "needle in the haystack", time.Now()); err != nil {
		t.Fatal(err)
	}
	e.syncMetadata = NewIndexSyncMetadata()
	empty, err := claudehistory.ScanTranscripts(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	result := &SyncResult{}
	e.syncSession(empty, claudehistory.ClaudeHistoryEntry{ID: "s1"}, false, result)

	if len(result.Errors) != 1 {
		t.Fatalf("errors = %v, want one", result.Errors)
	}
	got, err := e.Search("needle", SearchOptions{})
	if err != nil || len(got.Results) != 1 {
		t.Fatalf("Search = %+v, %v; want the previously indexed message", got, err)
	}
}

func TestPersist_should_NotRewriteIndex_When_SavedRecently(t *testing.T) {
	t.Parallel()
	store, err := NewIndexStoreWithDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := NewSearchEngineWithPersistence(store)
	e.syncMetadata = NewIndexSyncMetadata()
	add := func(text string) {
		t.Helper()
		if err := e.IndexMessage("s1", 0, "user", text, time.Now()); err != nil {
			t.Fatal(err)
		}
		if err := e.persist(); err != nil {
			t.Fatal(err)
		}
	}
	savedDocs := func() int {
		t.Helper()
		idx, _, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		return idx.GetTotalDocs()
	}

	add("first")
	add("second")
	if got := savedDocs(); got != 1 {
		t.Fatalf("saved docs = %d, want 1 (second save throttled)", got)
	}
	e.lastSave = time.Now().Add(-minIndexSaveInterval)
	add("third")
	if got := savedDocs(); got != 3 {
		t.Fatalf("saved docs = %d, want 3 after the interval", got)
	}
}
