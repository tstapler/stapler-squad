package services

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// logLineRegex matches the legacy stdlib-logger line format
// ("[instance] LEVEL:2026/08/25 12:34:56 file.go:123: message"), still
// emitted by call sites that haven't migrated off log.InfoLog().Printf and
// co. (see log/log.go's atomicLogger-backed loggers). Most log lines are
// JSON now (see parseJSONLogLine) — this is the fallback for the rest.
var logLineRegex = regexp.MustCompile(`^\[([^\]]+)\]\s+(\w+):(\d{4}/\d{2}/\d{2})\s+(\d{2}:\d{2}:\d{2})\s+([^:]+:\d+):\s+(.*)$`)

// parsedLogLine is the format-agnostic result of parsing one log line,
// whichever of the two on-disk formats (JSON or legacy plain-text) produced it.
type parsedLogLine struct {
	Timestamp time.Time
	Level     string
	Message   string
	Source    string
}

// parseJSONLogLine parses one slog JSON-lines record (the format
// log/log.go's handler chain writes today: {"time":...,"level":...,"msg":...,
// plus arbitrary attribute fields}). Extra attributes beyond time/level/msg
// are folded into Message as "key=value" pairs, sorted by key for
// deterministic output, since sessionv1.LogEntry has no structured-fields
// slot — this keeps them visible (and searchable) in the API response
// without a proto change.
func parseJSONLogLine(line string) (parsedLogLine, bool) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return parsedLogLine{}, false
	}

	timeStr, _ := raw["time"].(string)
	msg, hasMsg := raw["msg"].(string)
	level, _ := raw["level"].(string)
	if timeStr == "" || !hasMsg {
		return parsedLogLine{}, false
	}

	timestamp, err := time.Parse(time.RFC3339Nano, timeStr)
	if err != nil {
		return parsedLogLine{}, false
	}

	extraKeys := make([]string, 0, len(raw))
	for k := range raw {
		switch k {
		case "time", "level", "msg":
			continue
		}
		extraKeys = append(extraKeys, k)
	}
	sort.Strings(extraKeys)

	message := msg
	if len(extraKeys) > 0 {
		pairs := make([]string, 0, len(extraKeys))
		for _, k := range extraKeys {
			pairs = append(pairs, k+"="+formatLogFieldValue(raw[k]))
		}
		message = msg + " " + strings.Join(pairs, " ")
	}

	if level == "" {
		level = "INFO"
	}

	return parsedLogLine{Timestamp: timestamp, Level: level, Message: message}, true
}

// formatLogFieldValue renders one JSON attribute value as it would appear
// in the legacy plain-text logger's key=value convention.
func formatLogFieldValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	case nil:
		return "null"
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	}
}

// parseLegacyLogLine parses one line in the old stdlib-logger format via
// logLineRegex — see that variable's doc comment.
func parseLegacyLogLine(line string) (parsedLogLine, bool) {
	matches := logLineRegex.FindStringSubmatch(line)
	if len(matches) < 7 {
		return parsedLogLine{}, false
	}

	// matches[1] = instance (ignored for API)
	level := matches[2]
	dateStr := matches[3]
	timeStr := matches[4]
	source := matches[5]
	message := matches[6]

	timestampStr := fmt.Sprintf("%s %s", dateStr, timeStr)
	// ParseInLocation with Local timezone since these lines are written in local time.
	timestamp, err := time.ParseInLocation("2006/01/02 15:04:05", timestampStr, time.Local)
	if err != nil {
		return parsedLogLine{}, false
	}

	return parsedLogLine{Timestamp: timestamp, Level: level, Message: message, Source: source}, true
}

// parseLogLine parses one log line in whichever format it's actually
// written in — JSON first (the current default), falling back to the
// legacy plain-text format for lines from call sites that still use it.
// Checks the first non-space byte before attempting a JSON parse so legacy
// lines (which always start with "[") skip straight to the regex path
// instead of paying for a doomed json.Unmarshal on every one of them.
func parseLogLine(line string) (parsedLogLine, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(trimmed, "{") {
		if parsed, ok := parseJSONLogLine(trimmed); ok {
			return parsed, true
		}
	}
	return parseLegacyLogLine(line)
}

