package artifacts

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/tstapler/stapler-squad/session/scanstate"
)

// v1Scanned scans path with a version-1 extractor, persists its position, and
// returns a restarted version-2 harness over the same blob and state store.
func v1Scanned(t *testing.T, flag func() bool, paths ...string) *harness {
	t.Helper()
	states := &memStates{recs: map[string]scanstate.Record{}}
	old := newHarness(states)
	old.ae.scanVersion = 1 // the previous release
	for _, p := range paths {
		old.ae.scanFile(p)
	}
	if err := old.ae.FlushState(context.Background()); err != nil {
		t.Fatal(err)
	}
	cur := newHarness(states)
	cur.blob = old.blob
	cur.ae.scanVersion = 2
	cur.ae.scanV2 = flag
	cur.ae.restoreState(context.Background())
	return cur
}

func mustBlob(t *testing.T, raw string) *SessionArtifactsBlob {
	t.Helper()
	var b SessionArtifactsBlob
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		t.Fatal(err)
	}
	return &b
}

func transcript(t *testing.T, name string, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	appendLines(t, p, lines...)
	return p
}

func TestScanV2Flag_ShouldNotEnqueueBumpRescan_WhenFlagOff(t *testing.T) {
	t.Parallel()
	path := transcript(t, "s.jsonl", "PR https://github.com/o/r/pull/1")
	h := v1Scanned(t, func() bool { return false }, path)

	h.ae.walkAndEnqueue(context.Background(), filepath.Dir(path))
	if got := h.ae.queue.Pending(); got != 0 {
		t.Fatalf("flag off: stale unchanged transcript enqueued (pending=%d)", got)
	}
	stores := h.stores
	h.ae.scanFile(path)
	if h.stores != stores {
		t.Fatal("flag off: a direct scan rescanned an unchanged stale transcript")
	}
}

func TestScanV2Flag_ShouldRescanFromTop_WhenFlagOn(t *testing.T) {
	t.Parallel()
	path := transcript(t, "s.jsonl", "PR https://github.com/o/r/pull/1")
	h := v1Scanned(t, func() bool { return true }, path)

	h.ae.walkAndEnqueue(context.Background(), filepath.Dir(path))
	if got := h.ae.queue.Pending(); got != 1 {
		t.Fatalf("flag on: want the stale transcript enqueued, pending=%d", got)
	}
	stores := h.stores
	h.ae.scanFile(path)
	if h.stores != stores+1 {
		t.Fatalf("flag on: stale transcript not rescanned (stores %d -> %d)", stores, h.stores)
	}
	if v := mustBlob(t, h.blob).Version(); v != 2 {
		t.Fatalf("blob version after rescan = %d, want 2", v)
	}
	if tp, _ := h.ae.tracked(path); h.ae.stale(tp) {
		t.Fatal("position still stale after the rescan")
	}
}

func TestScanV2Flag_ShouldStopEnqueueing_WhenFlippedOffMidRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	names := []string{"a.jsonl", "b.jsonl", "c.jsonl"}
	paths := make([]string, 0, len(names))
	for _, n := range names {
		p := filepath.Join(dir, n)
		appendLines(t, p, "PR https://github.com/o/r/pull/1")
		paths = append(paths, p)
	}
	var reads atomic.Int32
	h := v1Scanned(t, func() bool { return reads.Add(1) == 1 }, paths...) // true once, then off

	h.ae.enqueueStale()
	if got := h.ae.queue.Pending(); got != 1 {
		t.Fatalf("flag flipped off after one read: want 1 enqueued, got %d", got)
	}
}

func TestScanV2Flag_ShouldResumeIncrementallyKeepingOldVersion_WhenFlagOffAndFileGrew(t *testing.T) {
	t.Parallel()
	path := transcript(t, "s.jsonl", "PR https://github.com/o/r/pull/1")
	h := v1Scanned(t, func() bool { return false }, path)
	before := h.ae.offsetOf(path)

	appendLines(t, path, "PR https://github.com/o/r/pull/2")
	h.ae.walkAndEnqueue(context.Background(), filepath.Dir(path))
	if got := h.ae.queue.Pending(); got != 1 {
		t.Fatalf("a grown file must still be caught up, pending=%d", got)
	}
	h.ae.scanFile(path)
	if h.ae.offsetOf(path) <= before {
		t.Fatal("offset did not advance")
	}
	if v := mustBlob(t, h.blob).Version(); v != 1 {
		t.Fatalf("incremental scan must not claim version 2, got %d", v)
	}
	if tp, _ := h.ae.tracked(path); !h.ae.stale(tp) {
		t.Fatal("position must stay stale so a later flag flip rescans")
	}
}

func TestDecodeOwner_ShouldRoundTripVersion_AndReadLegacyRows(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in       trackedPos
		wantVer  int
		wantVerf bool
	}{
		{trackedPos{owner: "s", verified: true, version: 2}, 2, true},
		{trackedPos{owner: "s", version: 3}, 3, false},
		{trackedPos{owner: "s", verified: true}, 1, true},
	} {
		owner, verified, ver := decodeOwner(encodeOwner(tc.in))
		if owner != "s" || verified != tc.wantVerf || ver != tc.wantVer {
			t.Errorf("round trip %+v -> %q %v %d", tc.in, owner, verified, ver)
		}
	}
	if _, _, v := decodeOwner([]byte("s\x00verified")); v != 1 {
		t.Fatalf("legacy row version = %d, want 1", v)
	}
}
