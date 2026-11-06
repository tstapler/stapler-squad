package artifacts

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func scanFixture(t *testing.T, lines ...string) (ae *ArtifactExtractor, path string, stored *string, completed *[]*SessionArtifactsBlob) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "session.jsonl")
	appendLines(t, path, lines...)
	var blob string
	var done []*SessionArtifactsBlob
	ae = NewArtifactExtractor(
		func(_, b string) error { blob = b; return nil },
		func(string) (string, error) { return blob, nil },
		func(string) (string, bool) { return "sess", true },
	)
	ae.OnScanComplete = func(_ string, b *SessionArtifactsBlob) { done = append(done, b) }
	return ae, path, &blob, &done
}

func decodeBlob(t *testing.T, raw string) SessionArtifactsBlob {
	t.Helper()
	var b SessionArtifactsBlob
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	return b
}

func TestScan_ShouldPersistNoneFound_WhenTranscriptHasNoMatches(t *testing.T) {
	t.Parallel()
	ae, path, stored, completed := scanFixture(t, "nothing interesting here", "still nothing")
	ae.scanFile(path)

	if *stored == "" {
		t.Fatal("empty scan was not persisted")
	}
	b := decodeBlob(t, *stored)
	if b.ScanState != ScanStateNoneFound {
		t.Fatalf("ScanState = %q, want none_found", b.ScanState)
	}
	if b.LastScannedAt.IsZero() {
		t.Error("LastScannedAt not set")
	}
	if len(*completed) != 1 || (*completed)[0] == nil {
		t.Fatalf("OnScanComplete must publish the none_found blob, got %v", *completed)
	}
}

func TestScan_ShouldNotRewriteBlob_WhenIncrementalScanFindsNothingAndStateRecorded(t *testing.T) {
	t.Parallel()
	ae, path, _, completed := scanFixture(t, "nothing")
	ae.scanFile(path)
	appendLines(t, path, "more nothing")
	ae.scanFile(path)
	if len(*completed) != 1 {
		t.Fatalf("incremental empty scan rewrote the blob (%d publishes)", len(*completed))
	}
}

func TestScan_ShouldPersistNoneFound_WhenUnchangedFileWasScannedBeforeStatesExisted(t *testing.T) {
	t.Parallel()
	ae, path, stored, _ := scanFixture(t, "nothing")
	ae.scanFile(path)
	// Simulate a pre-upgrade scan: position recorded unverified, nothing stored.
	ae.posMu.Lock()
	tp := ae.positions[path]
	tp.verified = false
	ae.positions[path] = tp
	ae.posMu.Unlock()
	ae.verified.Delete("sess")
	*stored = ""

	ae.scanFile(path)

	if got := decodeBlob(t, *stored).ScanState; got != ScanStateNoneFound {
		t.Fatalf("unchanged pre-upgrade transcript not upgraded, ScanState = %q", got)
	}
}

func TestScan_ShouldSetHasData_WhenArtifactsFound(t *testing.T) {
	t.Parallel()
	ae, path, stored, _ := scanFixture(t, "PR https://github.com/o/r/pull/5")
	ae.scanFile(path)
	if got := decodeBlob(t, *stored).ScanState; got != ScanStateHasData {
		t.Fatalf("ScanState = %q, want has_data", got)
	}
}

func TestMarkFailed_ShouldKeepLastGoodData_AndRecordFailedState(t *testing.T) {
	t.Parallel()
	ae, path, stored, completed := scanFixture(t, "PR https://github.com/o/r/pull/5")
	ae.scanFile(path)
	ae.markFailed(path, "sess")

	b := decodeBlob(t, *stored)
	if b.ScanState != ScanStateFailed {
		t.Fatalf("ScanState = %q, want failed", b.ScanState)
	}
	if len(b.PRURLs) != 1 {
		t.Fatalf("last good data lost on failure: %+v", b.PRURLs)
	}
	if got := (*completed)[len(*completed)-1].ScanState; got != ScanStateFailed {
		t.Fatalf("failure not published: %q", got)
	}
}

func TestSessionArtifactsBlob_State(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		blob SessionArtifactsBlob
		want ScanState
	}{
		{"explicit not_linked", SessionArtifactsBlob{ScanState: ScanStateNotLinked}, ScanStateNotLinked},
		{"explicit scanning", SessionArtifactsBlob{ScanState: ScanStateScanning}, ScanStateScanning},
		{"explicit none_found", SessionArtifactsBlob{ScanState: ScanStateNoneFound}, ScanStateNoneFound},
		{"explicit has_data", SessionArtifactsBlob{ScanState: ScanStateHasData}, ScanStateHasData},
		{"explicit failed", SessionArtifactsBlob{ScanState: ScanStateFailed}, ScanStateFailed},
		{"legacy with data", SessionArtifactsBlob{PRURLs: []string{"u"}}, ScanStateHasData},
		{"legacy command only", SessionArtifactsBlob{Commands: []CommandArtifact{{Type: "git_commit"}}}, ScanStateHasData},
		{"legacy empty", SessionArtifactsBlob{}, ScanStateNoneFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.blob.State(); got != tt.want {
				t.Fatalf("State() = %q, want %q", got, tt.want)
			}
		})
	}
}
