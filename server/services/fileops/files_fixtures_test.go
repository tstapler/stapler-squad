package fileops

// Fixtures shared by this package's tests (copies of the contentsearch ones).

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/tstapler/stapler-squad/server/services/fsops"
	"github.com/tstapler/stapler-squad/session/domain"
)

const (
	fixtureSeed      = 42
	searchBenchQuery = "TODO"
)

func mustMkdir(tb testing.TB, p string) {
	tb.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		tb.Fatal(err)
	}
}

func writeFixtureFile(tb testing.TB, p string, data []byte) {
	tb.Helper()
	if err := os.WriteFile(p, data, 0o644); err != nil {
		tb.Fatal(err)
	}
}

// treeDigest hashes relative paths and contents in walk order.
func treeDigest(tb testing.TB, root string) [32]byte {
	tb.Helper()
	h := sha256.New()
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		fmt.Fprintf(h, "%s\x00%d\x00", rel, len(data))
		h.Write(data)
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func onDisk(dir string) domain.Workspace {
	return domain.Workspace{RepoRoot: dir, ActiveDir: dir, ExistingDir: dir}
}

type recordingTelemetry struct {
	mu       sync.Mutex
	searches []fsops.SearchOutcomeEvent
	firsts   []fsops.SearchFirstResultEvent
	writes   []fsops.WriteOutcomeEvent
	patches  []fsops.PatchOutcomeEvent
}

func (r *recordingTelemetry) RecordWrite(_ context.Context, _ string, ev fsops.WriteOutcomeEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writes = append(r.writes, ev)
}

func (r *recordingTelemetry) RecordPatch(_ context.Context, _ string, ev fsops.PatchOutcomeEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.patches = append(r.patches, ev)
}

func (r *recordingTelemetry) RecordSearch(_ context.Context, _ string, ev fsops.SearchOutcomeEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.searches = append(r.searches, ev)
}

func (r *recordingTelemetry) RecordSearchFirstResult(_ context.Context, _ string, ev fsops.SearchFirstResultEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.firsts = append(r.firsts, ev)
}
