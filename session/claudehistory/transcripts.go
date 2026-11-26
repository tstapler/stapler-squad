package claudehistory

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tstapler/stapler-squad/session/instance"
)

// ErrTranscriptNotFound reports a history session with no conversation file on disk.
var ErrTranscriptNotFound = errors.New("conversation file not found")

var sessionRefPattern = regexp.MustCompile(`"sessionId"\s*:\s*"([^"]+)"`)

// Transcripts maps session IDs to conversation files from one scan of the projects
// directory. Use it for bulk reads: GetMessagesFromConversationFile walks the tree
// per call, and walks all of it for a session that has no file.
type Transcripts struct {
	paths map[string]string
}

// ScanTranscripts indexes projectsDir (empty means ~/.claude/projects). A file
// named after a session wins over one that only references it in its first 5 lines.
func ScanTranscripts(ctx context.Context, projectsDir string) (*Transcripts, error) {
	if projectsDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user home directory: %w", err)
		}
		projectsDir = filepath.Join(home, ".claude", "projects")
	}

	named := map[string]string{}
	referenced := map[string]string{}
	err := filepath.WalkDir(projectsDir, func(path string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil || d.IsDir() || filepath.Ext(path) != ".jsonl" || strings.Contains(d.Name(), "agent-") {
			return nil // unreadable entries are skipped, as in findConversationFilePath
		}
		named[strings.TrimSuffix(d.Name(), ".jsonl")] = path
		addSessionRefs(path, referenced)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan transcripts: %w", err)
	}

	for id, path := range named {
		referenced[id] = path
	}
	return &Transcripts{paths: referenced}, nil
}

// addSessionRefs records the session IDs in path's first 5 lines; first file seen wins.
func addSessionRefs(path string, into map[string]string) {
	file, err := os.Open(path) // #nosec G304 -- path comes from filepath.WalkDir enumerating the projects dir
	if err != nil {
		return
	}
	defer file.Close() //nolint:errcheck

	bufPtr := conversationScanBufPool.Get().(*[]byte)
	defer conversationScanBufPool.Put(bufPtr) //nolint:staticcheck

	scanner := bufio.NewScanner(file)
	scanner.Buffer(*bufPtr, 1024*1024)
	for i := 0; i < 5 && scanner.Scan(); i++ {
		for _, m := range sessionRefPattern.FindAllSubmatch(scanner.Bytes(), -1) {
			if _, seen := into[string(m[1])]; !seen {
				into[string(m[1])] = path
			}
		}
	}
}

// Messages returns all messages of sessionID, oldest first.
func (t *Transcripts) Messages(sessionID string) ([]instance.ClaudeConversationMessage, error) {
	path, ok := t.paths[sessionID]
	if !ok {
		return nil, fmt.Errorf("%w for session ID: %s", ErrTranscriptNotFound, sessionID)
	}
	return instance.ReadAllMessagesFromFile(path)
}
