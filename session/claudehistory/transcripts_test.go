package claudehistory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeTranscript(t *testing.T, path, sessionID, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"user","sessionId":"` + sessionID + `","message":{"role":"user","content":"` + text + `"},"timestamp":"2026-01-01T00:00:00Z"}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestScanTranscripts_should_ResolveByNameThenReference_When_Looking_UpSessions(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// "aaa" is referenced by an earlier-walked file but also has its own.
	writeTranscript(t, filepath.Join(root, "p1", "0-resumed.jsonl"), "aaa", "referencing")
	writeTranscript(t, filepath.Join(root, "p2", "aaa.jsonl"), "aaa", "own file")
	writeTranscript(t, filepath.Join(root, "p2", "other-name.jsonl"), "bbb", "found by reference")
	writeTranscript(t, filepath.Join(root, "p2", "agent-1.jsonl"), "ccc", "subagent")

	tr, err := ScanTranscripts(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{"aaa": "own file", "bbb": "found by reference"} {
		msgs, err := tr.Messages(id)
		if err != nil || len(msgs) != 1 || msgs[0].Content != want {
			t.Errorf("Messages(%s) = %+v, %v; want content %q", id, msgs, err, want)
		}
	}
	for _, id := range []string{"ccc", "missing"} {
		if _, err := tr.Messages(id); !errors.Is(err, ErrTranscriptNotFound) {
			t.Errorf("Messages(%s) error = %v, want ErrTranscriptNotFound", id, err)
		}
	}
}
