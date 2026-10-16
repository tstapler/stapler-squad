package session

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	ssqlog "github.com/tstapler/stapler-squad/log"
)

type fakeClaimRecorder struct {
	calls []ClaimRecord
	err   error
}

func (f *fakeClaimRecorder) RecordClaim(_ context.Context, record ClaimRecord) error {
	f.calls = append(f.calls, record)
	return f.err
}

func newClaimTestStorage(t *testing.T) *Storage {
	t.Helper()
	storage, err := NewStorageWithRepository(NewTestEntRepository(t))
	if err != nil {
		t.Fatalf("NewStorageWithRepository: %v", err)
	}
	return storage
}

func createItemWithURL(t *testing.T, storage *Storage, externalURL string) *BacklogItemData {
	t.Helper()
	item, err := storage.CreateBacklogItem(context.Background(), BacklogItemData{
		Title:       "claim recorder test",
		Status:      string(BacklogStatusInProgress),
		ExternalURL: externalURL,
	})
	if err != nil {
		t.Fatalf("CreateBacklogItem() error = %v, want nil", err)
	}
	return item
}

func TestStorage_CreateBacklogItem_should_CallClaimRecorderWithItemDeepLink_When_ExternalURLNonEmpty(t *testing.T) {
	storage := newClaimTestStorage(t)
	recorder := &fakeClaimRecorder{}
	storage.SetClaimRecorder(recorder)

	item := createItemWithURL(t, storage, testIssueURL)

	if len(recorder.calls) != 1 {
		t.Fatalf("RecordClaim called %d times, want 1", len(recorder.calls))
	}
	got := recorder.calls[0]
	if got.ExternalURL != testIssueURL {
		t.Errorf("ExternalURL = %q, want %q", got.ExternalURL, testIssueURL)
	}
	if want := BacklogItemDeepLinkPath(item); got.ItemDeepLink != want || !strings.HasPrefix(want, "/backlog/v1/bl_") {
		t.Errorf("ItemDeepLink = %q, want %q (public-ID based)", got.ItemDeepLink, want)
	}
}

func TestStorage_CreateBacklogItem_should_SkipClaimRecording_When_ExternalURLEmpty(t *testing.T) {
	storage := newClaimTestStorage(t)
	recorder := &fakeClaimRecorder{}
	storage.SetClaimRecorder(recorder)

	createItemWithURL(t, storage, "")

	if len(recorder.calls) != 0 {
		t.Fatalf("RecordClaim called %d times for a local-only item, want 0", len(recorder.calls))
	}
}

func TestStorage_CreateBacklogItem_should_SucceedAndLogRecordFailed_When_ClaimRecorderReturnsError(t *testing.T) {
	storage := newClaimTestStorage(t)
	storage.SetClaimRecorder(&fakeClaimRecorder{err: errors.New("lock timeout")})

	var buf bytes.Buffer
	prev := ssqlog.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { ssqlog.SetSlogDefaultForTest(prev) })

	item := createItemWithURL(t, storage, testIssueURL)

	if item == nil || item.ID == "" {
		t.Fatalf("item = %+v, want a created item despite recorder failure", item)
	}
	if _, err := storage.GetBacklogItemByExternalURL(context.Background(), testIssueURL); err != nil {
		t.Fatalf("created item not retrievable: %v", err)
	}
	if !strings.Contains(buf.String(), "claim_index.record_failed") {
		t.Fatalf("expected claim_index.record_failed log line, got: %s", buf.String())
	}
}

func TestStorage_CreateBacklogItem_should_SkipClaimRecording_When_ClaimRecorderNil(t *testing.T) {
	storage := newClaimTestStorage(t)
	if item := createItemWithURL(t, storage, testIssueURL); item == nil {
		t.Fatalf("CreateBacklogItem() = nil, want item with no recorder wired")
	}

	storage.SetClaimRecorder(&fakeClaimRecorder{})
	storage.SetClaimRecorder(nil)
	createItemWithURL(t, storage, "https://github.com/o/r/issues/2")
}

func TestStorage_CreateBacklogItem_should_PopulateRealClaimIndexEntry_When_UsingClaimIndexBackedClaimRecorder(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	local := newTestIdentity(t)
	index := newTestClaimIndex(t, t.TempDir(), clock)
	recorder, err := NewClaimIndexRecorder(local, index, nil, "hosta.example", clock)
	if err != nil {
		t.Fatalf("NewClaimIndexRecorder: %v", err)
	}
	storage := newClaimTestStorage(t)
	storage.SetClaimRecorder(recorder)

	item := createItemWithURL(t, storage, testIssueURL)

	got, ok := index.CheckClaim(testIssueURL)
	if !ok {
		t.Fatalf("CheckClaim() = not found, want the claim recorded by CreateBacklogItem")
	}
	if got.ClaimingHostID.String() != local.ID.String() {
		t.Errorf("ClaimingHostID = %s, want local %s", got.ClaimingHostID, local.ID)
	}
	if want := "ssq://hosta.example" + BacklogItemDeepLinkPath(item); got.ItemDeepLink != want {
		t.Errorf("ItemDeepLink = %q, want %q", got.ItemDeepLink, want)
	}
	if !got.Verify() {
		t.Errorf("stored claim does not verify")
	}
}

func TestNewClaimIndexRecorder_should_ReturnError_When_IdentityOrIndexMissing(t *testing.T) {
	index := newTestClaimIndex(t, t.TempDir(), &fakeClock{now: time.Now()})
	if _, err := NewClaimIndexRecorder(HostIdentity{}, index, nil, "h", nil); err == nil {
		t.Errorf("zero identity: error = nil, want error")
	}
	if _, err := NewClaimIndexRecorder(newTestIdentity(t), nil, nil, "h", nil); err == nil {
		t.Errorf("nil index: error = nil, want error")
	}
}
