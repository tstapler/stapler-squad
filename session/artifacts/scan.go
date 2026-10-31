package artifacts

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/scanstate"
)

// scanFile extracts artifacts from the bytes appended to filePath since its
// last scan. Files no session owns are skipped before any read.
func (ae *ArtifactExtractor) scanFile(filePath string) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return // file may have been deleted
	}
	pos, known := ae.position(filePath)
	if known && pos.Unchanged(fi) {
		return
	}
	title, ok := ae.lookupTitle(filePath)
	if !ok {
		return
	}

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
		return
	}

	var found scanFindings
	consumed, _, err := scanstate.ScanComplete(f, maxScannerTokenSize, found.addLine)
	if err != nil && !errors.Is(err, bufio.ErrTooLong) {
		log.Warn("[ArtifactExtractor] scanner error", "path", filePath, "err", err)
		return
	}
	newOffset := start + consumed
	newPos, err := scanstate.Capture(f, fi, newOffset)
	if err != nil {
		log.Warn("[ArtifactExtractor] fingerprint failed", "path", filePath, "err", err)
		return
	}

	if found.empty() {
		// New bytes held no artifacts — advance so they are not re-scanned.
		ae.setPosition(filePath, newPos)
		return
	}

	blob := ae.mergeAndPersist(title, newOffset, found.prURLs, found.commitSHAs, found.externalURLs, found.commands)
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
	ae.setPosition(filePath, newPos)
	ae.OnScanComplete(title, blob)
}

// scanFindings accumulates what one scan pass extracted.
type scanFindings struct {
	prURLs, commitSHAs, externalURLs []string
	commands                         []CommandArtifact
}

func (f *scanFindings) empty() bool {
	return len(f.prURLs)+len(f.commitSHAs)+len(f.externalURLs)+len(f.commands) == 0
}

// addLine extracts artifacts from one complete transcript line; malformed lines are skipped.
func (f *scanFindings) addLine(line []byte) {
	var entry artifactEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		return
	}
	var msg artifactMessage
	switch entry.Type {
	case "user":
		// Extract from tool_result content (actual command output).
		// Ignore assistant text to avoid false positives from doc links.
		if err := json.Unmarshal(entry.Message, &msg); err != nil {
			return
		}
		for _, c := range msg.Content {
			if c.Type != "tool_result" {
				continue
			}
			prs, shas, urls := ExtractFromToolResult(extractToolResultText(c.Content))
			f.prURLs = append(f.prURLs, prs...)
			f.commitSHAs = append(f.commitSHAs, shas...)
			f.externalURLs = append(f.externalURLs, urls...)
		}
	case "assistant":
		// Extract from tool_use bash commands — gives earlier signal than waiting
		// for the matching tool_result output.
		if err := json.Unmarshal(entry.Message, &msg); err != nil {
			return
		}
		for _, c := range msg.Content {
			if c.Type != "tool_use" || c.Name != "Bash" {
				continue
			}
			var inp bashInput
			if err := json.Unmarshal(c.Input, &inp); err != nil || inp.Command == "" {
				continue
			}
			if cmd := ExtractFromBashCommand(inp.Command); cmd != nil {
				f.commands = append(f.commands, *cmd)
			}
		}
	}
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
