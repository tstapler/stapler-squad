package instance

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/procinfo"
)

// HistoryFileInfo contains information about a detected Claude history file.
type HistoryFileInfo struct {
	ConversationUUID string
	HistoryFilePath  string
	ProjectDir       string
	// ModTime is the on-disk mtime of the winning candidate. Populated by
	// DetectByPath; left zero by Detect() since the PID-based fast path is
	// process ground truth and doesn't need mtime gating.
	ModTime time.Time
}

// ProcessFileInspector is the interface used by HistoryFileDetector.
// This allows mocking in tests.
type ProcessFileInspector interface {
	OpenFiles(pid int32) ([]string, error)
	IsAlive(pid int32, expectedCreateTimeMs int64) bool
}

// HistoryFileDetector detects Claude JSONL history files for a given process.
type HistoryFileDetector struct {
	inspector ProcessFileInspector
	// homeDir overrides os.UserHomeDir() when set. Used in tests.
	homeDir string
}

// NewHistoryFileDetector creates a new HistoryFileDetector.
func NewHistoryFileDetector(inspector ProcessFileInspector) *HistoryFileDetector {
	return &HistoryFileDetector{inspector: inspector}
}

// NewHistoryFileDetectorWithHomeDir creates a HistoryFileDetector with a fixed
// home directory. Use this in tests to avoid writing to the real home dir.
func NewHistoryFileDetectorWithHomeDir(inspector ProcessFileInspector, homeDir string) *HistoryFileDetector {
	return &HistoryFileDetector{inspector: inspector, homeDir: homeDir}
}

// resolveHomeDir returns homeDir if set, otherwise os.UserHomeDir().
func (d *HistoryFileDetector) resolveHomeDir() (string, error) {
	if d.homeDir != "" {
		return d.homeDir, nil
	}
	return os.UserHomeDir()
}

// claudeProjectsPattern matches ~/.claude/projects/<projectDir>/<uuid>.jsonl
// but NOT agent-*.jsonl files.
var claudeProjectsPattern = regexp.MustCompile(`/\.claude/projects/([^/]+)/([^/]+)\.jsonl$`)

// Detect scans the open files of the given PID for Claude JSONL history files.
// Returns nil, nil if no matching file is found or the process is dead.
func (d *HistoryFileDetector) Detect(pid int32) (*HistoryFileInfo, error) {
	files, err := d.inspector.OpenFiles(pid)
	if err != nil {
		// Process not found or dead — not an error for the caller
		return nil, nil //nolint:nilnil
	}

	homeDir, err := d.resolveHomeDir()
	if err != nil {
		return nil, err
	}

	claudeProjects := filepath.Join(homeDir, ".claude", "projects")
	// Resolve symlinks in claudeProjects too (e.g. macOS /tmp -> /private/tmp,
	// or a symlinked $HOME) so it can be compared against the OS-reported,
	// already-resolved open file paths on equal footing. Discovered via the
	// Story 1.1.2b correlation-feasibility spike: a bare filepath.Join
	// comparison silently failed to match PID-exact candidates whose homeDir
	// was reached through a symlink, causing the PID path to fall through to
	// the (still-correct, but less precise) path-heuristic fallback instead
	// of ConfidencePIDExact.
	resolvedClaudeProjects, err := filepath.EvalSymlinks(claudeProjects)
	if err != nil {
		resolvedClaudeProjects = claudeProjects // dir may not exist yet
	}

	for _, path := range files {
		// Normalize symlinks
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			resolved = path // use original if symlink resolution fails
		}

		if !strings.HasPrefix(resolved, claudeProjects) && !strings.HasPrefix(resolved, resolvedClaudeProjects) {
			continue
		}

		// Match the pattern
		matches := claudeProjectsPattern.FindStringSubmatch(resolved)
		if len(matches) != 3 {
			continue
		}

		projectDir := matches[1]
		basename := matches[2] // filename without .jsonl

		// Skip agent files
		if strings.HasPrefix(basename, "agent-") {
			continue
		}

		// Validate UUID format
		if !domain.IsValidUUID(basename) {
			continue
		}

		return &HistoryFileInfo{
			ConversationUUID: basename,
			HistoryFilePath:  resolved,
			ProjectDir:       projectDir,
		}, nil
	}

	return nil, nil //nolint:nilnil
}

