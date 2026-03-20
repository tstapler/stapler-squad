package services

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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
	logFilePath, err := log.GetLogFilePath(cfg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get log file path: %w", err))
	}

	// Read log file
	file, err := os.Open(logFilePath)
	if err != nil {
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
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
		log.WarningLog.Printf("Failed to activate window (bundle=%s, app=%s): %v, output: %s",
			bundleID, appName, err, outputStr)

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

	log.InfoLog.Printf("Window activated successfully (bundle=%s, app=%s)", bundleID, appName)
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

	log.InfoLog.Printf("[DebugSnapshot] Written to %s (%d bytes)", filePath, fileSizeBytes)

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
	// Log line format: [instance] LEVEL:date time file:line: message
	// Example: [pid-12345-timestamp] INFO:2025/10/17 14:23:45 app.go:123: Starting session
	logLineRegex := regexp.MustCompile(`^\[([^\]]+)\]\s+(\w+):(\d{4}/\d{2}/\d{2})\s+(\d{2}:\d{2}:\d{2})\s+([^:]+:\d+):\s+(.*)$`)

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

	var levelFilter string
	if req.Level != nil {
		levelFilter = strings.ToUpper(*req.Level)
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

		// Try to parse the log line
		matches := logLineRegex.FindStringSubmatch(line)
		if matches == nil || len(matches) < 7 {
			// Skip lines that don't match expected format
			continue
		}

		// Extract fields from regex match
		// matches[1] = instance (ignored for API)
		level := matches[2]
		dateStr := matches[3]
		timeStr := matches[4]
		source := matches[5]
		message := matches[6]

		// Parse timestamp - use ParseInLocation with Local timezone since logs are written in local time
		timestampStr := fmt.Sprintf("%s %s", dateStr, timeStr)
		timestamp, err := time.ParseInLocation("2006/01/02 15:04:05", timestampStr, time.Local)
		if err != nil {
			// Skip entries with invalid timestamps
			continue
		}

		// Apply level filter
		if levelFilter != "" && level != levelFilter {
			continue
		}

		// Apply time range filters
		if startTime != nil && timestamp.Before(*startTime) {
			continue
		}
		if endTime != nil && timestamp.After(*endTime) {
			continue
		}

		// Apply search query filter (case-insensitive, searches message and source)
		if searchQuery != "" {
			messageAndSource := strings.ToLower(message + " " + source)
			if !strings.Contains(messageAndSource, searchQuery) {
				continue
			}
		}

		// Create log entry
		entry := &sessionv1.LogEntry{
			Timestamp: timestamppb.New(timestamp),
			Level:     level,
			Message:   message,
			Source:    &source,
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
