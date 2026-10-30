package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tstapler/stapler-squad/log"
)

// MaxTranscriptTailBytes bounds how much of a transcript is read, whatever its size.
const MaxTranscriptTailBytes = 256 << 10

// ErrTranscriptPathRejected means the path resolves outside the Claude projects directory.
var ErrTranscriptPathRejected = errors.New("transcript path outside the Claude projects directory")

// AssistantMessage is the last assistant text found in a transcript tail.
type AssistantMessage struct {
	UUID string
	Text string
}

// ClaudeProjectsDir is the directory Claude Code writes transcripts under.
func ClaudeProjectsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// ValidateTranscriptPath resolves symlinks and returns the real path only when it lies
// inside projectsDir (also resolved), so ../ and symlink escapes are refused before any open.
func ValidateTranscriptPath(projectsDir, path string) (string, error) {
	if projectsDir == "" || path == "" {
		return "", ErrTranscriptPathRejected
	}
	root, err := filepath.EvalSymlinks(projectsDir)
	if err != nil {
		return "", fmt.Errorf("resolve projects dir: %w", err)
	}
	real, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve transcript path: %w", err)
	}
	rel, err := filepath.Rel(root, real)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrTranscriptPathRejected
	}
	return real, nil
}

// ReadLastAssistantMessage returns the last assistant text in the final MaxTranscriptTailBytes
// of the transcript at path. The path is validated against projectsDir first; a rejected path
// logs a Warn and returns ErrTranscriptPathRejected without opening anything. found is false
// when the tail holds no assistant text.
func ReadLastAssistantMessage(projectsDir, path string) (msg AssistantMessage, found bool, err error) {
	real, err := ValidateTranscriptPath(projectsDir, path)
	if err != nil {
		if errors.Is(err, ErrTranscriptPathRejected) {
			log.Warn("transcript path rejected", "path", path)
		}
		return AssistantMessage{}, false, err
	}
	f, err := os.Open(real) // #nosec G304 -- validated to be inside the Claude projects dir
	if err != nil {
		return AssistantMessage{}, false, err
	}
	defer f.Close()
	tail, truncated, err := readTail(f, MaxTranscriptTailBytes)
	if err != nil {
		return AssistantMessage{}, false, err
	}
	if truncated {
		// The first line is probably cut mid-record.
		if i := bytes.IndexByte(tail, '\n'); i >= 0 {
			tail = tail[i+1:]
		}
	}
	msg, found = lastAssistantText(tail)
	return msg, found, nil
}

func readTail(f *os.File, max int64) ([]byte, bool, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	var off int64
	if st.Size() > max {
		off = st.Size() - max
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return nil, false, err
	}
	b, err := io.ReadAll(io.LimitReader(f, max))
	return b, off > 0, err
}

type transcriptLine struct {
	Type    string `json:"type"`
	UUID    string `json:"uuid"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

func lastAssistantText(tail []byte) (AssistantMessage, bool) {
	var out AssistantMessage
	var found bool
	sc := bufio.NewScanner(bytes.NewReader(tail))
	sc.Buffer(make([]byte, 0, 64<<10), MaxTranscriptTailBytes+1)
	for sc.Scan() {
		var l transcriptLine
		if json.Unmarshal(sc.Bytes(), &l) != nil || l.Type != "assistant" {
			continue
		}
		if text := assistantContentText(l.Message.Content); text != "" {
			out, found = AssistantMessage{UUID: l.UUID, Text: text}, true
		}
	}
	return out, found
}

// assistantContentText joins the text blocks of a message content (a string or a block array).
func assistantContentText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
