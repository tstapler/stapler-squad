package session

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/ansi"
	"github.com/tstapler/stapler-squad/session/scrollback"
)

// DefaultReviewTranscriptMaxBytes bounds how much ANSI-stripped scrollback is written
// to a review transcript file. This is no longer a prompt-embedding budget (the file
// is searched on demand via the reviewer's Grep/Read tools, not injected into the
// prompt text), so it can be considerably larger than a typical per-section prompt
// budget -- 256KB comfortably covers a long session's tail without writing unbounded
// data into a real repo checkout.
const DefaultReviewTranscriptMaxBytes int64 = 256 * 1024

// reviewTranscriptTruncationMarker is prepended when the stripped transcript exceeds
// maxBytes and the head had to be dropped to keep the (more relevant) tail.
const reviewTranscriptTruncationMarker = "... [earlier transcript truncated] ...\n"

// reviewTranscriptFilePrefix names files written by WriteReviewTranscriptFile. It is
// dot-prefixed and namespaced so it is extremely unlikely to collide with any real
// file in a codebase checkout.
const reviewTranscriptFilePrefix = ".stapler-squad-review-transcript-"

// reviewTranscriptANSIRegex matches ANSI escape sequences for stripping: CSI sequences
// (colors, cursor moves), OSC sequences (window titles, hyperlinks), and G0/G1 charset
// designations (\x1b(B, \x1b(0, ...). This mirrors session/detection/detector.go's
// ansiStripRegex -- the same three-alternative pattern is the established convention
// in this codebase for stripping PTY/tmux output, and the CSI final-byte class comes
// from pkg/ansi.CSIFinalByteClass (the single source of truth fixed by BUG-025; see
// pkg/ansi/csi.go) rather than being hand-rolled again here.
var reviewTranscriptANSIRegex = regexp.MustCompile(`\x1b\[[0-9;]*` + ansi.CSIFinalByteClass + `|\x1b\][^\x07]*\x07|\x1b[()][A-Za-z0-9]`)

// stripANSI removes ANSI escape sequences from raw terminal bytes, returning clean text.
func stripANSI(raw []byte) string {
	return reviewTranscriptANSIRegex.ReplaceAllString(string(raw), "")
}

// reviewTranscriptFileName returns the file name used to store sessionUUID's transcript.
func reviewTranscriptFileName(sessionUUID string) string {
	return reviewTranscriptFilePrefix + sessionUUID + ".txt"
}

// WriteReviewTranscriptFile fetches sessionUUID's most recent scrollback, strips ANSI
// escape sequences, and writes the result to a file inside codebaseWorkDir so a
// reviewer LLM can search it on demand with its already-granted Read/Grep/Glob tools --
// instead of the orchestrator pre-injecting a text blob into the review prompt, which
// would bloat the prompt/context window regardless of session length.
//
// The returned relPath is relative to codebaseWorkDir (e.g.
// ".stapler-squad-review-transcript-<uuid>.txt"), so it can be dropped directly into a
// reviewer prompt template and is treated consistently by containment-checked
// tool_reads logic the same way as any other path the reviewer cites. cleanup removes
// the written file and is always safe to call (including when relPath == "", in which
// case it is a no-op) -- callers should defer cleanup() immediately after a successful
// call so the file does not linger in the real repo checkout after the review completes.
//
// Fetching scrollback is treated as best-effort enrichment: if the session has no
// scrollback (never started, expired, or storage error), WriteReviewTranscriptFile
// returns ("", no-op cleanup, nil) rather than an error, so a missing/expired session's
// scrollback never blocks a review. A non-nil error is returned only when scrollback
// WAS available but writing it to disk failed (e.g. codebaseWorkDir unwritable) --
// callers may choose to ignore this error too, given the enrichment-only contract.
//
// maxBytes bounds how much stripped transcript is written; pass <= 0 to use
// DefaultReviewTranscriptMaxBytes. When the stripped transcript exceeds maxBytes, the
// HEAD is dropped and the tail is kept (most recent activity is most relevant to a
// reviewer checking final state), prefixed with a truncation marker.
func WriteReviewTranscriptFile(sm *scrollback.ScrollbackManager, sessionUUID, codebaseWorkDir string, maxBytes int64) (relPath string, cleanup func(), err error) {
	noop := func() {}

	if sm == nil || sessionUUID == "" || codebaseWorkDir == "" {
		return "", noop, nil
	}
	if maxBytes <= 0 {
		maxBytes = DefaultReviewTranscriptMaxBytes
	}

	// Fetch a bit more than the target so that after ANSI stripping removes escape
	// bytes, the result still comes close to filling maxBytes rather than falling
	// noticeably short of it.
	raw, fetchErr := sm.GetRecentBytes(sessionUUID, maxBytes*2)
	if fetchErr != nil {
		log.Debug("review transcript: scrollback unavailable", "session", sessionUUID, "err", fetchErr)
		return "", noop, nil
	}
	if len(raw) == 0 {
		log.Debug("review transcript: no scrollback for session", "session", sessionUUID)
		return "", noop, nil
	}

	stripped := stripANSI(raw)
	if int64(len(stripped)) > maxBytes {
		stripped = reviewTranscriptTruncationMarker + stripped[int64(len(stripped))-maxBytes:]
	}

	name := reviewTranscriptFileName(sessionUUID)
	absPath := filepath.Join(codebaseWorkDir, name)
	if writeErr := os.WriteFile(absPath, []byte(stripped), 0o644); writeErr != nil {
		return "", noop, fmt.Errorf("failed to write review transcript file: %w", writeErr)
	}

	cleanup = func() {
		if rmErr := os.Remove(absPath); rmErr != nil && !os.IsNotExist(rmErr) {
			log.Warn("review transcript: failed to clean up transcript file", "path", absPath, "err", rmErr)
		}
	}

	return name, cleanup, nil
}