// ClaudeProjectDirName returns the directory name Claude uses for a given
// absolute project path. Claude encodes the path by replacing every non-alphanumeric
// character with '-'. This includes '/', '.', '_', and any other non-word characters.
// Example: "/Users/alice/myproject"          → "-Users-alice-myproject"
// Example: "/Users/alice/.hidden/my_project" → "-Users-alice--hidden-my-project"
func ClaudeProjectDirName(projectPath string) string {
	result := make([]byte, len(projectPath))
	for i := 0; i < len(projectPath); i++ {
		c := projectPath[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			result[i] = c
		} else {
			result[i] = '-'
		}
	}
	return string(result)
}

// DetectByPath scans ~/.claude/projects/<encoded-path>/ for the most recently
// modified conversation JSONL file. It does NOT require a live process, making
// it suitable for sessions whose tmux session is dead (e.g. after a reboot).
//
// Returns nil, nil if the project directory does not exist or contains no
// valid conversation files.
func (d *HistoryFileDetector) DetectByPath(projectPath string) (*HistoryFileInfo, error) {
	homeDir, err := d.resolveHomeDir()
	if err != nil {
		return nil, err
	}

	projectDir := ClaudeProjectDirName(projectPath)
	dir := filepath.Join(homeDir, ".claude", "projects", projectDir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		// Directory doesn't exist — not an error.
		return nil, nil //nolint:nilnil
	}

	type candidate struct {
		uuid    string
		path    string
		modTime int64
	}
	var candidates []candidate

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		basename := strings.TrimSuffix(name, ".jsonl")
		if strings.HasPrefix(basename, "agent-") {
			continue
		}
		if !domain.IsValidUUID(basename) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{
			uuid:    basename,
			path:    filepath.Join(dir, name),
			modTime: info.ModTime().UnixNano(),
		})
	}

	if len(candidates) == 0 {
		return nil, nil //nolint:nilnil
	}

	// Pick the most recently modified file — that's the active conversation.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime > candidates[j].modTime
	})
	best := candidates[0]
	return &HistoryFileInfo{
		ConversationUUID: best.uuid,
		HistoryFilePath:  best.path,
		ProjectDir:       projectDir,
		ModTime:          time.Unix(0, best.modTime),
	}, nil
}

// NewHistoryFileDetectorWithRealInspector creates a HistoryFileDetector using
// the real gopsutil-based ProcessInspector on darwin.
func NewHistoryFileDetectorWithRealInspector() *HistoryFileDetector {
	return NewHistoryFileDetector(procinfo.NewProcessInspector())
}

// ResolveFilePath reconstructs the on-disk JSONL path for a known
// (projectPath, conversationUUID) pair, using the same home-dir resolution
// (including the test override) and path-encoding convention as
// Detect/DetectByPath/DetectAllByPath. Used by callers that already know
// the UUID (e.g. a Resolved CorrelationResult from CorrelateCandidate) and
// need the file path without re-scanning the directory.
func (d *HistoryFileDetector) ResolveFilePath(projectPath, conversationUUID string) (string, error) {
	homeDir, err := d.resolveHomeDir()
	if err != nil {
		return "", err
	}
	projectDir := ClaudeProjectDirName(projectPath)
	return filepath.Join(homeDir, ".claude", "projects", projectDir, conversationUUID+".jsonl"), nil
}

