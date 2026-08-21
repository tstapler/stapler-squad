package session

import "github.com/tstapler/stapler-squad/session/mux"

// ImportSourceKind identifies which discovery mechanism produced an
// ExternalSessionCandidate. It determines which correlation inputs are
// available (socket+PID vs. pane-only) and which kill primitive applies.
//
// PlainTmux is defined now (Phase 1) even though it is not yet produced by
// any discovery path — that arrives in Phase 2 — so that ExternalSessionCandidate
// does not need a breaking change when the second source is added.
type ImportSourceKind int

const (
	// MuxDiscovered indicates the candidate came from an ssq-mux-wrapped
	// session discovered via mux.Discovery.Scan().
	MuxDiscovered ImportSourceKind = iota
	// PlainTmux indicates the candidate came from a plain tmux pane with no
	// ssq-mux wrapper (Phase 2; unused in Phase 1).
	PlainTmux
)

// String returns a human-readable name for logging.
func (k ImportSourceKind) String() string {
	switch k {
	case MuxDiscovered:
		return "mux_discovered"
	case PlainTmux:
		return "plain_tmux"
	default:
		return "unknown"
	}
}

// ExternalSessionCandidate describes an unmanaged, discovered process/pane
// eligible for import, before any Instance is constructed for it. It is
// never persisted — it exists only to carry enough information through the
// preview/commit/kill pipeline.
type ExternalSessionCandidate struct {
	SourceKind  ImportSourceKind
	Path        string
	Program     string
	PID         int32
	TmuxSession string
	// SocketPath is empty for PlainTmux candidates.
	SocketPath string
}

// NewCandidateFromDiscovered maps an ssq-mux DiscoveredSession into a
// source-agnostic ExternalSessionCandidate. Pure mapping, no I/O.
func NewCandidateFromDiscovered(ds *mux.DiscoveredSession) ExternalSessionCandidate {
	candidate := ExternalSessionCandidate{
		SourceKind: MuxDiscovered,
		SocketPath: ds.SocketPath,
	}
	if ds.Metadata != nil {
		candidate.Path = ds.Metadata.Cwd
		candidate.Program = ds.Metadata.Command
		candidate.PID = int32(ds.Metadata.PID)
		candidate.TmuxSession = ds.Metadata.TmuxSession
	}
	return candidate
}