// UtilityService handles miscellaneous utility RPCs: GetLogs, FocusWindow,
// and CreateDebugSnapshot.
//
// Dependencies:
//   - approvalStore:      needed by CreateDebugSnapshot to capture pending approvals
//   - reviewQueuePoller:  late-wired; needed by CreateDebugSnapshot for live instances
type UtilityService struct {
	approvalStore     *ApprovalStore
	reviewQueuePoller *session.ReviewQueuePoller
}

// NewUtilityService creates a UtilityService with the given dependencies.
func NewUtilityService(approvalStore *ApprovalStore) *UtilityService {
	return &UtilityService{approvalStore: approvalStore}
}

// SetReviewQueuePoller sets the review queue poller (late-wired).
func (us *UtilityService) SetReviewQueuePoller(poller *session.ReviewQueuePoller) {
	us.reviewQueuePoller = poller
}

// ---------------------------------------------------------------------------
// RPC methods
// ---------------------------------------------------------------------------

// GetLogs retrieves application logs with optional filtering and search.
func (us *UtilityService) GetLogs(
	ctx context.Context,
	req *connect.Request[sessionv1.GetLogsRequest],
) (*connect.Response[sessionv1.GetLogsResponse], error) {
	// Get log file path from config
	cfg := log.ConfigToLogConfig(config.LoadConfig())
	var logFilePath string
	var err error
	if sid := req.Msg.GetSessionId(); sid != "" {
		// Log files are written using inst.Title as the key (not UUID).
		// Resolve the incoming ID (which may be a UUID) to the session Title
		// before constructing the log file path.
		resolvedID := sid
		if us.reviewQueuePoller != nil {
			if inst := us.reviewQueuePoller.FindInstance(sid); inst != nil {
				resolvedID = inst.Title
			}
		}
		logFilePath, err = log.GetSessionLogFilePath(cfg, resolvedID)
	} else {
		logFilePath, err = log.GetLogFilePath(cfg)
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get log file path: %w", err))
	}

	// Read log file
	file, err := os.Open(logFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return connect.NewResponse(&sessionv1.GetLogsResponse{
				Entries:    []*sessionv1.LogEntry{},
				TotalCount: 0,
				HasMore:    false,
			}), nil
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to open log file: %w", err))
	}
	defer file.Close()

	// Parse logs with filters
	result, err := parseLogs(file, req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse logs: %w", err))
	}

	return connect.NewResponse(&sessionv1.GetLogsResponse{
		Entries:    result.Entries,
		TotalCount: int32(result.TotalCount),
		HasMore:    result.HasMore,
	}), nil
}

// FocusWindow activates a window for the specified application.
// Uses AppleScript on macOS to bring the application to front.
func (us *UtilityService) FocusWindow(
	ctx context.Context,
	req *connect.Request[sessionv1.FocusWindowRequest],
) (*connect.Response[sessionv1.FocusWindowResponse], error) {
	// Validate localhost-only origin
	if err := validateLocalhostOriginForFocus(ctx, req); err != nil {
		return nil, err
	}

	platform := detectPlatform()

	// Need at least bundle_id or app_name
	bundleID := ""
	if req.Msg.BundleId != nil {
		bundleID = *req.Msg.BundleId
	}
	appName := ""
	if req.Msg.AppName != nil {
		appName = *req.Msg.AppName
	}

	if bundleID == "" && appName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("bundle_id or app_name is required"))
	}

	// Only macOS is supported currently
	if platform != "darwin" {
		return connect.NewResponse(&sessionv1.FocusWindowResponse{
			Success:  false,
			Message:  fmt.Sprintf("window activation not supported on platform: %s", platform),
			Platform: platform,
		}), nil
	}

	// Try to activate the window using AppleScript
	var script string
	if bundleID != "" {
		// Prefer bundle ID for more reliable activation
		script = fmt.Sprintf(`tell application id "%s" to activate`, bundleID)
	} else {
		// Fallback to app name
		script = fmt.Sprintf(`tell application "%s" to activate`, appName)
	}

	// Execute AppleScript
	cmd := safeexec.CommandContext(ctx, "osascript", "-e", script)
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
		log.Warn("failed to activate window", "bundle", bundleID, "app", appName, "err", err, "output", outputStr)

		// Check for common permission-related errors
		message := fmt.Sprintf("failed to activate window: %v", err)
		if strings.Contains(outputStr, "not allowed") ||
			strings.Contains(outputStr, "permission") ||
			strings.Contains(outputStr, "accessibility") ||
			strings.Contains(outputStr, "System Events") {
			message = "Permission denied. Please grant Accessibility permissions: " +
				"System Preferences > Security & Privacy > Privacy > Accessibility. " +
				"Add Terminal (or your terminal app) to the list."
		} else if strings.Contains(outputStr, "Application isn't running") ||
			strings.Contains(outputStr, "Can't get application") {
			targetApp := bundleID
			if targetApp == "" {
				targetApp = appName
			}
			message = fmt.Sprintf("Application '%s' is not running", targetApp)
		}

		return connect.NewResponse(&sessionv1.FocusWindowResponse{
			Success:  false,
			Message:  message,
			Platform: platform,
		}), nil
	}

	log.Info("window activated successfully", "bundle", bundleID, "app", appName)
	return connect.NewResponse(&sessionv1.FocusWindowResponse{
		Success:  true,
		Message:  "Window activated successfully",
		Platform: platform,
	}), nil
}

