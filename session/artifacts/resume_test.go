package artifacts

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/tstapler/stapler-squad/session/scanstate"
)

// memStates is an in-memory StateStore standing in for the ent table.
type memStates struct {
	mu   sync.Mutex
	recs map[string]scanstate.Record
}

func (m *memStates) Load(context.Context) (map[string]scanstate.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]scanstate.Record{}
	for k, v := range m.recs {
		out[k] = v
	}
	return out, nil
}

func (m *memStates) Save(_ context.Context, recs []scanstate.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range recs {
		m.recs[r.Path] = r
	}
	return nil
}

// harness wires an extractor to an in-memory blob store and counts calls.
type harness struct {
	ae      *ArtifactExtractor
	blob    string
	lookups int
	stores  int
}

func newHarness(states StateStore) *harness {
	h := &harness{}
	h.ae = NewArtifactExtractor(
		func(_, blob string) error { h.blob = blob; h.stores++; return nil },
		func(string) (string, error) { return h.blob, nil },
		func(string) (string, bool) { h.lookups++; return "sess", true },
		WithStateStore(states),
	)
	return h
}

func appendLines(t *testing.T, path string, texts ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, txt := range texts {
		if err := writeJSONLLine(f, txt); err != nil {
			t.Fatal(err)
		}
	}
}

func prURLs(t *testing.T, blob string) []string {
	t.Helper()
	var b SessionArtifactsBlob
	if err := json.Unmarshal([]byte(blob), &b); err != nil {
		t.Fatal(err)
	}
	return b.PRURLs
}

func TestScanFile_RestartDoesNotRescanUnchangedTranscript(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	appendLines(t, path, "no artifacts here")
	states := &memStates{recs: map[string]scanstate.Record{}}

	first := newHarness(states)
	first.ae.scanFile(path)
	if err := first.ae.FlushState(context.Background()); err != nil {
		t.Fatal(err)
	}

	second := newHarness(states)
	second.ae.restoreState(context.Background())
	second.ae.scanFile(path)

	// An artifact-free file never wrote a blob, so only the state table can
	// have remembered it; a skipped scan never even resolves the session.
	if second.lookups != 0 {
		t.Fatalf("unchanged file was rescanned after restart (lookups=%d)", second.lookups)
	}
}

func TestScanFile_RestartThenAppendScansOnlyNewBytes(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	appendLines(t, path, "PR https://github.com/o/r/pull/1")
	states := &memStates{recs: map[string]scanstate.Record{}}

	first := newHarness(states)
	first.ae.scanFile(path)
	if err := first.ae.FlushState(context.Background()); err != nil {
		t.Fatal(err)
	}
	oldOffset := first.ae.offsetOf(path)

	appendLines(t, path, "PR https://github.com/o/r/pull/2")
	second := newHarness(states)
	second.blob = first.blob
	second.ae.restoreState(context.Background())
	second.ae.scanFile(path)

	got := prURLs(t, second.blob)
	if len(got) != 2 || got[0] != "https://github.com/o/r/pull/1" || got[1] != "https://github.com/o/r/pull/2" {
		t.Fatalf("PR URLs after restart+append = %v", got)
	}
	if second.ae.offsetOf(path) <= oldOffset {
		t.Fatalf("offset did not advance past %d", oldOffset)
	}
}

func TestScanFile_TruncatedFileIsRescannedFromStart(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	appendLines(t, path, "PR https://github.com/o/r/pull/1", "filler line to make the first file long enough")
	h := newHarness(&memStates{recs: map[string]scanstate.Record{}})
	h.ae.scanFile(path)

	// Rewritten shorter, with a different artifact: the stale offset lies past EOF.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	appendLines(t, path, "PR https://github.com/o/r/pull/7")
	h.ae.scanFile(path)

	got := prURLs(t, h.blob)
	if len(got) != 2 || got[1] != "https://github.com/o/r/pull/7" {
		t.Fatalf("truncated file not rescanned from the top: %v", got)
	}
}

func TestScanFile_UnownedFileIsNotScanned(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	appendLines(t, path, "PR https://github.com/o/r/pull/1")
	ae := NewArtifactExtractor(
		func(_, _ string) error { t.Error("storeFn called for an unowned file"); return nil },
		func(string) (string, error) { return "", nil },
		func(string) (string, bool) { return "", false },
	)
	ae.scanFile(path)
	if _, known := ae.position(path); known {
		t.Fatal("position recorded for a file no session owns")
	}
}

func TestScanFile_PartialLastLineWaitsForItsNewline(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	appendLines(t, path, "PR https://github.com/o/r/pull/1")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"PR https://github.com/o/r/pu`); err != nil {
		t.Fatal(err)
	}
	h := newHarness(&memStates{recs: map[string]scanstate.Record{}})
	h.ae.scanFile(path)
	if got := prURLs(t, h.blob); len(got) != 1 {
		t.Fatalf("partial line leaked into results: %v", got)
	}

	if _, err := f.WriteString("ll/2\"}]}}\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	h.ae.scanFile(path)
	if got := prURLs(t, h.blob); len(got) != 2 || got[1] != "https://github.com/o/r/pull/2" {
		t.Fatalf("completed line not picked up exactly once: %v", got)
	}
}

func TestScanFile_NewOwnerRescansFileFromTheTop(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	appendLines(t, path, "PR https://github.com/o/r/pull/1")
	owner := "A"
	var blobs = map[string]string{}
	ae := NewArtifactExtractor(
		func(title, blob string) error { blobs[title] = blob; return nil },
		func(title string) (string, error) { return blobs[title], nil },
		func(string) (string, bool) { return owner, true },
	)
	ae.scanFile(path)

	owner = "B" // another session takes over the same transcript
	appendLines(t, path, "no artifact")
	ae.scanFile(path)

	if got := prURLs(t, blobs["B"]); len(got) != 1 || got[0] != "https://github.com/o/r/pull/1" {
		t.Fatalf("new owner missed artifacts from before it owned the file: %v", got)
	}
}
