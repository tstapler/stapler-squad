package artifacts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/session/domain"
)

// largestRealTranscript returns the biggest transcript under ~/.claude/projects, or "".
func largestRealTranscript() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	var best string
	var bestSize int64
	_ = filepath.WalkDir(filepath.Join(home, ".claude", "projects"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".jsonl") {
			return nil
		}
		if fi, err := d.Info(); err == nil && fi.Size() > bestSize {
			best, bestSize = p, fi.Size()
		}
		return nil
	})
	return best
}

func scanToBlob(t *testing.T, path string) (*domain.SessionArtifactsBlob, int) {
	t.Helper()
	var raw string
	ae := NewArtifactExtractor(
		func(_, blob string) error { raw = blob; return nil },
		func(string) (string, error) { return raw, nil },
		func(string) (string, bool) { return "s", true },
	)
	ae.scanFile(path)
	return mustBlob(t, raw), len(raw)
}

func TestScan_ShouldCapBlobAt128KB_WhenGeneratedHugeTranscript(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "huge.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3000; i++ {
		id := fmt.Sprintf("t%d", i)
		cmd := fmt.Sprintf(`git commit -m "change %d %s"`, i, strings.Repeat("x", 200))
		out := fmt.Sprintf("[b%d %07x] change %d\nhttps://github.com/o/r/pull/%d https://e.io/p/%d %s", i, i, i, i, i, strings.Repeat("y", 3000))
		_, _ = f.Write(append(toolUseLine(t, "2026-11-04T10:00:00Z", id, cmd), '\n'))
		_, _ = f.Write(append(toolResultLine(t, "2026-11-04T10:00:01Z", id, out), '\n'))
	}
	_ = f.Close()

	blob, size := scanToBlob(t, path)
	if size > MaxBlobBytes {
		t.Fatalf("stored blob is %d bytes, cap %d", size, MaxBlobBytes)
	}
	for _, r := range blob.Records {
		if c, ok := r.(domain.CommandRecord); ok && len(c.Result) > MaxResultBytes {
			t.Fatalf("command result %d bytes", len(c.Result))
		}
	}
	if len(blob.Records) == 0 {
		t.Fatal("cap dropped everything")
	}
	if MaxBlobBytesObserved() == 0 {
		t.Fatal("ssq_artifact_blob_bytes was never observed")
	}
}

func TestScan_ShouldCapBlobAt128KB_WhenLargestRealTranscript(t *testing.T) {
	if testing.Short() {
		t.Skip("scans the largest real transcript")
	}
	path := largestRealTranscript()
	if path == "" {
		t.Skip("no real transcript on this machine; the generated fixture covers the shape")
	}
	_, size := scanToBlob(t, path)
	t.Logf("%s -> blob %d bytes", path, size)
	if size > MaxBlobBytes {
		t.Fatalf("stored blob is %d bytes, cap %d", size, MaxBlobBytes)
	}
}