// CreateDebugSnapshot captures diagnostic information and writes a JSON file to the log directory.
func (us *UtilityService) CreateDebugSnapshot(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateDebugSnapshotRequest],
) (*connect.Response[sessionv1.CreateDebugSnapshotResponse], error) {
	snapCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Collect live instances
	var instances []*session.Instance
	if us.reviewQueuePoller != nil {
		instances = us.reviewQueuePoller.GetInstances()
	}

	// Determine log line count
	logLines := int32(200)
	if req.Msg.LogLines != nil && *req.Msg.LogLines > 0 {
		logLines = *req.Msg.LogLines
	}

	note := ""
	if req.Msg.Note != nil {
		note = *req.Msg.Note
	}

	// Collect snapshot
	snap := CollectSnapshot(snapCtx, note, instances, us.approvalStore, int(logLines))

	// Get log directory for output
	logDir, err := log.GetLogDir(log.ConfigToLogConfig(config.LoadConfig()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get log directory: %w", err))
	}

	// Write snapshot to disk
	filePath, err := WriteSnapshot(snap, logDir)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to write snapshot: %w", err))
	}

	// Get file size
	var fileSizeBytes int64
	if info, err := os.Stat(filePath); err == nil {
		fileSizeBytes = info.Size()
	}

	// Build summary
	pendingApprovals := 0
	if us.approvalStore != nil {
		pendingApprovals = len(us.approvalStore.ListAll())
	}
	summary := fmt.Sprintf("Captured %d sessions, %d pending approvals, %d log lines",
		len(instances), pendingApprovals, snap.RecentLogs.LineCount)
	if len(snap.Errors) > 0 {
		summary += fmt.Sprintf(" (%d collection errors)", len(snap.Errors))
	}

	log.Info("[DebugSnapshot] written", "path", filePath, "bytes", fileSizeBytes)

	return connect.NewResponse(&sessionv1.CreateDebugSnapshotResponse{
		FilePath:      filePath,
		Summary:       summary,
		Timestamp:     snap.Timestamp.Format(time.RFC3339),
		FileSizeBytes: fileSizeBytes,
	}), nil
}

// ---------------------------------------------------------------------------
// Helper functions (shared utilities for this service)
// ---------------------------------------------------------------------------

// validateLocalhostOriginForFocus ensures FocusWindow requests come from localhost.
func validateLocalhostOriginForFocus(ctx context.Context, req *connect.Request[sessionv1.FocusWindowRequest]) error {
	// Check X-Real-IP header first (if behind a proxy)
	realIP := req.Header().Get("X-Real-IP")
	if realIP != "" {
		if !isLocalhostIP(realIP) {
			return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("FocusWindow can only be called from localhost"))
		}
		return nil
	}

	// Check X-Forwarded-For header
	forwardedFor := req.Header().Get("X-Forwarded-For")
	if forwardedFor != "" {
		ips := strings.Split(forwardedFor, ",")
		if len(ips) > 0 {
			clientIP := strings.TrimSpace(ips[0])
			if !isLocalhostIP(clientIP) {
				return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("FocusWindow can only be called from localhost"))
			}
			return nil
		}
	}

	// Direct connection mode - server binds to localhost
	return nil
}

