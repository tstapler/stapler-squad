package session

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/mux"
)

func TestNewCandidateFromDiscovered_MapsAllFields_When_MuxDiscoveredSessionValid(t *testing.T) {
	ds := &mux.DiscoveredSession{
		SocketPath: "/tmp/ssq-mux-4821.sock",
		Metadata: &mux.SessionMetadata{
			Cwd:         "/Users/x/proj",
			PID:         4821,
			TmuxSession: "ssq-mux-4821",
			Command:     "claude",
		},
	}

	got := NewCandidateFromDiscovered(ds)

	want := ExternalSessionCandidate{
		SourceKind:  MuxDiscovered,
		Path:        "/Users/x/proj",
		Program:     "claude",
		PID:         4821,
		TmuxSession: "ssq-mux-4821",
		SocketPath:  "/tmp/ssq-mux-4821.sock",
	}
	if got != want {
		t.Fatalf("NewCandidateFromDiscovered() = %+v, want %+v", got, want)
	}
}

func TestNewCandidateFromDiscovered_LeavesTmuxSessionEmpty_When_TmuxSessionMissing(t *testing.T) {
	ds := &mux.DiscoveredSession{
		SocketPath: "/tmp/ssq-mux-99.sock",
		Metadata: &mux.SessionMetadata{
			Cwd:     "/Users/x/proj2",
			PID:     99,
			Command: "claude",
			// TmuxSession intentionally left empty.
		},
	}

	got := NewCandidateFromDiscovered(ds)

	if got.TmuxSession != "" {
		t.Fatalf("TmuxSession = %q, want empty", got.TmuxSession)
	}
	if got.SourceKind != MuxDiscovered {
		t.Fatalf("SourceKind = %v, want MuxDiscovered", got.SourceKind)
	}
	if got.PID != 99 || got.Path != "/Users/x/proj2" || got.Program != "claude" {
		t.Fatalf("unexpected candidate: %+v", got)
	}
}
