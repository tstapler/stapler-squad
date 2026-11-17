package artifacts

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/tstapler/stapler-squad/session/domain"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/scanstate"
)

// scanFile extracts artifacts from the bytes appended to filePath since its
// last scan. Files no session owns are skipped before any read. A transcript
// whose session has no recorded scan state is re-checked even when unchanged,
// so a scan that found nothing before states existed ends up as none_found.
func (ae *ArtifactExtractor) scanFile(filePath string) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return // file may have been deleted
	}
	prev, known := ae.tracked(filePath)
	rescanAll := known && ae.stale(prev) && ae.scanV2()
	if known && prev.Unchanged(fi) && prev.verified && !rescanAll {
		return
	}
	title, sub, ok := ae.ownerOf(filePath)
	if !ok {
		return
	}
	pos, version := prev.Position, ae.scanVersion
	switch {
	case !known || rescanAll || (prev.owner != "" && prev.owner != title):
		// First sight, the flagged version bump, or a different session now owns the file: scan it all.
		pos = scanstate.Position{}
	default:
		version = domain.NormalizeScanVersion(prev.version) // incremental: the file's older bytes keep their version
	}
	ae.scanFrom(filePath, fi, title, sub, pos, version)
}

// scanFrom scans filePath from pos (or the top if pos no longer fits the file)
// and commits the findings; a read error records the scan as failed.
func (ae *ArtifactExtractor) scanFrom(filePath string, fi os.FileInfo, title string, sub bool, pos scanstate.Position, version int) {
	f, err := os.Open(filePath) // #nosec G304 -- filePath comes from HistoryLinker's filesystem watch of ~/.claude/projects, an enumerated existing conversation file path, not external request input
	if err != nil {
		return
	}
	defer f.Close()

	start := pos.Offset
	if !pos.Resumable(f, fi.Size()) { // truncated, rotated or rewritten: rescan from the top
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		log.Warn("[ArtifactExtractor] seek failed", "path", filePath, "err", err)
		ae.markFailed(filePath, title)
		return
	}

	var found scanFindings
	consumed, _, err := scanstate.ScanComplete(f, maxScannerTokenSize, found.addLine)
	found.finish()
	if err != nil && !errors.Is(err, bufio.ErrTooLong) {
		log.Warn("[ArtifactExtractor] scanner error", "path", filePath, "err", err)
		ae.markFailed(filePath, title)
		return
	}
	newOffset := start + consumed
	newPos, err := scanstate.Capture(f, fi, newOffset)
	if err != nil {
		log.Warn("[ArtifactExtractor] fingerprint failed", "path", filePath, "err", err)
		ae.markFailed(filePath, title)
		return
	}
	ae.commitScan(filePath, title, sub, start == 0, newOffset, newPos, version, &found)
}

// commitScan persists a scan's findings. Incremental scans that found nothing
// only advance the position, unless the session has no recorded scan state yet
// (then the empty result is persisted so the tab can say "none found"); a scan
// from the top always persists so its timestamp is fresh. Subagent scans never
// persist emptiness or touch the parent's stored offset.
func (ae *ArtifactExtractor) commitScan(filePath, title string, sub, full bool, newOffset int64, newPos scanstate.Position, version int, found *scanFindings) {
	if found.empty() && (sub || (!full && ae.stateRecorded(title))) {
		ae.setPosition(filePath, title, newPos, version)
		return
	}
	if sub {
		newOffset = keepOffset
	}
	blob := ae.mergeAndPersist(title, newOffset, found, full && !sub && version == ae.scanVersion)
	encoded, err := json.Marshal(blob)
	if err != nil {
		log.Warn("[ArtifactExtractor] marshal failed", "session", title, "err", err)
		return
	}
	// Only commit the position AFTER storeFn succeeds (M-2 fix).
	// If storeFn fails, the position stays at the old value so the next scan retries.
	if err := ae.storeFn(title, string(encoded)); err != nil {
		log.Warn("[ArtifactExtractor] persist failed", "session", title, "err", err)
		return
	}
	ae.verified.Store(title, struct{}{})
	ae.setPosition(filePath, title, newPos, version)
	ae.OnScanComplete(title, blob)
}

// scanFindings accumulates what one scan pass extracted.
type scanFindings struct {
	prURLs, commitSHAs, externalURLs []string
	commands                         []domain.CommandArtifact
	records                          []domain.ArtifactRecord
	pending                          map[string]pendingCall // tool_use id -> call awaiting its result
}

func (f *scanFindings) empty() bool {
	return len(f.prURLs)+len(f.commitSHAs)+len(f.externalURLs)+len(f.commands)+len(f.records) == 0
}

// extractToolResultText converts tool_result content (string or []textBlock) to plain text.
func extractToolResultText(raw json.RawMessage) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, b := range blocks {
		if b.Type == "text" {
			sb.WriteString(b.Text)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}
