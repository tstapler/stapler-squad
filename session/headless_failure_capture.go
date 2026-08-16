package session

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultHeadlessFailureCaptureMaxBytes bounds how much raw headless (claude -p)
// stdout is written to a failure capture file. Mirrors
// DefaultReviewTranscriptMaxBytes's precedent and size (256KB) — this is not a
// prompt-embedding budget (the file is read by a human via the UI/API, not
// injected into another LLM's context), so it can comfortably hold a long
// triage/review call's full output.
const DefaultHeadlessFailureCaptureMaxBytes int64 = 256 * 1024

// headlessFailureCaptureTruncationMarker is prepended when raw exceeds maxBytes
// and the head had to be dropped to keep the tail. The tail (not the head) is
// kept deliberately: for a parse failure the interesting content — the
// malformed JSON, an error message, or trailing chatter — is almost always at
// the end of the LLM's output, mirroring review_transcript.go's identical
// head-drop rationale for the same reason.
const headlessFailureCaptureTruncationMarker = "... [earlier output truncated] ...\n"

// headlessFailureCaptureFilePrefix names files written by WriteHeadlessFailureCapture.
const headlessFailureCaptureFilePrefix = "headless-failure-"

// WriteHeadlessFailureCapture writes raw (the accumulated stdout of a headless
// triage/review claude -p call that either errored or produced output
// ParseHeadlessTriageResult/ParseHeadlessVerdictResult could not use) to a
// durable file under dir, named after sessionUUID, and returns its absolute
// path.
//
// This exists because the log line previously emitted on a parse failure only
// includes a ~200-byte preview, and the log file itself rotates out of
// ~/.stapler-squad/logs/ within a few hours — for a long-running call there was
// previously no way to recover what the LLM actually returned once that window
// closed. Unlike WriteReviewTranscriptFile (session/review_transcript.go),
// which writes into a real repo checkout and is cleaned up immediately after a
// review completes, this file is meant to persist indefinitely for later
// diagnosis, so it is written to the caller-supplied dir (in practice
// config.Config.HeadlessFailureCaptureDirOrDefault(), under
// ~/.stapler-squad/headless-failures/) and never removed here.
//
// raw is written verbatim, with no ANSI stripping: claude -p's headless
// stdout is not PTY output (unlike the scrollback WriteReviewTranscriptFile
// captures), so it does not carry escape sequences.
//
// Returns ("", nil) when raw is empty (nothing to write) — a no-op, not an
// error, since an empty capture would tell a future reader nothing. maxBytes
// bounds how much of raw is written; pass <= 0 to use
// DefaultHeadlessFailureCaptureMaxBytes.
func WriteHeadlessFailureCapture(dir, sessionUUID, raw string, maxBytes int64) (absPath string, err error) {
	if dir == "" || sessionUUID == "" || raw == "" {
		return "", nil
	}
	if maxBytes <= 0 {
		maxBytes = DefaultHeadlessFailureCaptureMaxBytes
	}

	content := raw
	if int64(len(content)) > maxBytes {
		content = headlessFailureCaptureTruncationMarker + content[int64(len(content))-maxBytes:]
	}

	if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
		return "", fmt.Errorf("failed to create headless failure capture dir %s: %w", dir, mkErr)
	}

	absPath = filepath.Join(dir, headlessFailureCaptureFilePrefix+sessionUUID+".txt")
	if writeErr := os.WriteFile(absPath, []byte(content), 0o644); writeErr != nil {
		return "", fmt.Errorf("failed to write headless failure capture file: %w", writeErr)
	}

	return absPath, nil
}