// DetectAllByPath scans ~/.claude/projects/<encoded-path>/ and returns every
// valid conversation JSONL file found, sorted most-recently-modified first.
// Unlike DetectByPath, it does not silently collapse multiple candidates down
// to "most recent" — callers that need to detect ambiguity (e.g. import
// correlation) use this variant; DetectByPath's existing single-result
// contract and callers (HistoryLinker) are unchanged.
//
// Returns nil, nil if the project directory does not exist or contains no
// valid conversation files.
func (d *HistoryFileDetector) DetectAllByPath(projectPath string) ([]HistoryFileInfo, error) {
	homeDir, err := d.resolveHomeDir()
	if err != nil {
		return nil, err
	}

	projectDir := ClaudeProjectDirName(projectPath)
	dir := filepath.Join(homeDir, ".claude", "projects", projectDir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		// Directory doesn't exist — not an error.
		return nil, nil //nolint:nilnil
	}

	type candidate struct {
		info    HistoryFileInfo
		modTime int64
	}
	var candidates []candidate

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		basename := strings.TrimSuffix(name, ".jsonl")
		if strings.HasPrefix(basename, "agent-") {
			continue
		}
		if !domain.IsValidUUID(basename) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{
			info: HistoryFileInfo{
				ConversationUUID: basename,
				HistoryFilePath:  filepath.Join(dir, name),
				ProjectDir:       projectDir,
			},
			modTime: info.ModTime().UnixNano(),
		})
	}

	if len(candidates) == 0 {
		return nil, nil //nolint:nilnil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime > candidates[j].modTime
	})

	result := make([]HistoryFileInfo, 0, len(candidates))
	for _, c := range candidates {
		result = append(result, c.info)
	}
	return result, nil
}

// jsonlStartTime returns the first timestamp in a Claude JSONL file. ok=false means
// undatable (unreadable, none found, or past the read cap), so callers fail open.
func JsonlStartTime(path string) (t time.Time, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, false
	}
	defer f.Close()

	dec := json.NewDecoder(io.LimitReader(f, JsonlStartScanBytes))
	for range JsonlStartScanEntries {
		var entry struct {
			Timestamp string `json:"timestamp"`
		}
		if err := dec.Decode(&entry); err != nil {
			return time.Time{}, false
		}
		if ts, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
			return ts, true
		}
	}
	return time.Time{}, false
}

const (
	JsonlStartScanEntries = 20      // entries to scan for a timestamp
	JsonlStartScanBytes   = 1 << 20 // read cap; entries can embed multi-MB pasted content
)

// readLastNMessagesFromFile returns the last n user/assistant messages from the
// JSONL file at path without loading the whole file into memory. It reads the file
// in 64 KiB chunks from the end, parses lines as it goes, and stops as soon as it
// has collected n messages. Results are returned in chronological order (oldest first).
//
// If the file has fewer than n qualifying messages, all of them are returned.
func ReadLastNMessagesFromFile(path string, n int) ([]ClaudeConversationMessage, error) {
	if n <= 0 {
		return ReadAllMessagesFromFile(path)
	}

	f, err := os.Open(path) // #nosec G304 -- callers only ever pass a conversationFile resolved by findConversationFilePath's safe directory walk, or an internal HistoryFilePath, never external input
	if err != nil {
		return nil, fmt.Errorf("failed to open conversation file: %w", err)
	}
	defer f.Close() //nolint:errcheck

	stat, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat conversation file: %w", err)
	}
	fileSize := stat.Size()

	const chunkSize = int64(64 * 1024) // 64 KiB

	// messages accumulates entries newest-first; we reverse before returning.
	messages := make([]ClaudeConversationMessage, 0, n)

	pos := fileSize
	var tail []byte // partial line bytes from the right edge of the previous chunk

	for pos > 0 && len(messages) < n {
		readLen := chunkSize
		if pos < readLen {
			readLen = pos
		}
		pos -= readLen

		buf := make([]byte, readLen)
		nr, err := f.ReadAt(buf, pos)
		if err != nil && !isEOFErr(err) {
			return nil, fmt.Errorf("error reading conversation file: %w", err)
		}
		buf = buf[:nr]

		// combined = current chunk (left/older) + tail (right/newer)
		combined := append(buf, tail...) //nolint:gocritic

		// Walk from the right of combined, extracting complete lines.
		for len(messages) < n {
			idx := bytes.LastIndexByte(combined, '\n')
			if idx < 0 {
				// No newline in combined — the entire thing is a partial line.
				tail = combined
				combined = nil
				break
			}
			line := bytes.TrimSpace(combined[idx+1:])
			combined = combined[:idx]

			if len(line) == 0 {
				continue
			}
			var raw ConversationMessage
			if err := json.Unmarshal(line, &raw); err != nil {
				continue
			}
			if msg, ok := ExtractMsgContent(raw); ok {
				messages = append(messages, msg)
			}
		}
		if combined != nil {
			// whatever remains is the partial left edge — save for next iteration
			tail = combined
		}
	}

	// Process any remaining tail (the very first line of the file)
	if len(messages) < n && len(bytes.TrimSpace(tail)) > 0 {
		var raw ConversationMessage
		if err := json.Unmarshal(bytes.TrimSpace(tail), &raw); err == nil {
			if msg, ok := ExtractMsgContent(raw); ok {
				messages = append(messages, msg)
			}
		}
	}

	// messages is newest-first; reverse to chronological order.
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

