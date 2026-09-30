package services

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/puzpuzpuz/xsync/v4"

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
			reg := streamhub.NewTapRegistry(streamhub.TapRegistryOptions{EnvDir: dir})

			var ready, settling atomic.Bool
			ready.Store(tc.forwardingReady)
			settling.Store(tc.resizeSettling)
			p := controlModeOutputForwarderParams{
				sessionID:       "legacy-drop",
				forwardingReady: &ready,
				resizeSettling:  &settling,
				tap:             reg.Handle("legacy-drop").As(streamhub.TapSourceLegacy),
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

// The hub and legacy paths must tap under the same key. The hub's own session
// name is the prefixed tmux name, so a title-scoped enable only reaches the hub
// if hub creation is keyed on the title.
func TestHubRegistry_should_RecordUnderTitle_When_TapEnabledByTitle(t *testing.T) {
	dir := t.TempDir()
	reg := streamhub.NewTapRegistry(streamhub.TapRegistryOptions{DirFn: func() (string, error) { return dir, nil }})
	registry := &hubRegistry{hubs: xsync.NewMap[string, *streamhub.StreamHub](), tapRegistry: reg}

	const title = "my session"
	tmuxName := streamHubSessionKey(title, "")
	if tmuxName == title {
		t.Fatalf("test needs a tmux name (%q) distinct from the title", tmuxName)
	}
	if _, err := reg.Set(true, []string{title}, 0); err != nil {
		t.Fatal(err)
	}

	hub, err := registry.GetOrCreate(tmuxName, title, &fakeSessionController{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = hub.ForceTeardown() })
	hub.OnRawOutput([]byte("hello"))

	st := reg.Status()
	if len(st.Sessions) != 1 || st.Sessions[0].Name != title {
		t.Fatalf("tap sessions = %+v, want only one keyed by the title %q", st.Sessions, title)
	}
	f, err := os.Open(st.Sessions[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	recs, err := streamhub.ReadTap(f)
	if err != nil || len(recs) != 1 || recs[0].Src != streamhub.TapSourceHub || string(recs[0].Bytes()) != "hello" {
		t.Fatalf("records = %+v, %v; want one hub output record", recs, err)
	}
}
