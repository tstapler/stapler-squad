package services

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// The legacy forwarder must record why it drops a frame, and pick the cause
// from the same condition that triggered the drop.
func TestForwardOneControlModeFrame_should_RecordDropCause_When_FrameIsDropped(t *testing.T) {
	tests := []struct {
		name            string
		forwardingReady bool
		resizeSettling  bool
		wantCause       streamhub.DropCause
	}{
		{"not ready", false, false, streamhub.DropCauseForwardingNotReady},
		{"not ready wins over settling", false, true, streamhub.DropCauseForwardingNotReady},
		{"ready but settling", true, true, streamhub.DropCauseResizeSettling},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv(streamhub.CaptureTapDirEnv, dir)

			var ready, settling atomic.Bool
			ready.Store(tc.forwardingReady)
			settling.Store(tc.resizeSettling)
			p := controlModeOutputForwarderParams{
				sessionID:       "legacy-drop",
				forwardingReady: &ready,
				resizeSettling:  &settling,
				tap:             streamhub.CaptureTapFor("legacy-drop").As(streamhub.TapSourceLegacy),
			}

			stop := (&ConnectRPCWebSocketHandler{}).forwardOneControlModeFrame(p, nil, []byte("dropped"))
			if stop {
				t.Fatal("a dropped frame must not stop the forwarder")
			}

			f, err := os.Open(filepath.Join(dir, "legacy-drop.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			recs, err := streamhub.ReadTap(f)
			if err != nil || len(recs) != 1 {
				t.Fatalf("ReadTap = %v, %v; want 1 record", recs, err)
			}
			got := recs[0]
			if got.Kind != streamhub.TapDrop || got.Cause != tc.wantCause || got.Src != streamhub.TapSourceLegacy || string(got.Bytes()) != "dropped" {
				t.Fatalf("record = %+v, want drop/%s from legacy carrying the frame", got, tc.wantCause)
			}
		})
	}
}