// detectPlatform returns the current operating system.
func detectPlatform() string {
	switch osVal := os.Getenv("GOOS"); osVal {
	case "":
		// GOOS not set, use runtime detection
		return runtime.GOOS
	default:
		return osVal
	}
}

// parseLogsResult contains the result of parsing logs with pagination info
type parseLogsResult struct {
	Entries    []*sessionv1.LogEntry
	TotalCount int
	HasMore    bool
}

// parseLogs reads log file and applies filters to return matching entries
func parseLogs(reader io.Reader, req *sessionv1.GetLogsRequest) (*parseLogsResult, error) {
	var entries []*sessionv1.LogEntry
	scanner := bufio.NewScanner(reader)

	// Default limit if not specified
	limit := 100
	if req.Limit != nil && *req.Limit > 0 {
		limit = int(*req.Limit)
	}

	// Parse offset (default: 0)
	offset := 0
	if req.Offset != nil && *req.Offset > 0 {
		offset = int(*req.Offset)
	}

	// Parse filters
	var searchQuery string
	if req.SearchQuery != nil {
		searchQuery = strings.ToLower(*req.SearchQuery)
	}

	// Build level filter set: prefer repeated Levels field; fall back to single Level for backward compat.
	var levelFilterSet map[string]struct{}
	if len(req.Levels) > 0 {
		levelFilterSet = make(map[string]struct{}, len(req.Levels))
		for _, l := range req.Levels {
			if l != "" {
				levelFilterSet[strings.ToUpper(l)] = struct{}{}
			}
		}
	} else if req.Level != nil && *req.Level != "" {
		levelFilterSet = map[string]struct{}{strings.ToUpper(*req.Level): {}}
	}

	var startTime, endTime *time.Time
	if req.StartTime != nil {
		t := req.StartTime.AsTime()
		startTime = &t
	}
	if req.EndTime != nil {
		t := req.EndTime.AsTime()
		endTime = &t
	}

	for scanner.Scan() {
		line := scanner.Text()

		parsed, ok := parseLogLine(line)
		if !ok {
			// Skip lines that don't match either known format (e.g. a
			// stray non-log line, or one truncated mid-write).
			continue
		}

		// Apply level filter (OR logic across multi-level set)
		if len(levelFilterSet) > 0 {
			if _, ok := levelFilterSet[strings.ToUpper(parsed.Level)]; !ok {
				continue
			}
		}

		// Apply time range filters
		if startTime != nil && parsed.Timestamp.Before(*startTime) {
			continue
		}
		if endTime != nil && parsed.Timestamp.After(*endTime) {
			continue
		}

		// Apply search query filter (case-insensitive, searches message and source)
		if searchQuery != "" {
			messageAndSource := strings.ToLower(parsed.Message + " " + parsed.Source)
			if !strings.Contains(messageAndSource, searchQuery) {
				continue
			}
		}

		// Create log entry
		entry := &sessionv1.LogEntry{
			Timestamp: timestamppb.New(parsed.Timestamp),
			Level:     parsed.Level,
			Message:   parsed.Message,
			Source:    &parsed.Source,
		}

		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading log file: %w", err)
	}

	// Reverse entries to show most recent first
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	// Store total count before pagination
	totalCount := len(entries)

	// Apply offset
	if offset >= len(entries) {
		// Offset beyond available entries, return empty result
		return &parseLogsResult{
			Entries:    []*sessionv1.LogEntry{},
			TotalCount: totalCount,
			HasMore:    false,
		}, nil
	}

	// Apply offset and limit
	start := offset
	end := offset + limit
	if end > len(entries) {
		end = len(entries)
	}

	paginatedEntries := entries[start:end]
	hasMore := end < len(entries)

	return &parseLogsResult{
		Entries:    paginatedEntries,
		TotalCount: totalCount,
		HasMore:    hasMore,
	}, nil
}
