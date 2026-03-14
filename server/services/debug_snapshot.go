package services

import (
	"bufio"
	"claude-squad/config"
	"claude-squad/log"
	"claude-squad/session"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// serverStartTime records when this process started, for uptime calculation.
var serverStartTime = time.Now()

// DebugSnapshot is the top-level JSON structure written to disk.
type DebugSnapshot struct {
	Version    int                 `json:"version"`
	Timestamp  time.Time           `json:"timestamp"`
	Note       string              `json:"note,omitempty"`
	Server     ServerInfo          `json:"server"`
	Sessions   []SessionSnapshot   `json:"sessions"`
	Tmux       TmuxSnapshot        `json:"tmux"`
	Approvals  ApprovalSnapshot    `json:"approvals"`
	RecentLogs RecentLogsSnapshot  `json:"recent_logs"`
	Errors     []string            `json:"errors,omitempty"`
}

// ServerInfo contains runtime metadata for the server process.
type ServerInfo struct {
	PID           int    `json:"pid"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	GoVersion     string `json:"go_version"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
}

// SessionSnapshot captures the state of a single session at snapshot time.
type SessionSnapshot struct {
	Title               string    `json:"title"`
	Status              string    `json:"status"`
	Program             string    `json:"program"`
	Path                string    `json:"path"`
	Branch              string    `json:"branch"`
	SessionType         string    `json:"session_type"`
	Category            string    `json:"category"`
	Tags                []string  `json:"tags"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	LastTerminalUpdate  time.Time `json:"last_terminal_update,omitempty"`
	LastMeaningfulOutput time.Time `json:"last_meaningful_output,omitempty"`
	LastOutputSignature string    `json:"last_output_signature,omitempty"`
	PaneContent         string    `json:"pane_content,omitempty"`
	PaneContentRaw      string    `json:"pane_content_raw,omitempty"`
	PaneContentTruncated bool     `json:"pane_content_truncated,omitempty"`
	InstanceType        string    `json:"instance_type"`
	GitHubPRNumber      int       `json:"github_pr_number,omitempty"`
}

// TmuxSnapshot captures global tmux state.
type TmuxSnapshot struct {
	ListSessionsOutput string              `json:"list_sessions_output"`
	PerSession         []TmuxSessionDetail `json:"per_session"`
}

// TmuxSessionDetail captures per-tmux-session diagnostic info.
type TmuxSessionDetail struct {
	TmuxSessionName  string `json:"tmux_session_name"`
	ListPanesOutput  string `json:"list_panes_output,omitempty"`
	PaneContent      string `json:"pane_content,omitempty"`
}

// ApprovalSnapshot captures the pending approvals state.
type ApprovalSnapshot struct {
	PendingCount int               `json:"pending_count"`
	Pending      []ApprovalDetail  `json:"pending,omitempty"`
}

// ApprovalDetail captures the fields of a single pending approval.
type ApprovalDetail struct {
	ID             string                 `json:"id"`
	SessionID      string                 `json:"session_id"`
	ClaudeSessionID string                `json:"claude_session_id"`
	ToolName       string                 `json:"tool_name"`
	ToolInput      map[string]interface{} `json:"tool_input,omitempty"`
	Cwd            string                 `json:"cwd"`
	PermissionMode string                 `json:"permission_mode"`
	CreatedAt      time.Time              `json:"created_at"`
	ExpiresAt      time.Time              `json:"expires_at"`
}

// RecentLogsSnapshot contains the most recent log lines.
type RecentLogsSnapshot struct {
	LogFilePath string   `json:"log_file_path"`
	LineCount   int      `json:"line_count"`
	Lines       []string `json:"lines"`
}

const (
	maxPaneContentBytes = 10000
	defaultLogLines     = 200
)

// CollectSnapshot gathers all diagnostic data into a DebugSnapshot.
// Individual subsystem failures are recorded in Errors and do not abort the collection.
func CollectSnapshot(ctx context.Context, note string, instances []*session.Instance, approvalStore *ApprovalStore, logLines int) *DebugSnapshot {
	if logLines <= 0 {
		logLines = defaultLogLines
	}

	snap := &DebugSnapshot{
		Version:   1,
		Timestamp: time.Now().UTC(),
		Note:      note,
	}

	snap.Server = collectServerInfo()
	snap.Sessions, snap.Errors = collectSessionSnapshots(ctx, instances)
	snap.Tmux, snap.Errors = collectTmuxSnapshot(ctx, snap.Errors)
	snap.Approvals = collectApprovalSnapshot(approvalStore)
	snap.RecentLogs = collectRecentLogs(logLines)

	return snap
}

// collectServerInfo returns runtime metadata for the current process.
func collectServerInfo() ServerInfo {
	return ServerInfo{
		PID:           os.Getpid(),
		UptimeSeconds: int64(time.Since(serverStartTime).Seconds()),
		GoVersion:     runtime.Version(),
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
	}
}

// collectSessionSnapshots captures the state of all active sessions.
func collectSessionSnapshots(ctx context.Context, instances []*session.Instance) ([]SessionSnapshot, []string) {
	var snapshots []SessionSnapshot
	var errors []string

	for _, inst := range instances {
		if inst == nil {
			continue
		}

		data := inst.ToInstanceData()

		ss := SessionSnapshot{
			Title:                data.Title,
			Status:               data.Status.String(),
			Program:              data.Program,
			Path:                 data.Path,
			Branch:               data.Branch,
			SessionType:          string(data.SessionType),
			Category:             data.Category,
			Tags:                 data.Tags,
			CreatedAt:            data.CreatedAt,
			UpdatedAt:            data.UpdatedAt,
			LastTerminalUpdate:   data.LastTerminalUpdate,
			LastMeaningfulOutput: data.LastMeaningfulOutput,
			LastOutputSignature:  data.LastOutputSignature,
			GitHubPRNumber:       data.GitHubPRNumber,
		}

		// Capture pane content with per-instance timeout
		paneCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		content, err := inst.CapturePaneContent()
		cancel()
		if err != nil {
			errors = append(errors, fmt.Sprintf("capture-pane for session '%s': %v", inst.Title, err))
		} else {
			_ = paneCtx
			if len(content) > maxPaneContentBytes {
				content = content[:maxPaneContentBytes]
				ss.PaneContentTruncated = true
			}
			ss.PaneContent = content
		}

		// Capture raw pane content
		rawCtx, rawCancel := context.WithTimeout(ctx, 5*time.Second)
		rawContent, err := inst.CapturePaneContentRaw()
		rawCancel()
		if err != nil {
			errors = append(errors, fmt.Sprintf("capture-pane-raw for session '%s': %v", inst.Title, err))
		} else {
			_ = rawCtx
			if len(rawContent) > maxPaneContentBytes {
				rawContent = rawContent[:maxPaneContentBytes]
			}
			ss.PaneContentRaw = rawContent
		}

		snapshots = append(snapshots, ss)
	}

	return snapshots, errors
}

// collectTmuxSnapshot runs global tmux diagnostic commands.
func collectTmuxSnapshot(ctx context.Context, existingErrors []string) (TmuxSnapshot, []string) {
	snap := TmuxSnapshot{}
	errors := existingErrors

	// list-sessions
	listCtx, listCancel := context.WithTimeout(ctx, 5*time.Second)
	listOut, err := exec.CommandContext(listCtx, "tmux", "list-sessions").Output()
	listCancel()
	if err != nil {
		errors = append(errors, fmt.Sprintf("tmux list-sessions: %v", err))
		snap.ListSessionsOutput = fmt.Sprintf("<error: %v>", err)
	} else {
		snap.ListSessionsOutput = strings.TrimSpace(string(listOut))
	}

	// For each tmux session, get list-panes and capture-pane
	for _, line := range strings.Split(snap.ListSessionsOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "<error") {
			continue
		}
		// session name is the part before the first ':'
		sessionName := strings.SplitN(line, ":", 2)[0]
		if sessionName == "" {
			continue
		}

		detail := TmuxSessionDetail{TmuxSessionName: sessionName}

		// list-panes
		panesCtx, panesCancel := context.WithTimeout(ctx, 5*time.Second)
		panesOut, err := exec.CommandContext(panesCtx, "tmux", "list-panes", "-t", sessionName).Output()
		panesCancel()
		if err != nil {
			errors = append(errors, fmt.Sprintf("tmux list-panes -t %s: %v", sessionName, err))
		} else {
			detail.ListPanesOutput = strings.TrimSpace(string(panesOut))
		}

		// capture-pane (visible area only, no full scrollback to avoid timeouts)
		capCtx, capCancel := context.WithTimeout(ctx, 5*time.Second)
		capOut, err := exec.CommandContext(capCtx, "tmux", "capture-pane", "-p", "-t", sessionName).Output()
		capCancel()
		if err != nil {
			errors = append(errors, fmt.Sprintf("tmux capture-pane -t %s: %v", sessionName, err))
		} else {
			content := strings.TrimSpace(string(capOut))
			if len(content) > maxPaneContentBytes {
				content = content[:maxPaneContentBytes]
			}
			detail.PaneContent = content
		}

		snap.PerSession = append(snap.PerSession, detail)
	}

	return snap, errors
}

// collectApprovalSnapshot captures the current pending approval state.
func collectApprovalSnapshot(store *ApprovalStore) ApprovalSnapshot {
	if store == nil {
		return ApprovalSnapshot{}
	}

	pending := store.ListAll()
	snap := ApprovalSnapshot{
		PendingCount: len(pending),
	}

	for _, a := range pending {
		snap.Pending = append(snap.Pending, ApprovalDetail{
			ID:              a.ID,
			SessionID:       a.SessionID,
			ClaudeSessionID: a.ClaudeSessionID,
			ToolName:        a.ToolName,
			ToolInput:       a.ToolInput,
			Cwd:             a.Cwd,
			PermissionMode:  a.PermissionMode,
			CreatedAt:       a.CreatedAt,
			ExpiresAt:       a.ExpiresAt,
		})
	}

	return snap
}

// collectRecentLogs reads the last n lines from the main log file.
func collectRecentLogs(lineCount int) RecentLogsSnapshot {
	cfg := log.ConfigToLogConfig(config.LoadConfig())
	logFilePath, err := log.GetLogFilePath(cfg)
	snap := RecentLogsSnapshot{
		LogFilePath: logFilePath,
	}
	if err != nil {
		snap.Lines = []string{fmt.Sprintf("<error getting log file path: %v>", err)}
		return snap
	}

	lines, err := tailFile(logFilePath, lineCount)
	if err != nil {
		snap.Lines = []string{fmt.Sprintf("<error reading log file: %v>", err)}
		return snap
	}

	snap.Lines = lines
	snap.LineCount = len(lines)
	return snap
}

// tailFile returns the last n lines of a file efficiently.
func tailFile(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	// Use a ring buffer of size n
	ring := make([]string, n)
	count := 0
	for scanner.Scan() {
		ring[count%n] = scanner.Text()
		count++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if count <= n {
		lines = ring[:count]
	} else {
		// Reconstruct in order
		start := count % n
		lines = make([]string, n)
		copy(lines, ring[start:])
		copy(lines[n-start:], ring[:start])
	}

	return lines, nil
}

// WriteSnapshot serializes the snapshot to a JSON file in the given directory.
// Returns the absolute path of the written file.
func WriteSnapshot(snap *DebugSnapshot, dir string) (string, error) {
	filename := fmt.Sprintf("debug-snapshot-%s.json", snap.Timestamp.Format("20060102-150405"))
	path := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write snapshot file: %w", err)
	}

	return path, nil
}