// conversationMessage represents a single message in a Claude conversation file (per-session JSONL).
type ConversationMessage struct {
	Type      string `json:"type"`
	UUID      string `json:"uuid"`
	SessionID string `json:"sessionId"`
	Timestamp string `json:"timestamp"`
	CWD       string `json:"cwd"`
	Message   struct {
		Role    string      `json:"role"`
		Model   string      `json:"model,omitempty"`
		Content interface{} `json:"content"`
	} `json:"message"`
}

// ClaudeConversationMessage represents a message in a conversation
type ClaudeConversationMessage struct {
	Role      string
	Content   string
	Timestamp time.Time
	Model     string
}

// extractMsgContent converts a raw conversationMessage into a ClaudeConversationMessage.
// Returns (msg, true) when the raw entry represents a user or assistant turn;
// (zero, false) for tool-use, metadata, and other entry types.
func ExtractMsgContent(raw ConversationMessage) (ClaudeConversationMessage, bool) {
	if raw.Type != "user" && raw.Type != "assistant" {
		return ClaudeConversationMessage{}, false
	}

	var content string
	switch v := raw.Message.Content.(type) {
	case string:
		content = v
	case []interface{}:
		for _, item := range v {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if itemType, ok := itemMap["type"].(string); ok && itemType == "text" {
					if text, ok := itemMap["text"].(string); ok {
						content += text + "\n"
					}
				}
			}
		}
	default:
		contentJSON, _ := json.Marshal(raw.Message)
		content = string(contentJSON)
	}

	ts, _ := time.Parse(time.RFC3339, raw.Timestamp)
	return ClaudeConversationMessage{
		Role:      raw.Message.Role,
		Content:   content,
		Timestamp: ts,
		Model:     raw.Message.Model,
	}, true
}

// readAllMessagesFromFile reads every user/assistant message from the JSONL file
// at path in chronological order (oldest first).
func ReadAllMessagesFromFile(path string) ([]ClaudeConversationMessage, error) {
	file, err := os.Open(path) // #nosec G304 -- callers only ever pass a conversationFile resolved by findConversationFilePath's safe directory walk, or an internal HistoryFilePath, never external input
	if err != nil {
		return nil, fmt.Errorf("failed to open conversation file: %w", err)
	}
	defer file.Close() //nolint:errcheck

	var messages []ClaudeConversationMessage
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var raw ConversationMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		if msg, ok := ExtractMsgContent(raw); ok {
			messages = append(messages, msg)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading conversation file: %w", err)
	}
	return messages, nil
}

// isEOFErr reports whether err represents an end-of-file condition.
func isEOFErr(err error) bool {
	return err == io.EOF || err == io.ErrUnexpectedEOF
}
