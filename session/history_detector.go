package session

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tstapler/stapler-squad/session/procinfo"
)

// HistoryFileInfo contains information about a detected Claude history file.
type HistoryFileInfo struct {
	ConversationUUID string
	HistoryFilePath  string
	ProjectDir       string
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

	for _, path := range files {
		// Normalize symlinks
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			resolved = path // use original if symlink resolution fails
		}

		if !strings.HasPrefix(resolved, claudeProjects) {
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
		if !isValidUUID(basename) {
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
		if !isValidUUID(basename) {
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
	}, nil
}

// NewHistoryFileDetectorWithRealInspector creates a HistoryFileDetector using
// the real gopsutil-based ProcessInspector on darwin.
func NewHistoryFileDetectorWithRealInspector() *HistoryFileDetector {
	return NewHistoryFileDetector(procinfo.NewProcessInspector())
}
