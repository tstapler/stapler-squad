package artifacts

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/tstapler/stapler-squad/log"
)

func (ae *ArtifactExtractor) scanFile(filePath string) {
	defer ae.inflight.Delete(filePath)

	f, err := os.Open(filePath)
	if err != nil {
		return // file may have been deleted
	}
	defer f.Close()

	ae.offsetsMu.Lock()
	offset := ae.offsets[filePath]
	ae.offsetsMu.Unlock()

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			log.Warn("[ArtifactExtractor] seek failed", "path", filePath, "err", err)
			return
		}
	}

	var newPRURLs, newCommitSHAs, newExternalURLs []string
	var newCommands []CommandArtifact
	scanner := bufio.NewScanner(f)
	// 10 MB buffer — matches tokens/parser.go; handles large base64 tool outputs.
	scanner.Buffer(make([]byte, maxScannerTokenSize), maxScannerTokenSize)

	for scanner.Scan() {
		line := scanner.Bytes()

		var entry artifactEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Partial last line — stop here; do NOT advance offset past it.
			break
		}

		switch entry.Type {
		case "user":
			// Extract from tool_result content (actual command output).
			// Ignore assistant text to avoid false positives from doc links.
			var msg artifactMessage
			if err := json.Unmarshal(entry.Message, &msg); err != nil {
				continue
			}
			for _, c := range msg.Content {
				if c.Type != "tool_result" {
					continue
				}
				text := extractToolResultText(c.Content)
				prs, shas, urls := ExtractFromToolResult(text)
				newPRURLs = append(newPRURLs, prs...)
				newCommitSHAs = append(newCommitSHAs, shas...)
				newExternalURLs = append(newExternalURLs, urls...)
			}
		case "assistant":
			// Extract from tool_use bash commands — gives earlier signal than waiting
			// for the matching tool_result output.
			var msg artifactMessage
			if err := json.Unmarshal(entry.Message, &msg); err != nil {
				continue
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
					newCommands = append(newCommands, *cmd)
				}
			}
		}
	}

	// Check for scanner errors (e.g. token-too-long) — abort without advancing offset.
	if err := scanner.Err(); err != nil {
		log.Warn("[ArtifactExtractor] scanner error", "path", filePath, "err", err)
		return
	}

	// bufio.Scanner reads ahead in chunks; the file position is only reliable
	// after Scan() returns false — use Seek to get the authoritative new offset.
	newOffset, _ := f.Seek(0, io.SeekCurrent)
	if newOffset <= offset {
		return // no new content
	}

	ae.offsetsMu.Lock()
	ae.offsets[filePath] = newOffset
	ae.offsetsMu.Unlock()

	if len(newPRURLs)+len(newCommitSHAs)+len(newExternalURLs)+len(newCommands) == 0 {
		return
	}

	title, ok := ae.lookupTitle(filePath)
	if !ok {
		return
	}

	blob := ae.mergeAndPersist(title, newOffset, newPRURLs, newCommitSHAs, newExternalURLs, newCommands)
	encoded, err := json.Marshal(blob)
	if err != nil {
		log.Warn("[ArtifactExtractor] marshal failed", "session", title, "err", err)
		return
	}
	if err := ae.storeFn(title, string(encoded)); err != nil {
		log.Warn("[ArtifactExtractor] persist failed", "session", title, "err", err)
		return
	}
	ae.OnScanComplete(title, blob)
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
